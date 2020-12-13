package main

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"sync"
	"time"

	"github.com/nazavode/rm"
	"github.com/nazavode/rm/pocket"
	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
)

type conf struct {
	Timeout time.Duration
	WorkDir string
	Pandoc  string
	Keep    bool
	DestDir string
}

type document struct {
	ID       uint64
	FilePath string
}

func doUpload(c *conf, conn *rm.Connection, wg *sync.WaitGroup) (chan<- *document, chan<- bool) {
	in := make(chan *document, 100)
	stop := make(chan bool, 1)
	go func() {
		log.Trace("uploader started")
		defer wg.Done()
		defer log.Trace("uploader done")
		for {
			select {
			case doc := <-in:
				if err := conn.Put(doc.FilePath, c.DestDir); err != nil {
					log.WithError(err).
						WithFields(log.Fields{"id": doc.ID, "path": doc.FilePath}).
						Warn("document upload failed")
				}
				log.WithFields(log.Fields{"id": doc.ID, "path": doc.FilePath}).
					Trace("document uploaded")
				if !c.Keep {
					if err := os.Remove(doc.FilePath); err != nil {
						log.WithField("path", doc.FilePath).
							WithError(err).
							Warn("failed to remove document")
					} else {
						log.WithField("path", doc.FilePath).
							Trace("document removed")
					}
				}

			case <-stop:
				log.Trace("uploader received shutdown request")
				return
			}
		}
	}()
	return in, stop
}

func doRetrieve(id uint64, c *conf, item *url.URL, upload chan<- *document, wg *sync.WaitGroup) {
	defer wg.Done()
	out := log.WithField("id", id)
	out.Trace("worker started")
	defer out.Trace("worker done")
	// Download URL
	out.WithField("url", item).Trace("retrieving item")
	doc, err := rm.Retrieve(item, c.Timeout)
	if err != nil {
		out.WithField("url", item).
			WithError(err).
			Warn("failed to retrieve item")
		return
	}
	out = out.WithField("item", doc.Slug())
	out.WithField("url", item).Trace("item retrieved")
	// Convert document
	basename := fmt.Sprintf("%s.epub", doc.Slug())
	outPath := path.Join(c.WorkDir, basename)
	out.WithField("path", outPath).Trace("converting item")
	if err := rm.DocumentToEPUB(doc, outPath, c.Timeout); err != nil {
		out.WithField("path", outPath).
			WithError(err).
			Warn("item conversion failed")
		return
	}
	out.WithField("path", outPath).Trace("item converted")
	// Upload
	upload <- &document{ID: id, FilePath: outPath}
}

func handleSignals(chans ...chan<- bool) {
	// Send stop signal to tailer goroutine when
	// we receive a SIGTERM
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() {
		log.Trace("signal handler started")
		defer log.Trace("signal handler exiting")
		sig := <-signals
		log.WithField("signal", sig).
			Trace("signal handler received signal")
		signal.Stop(signals)
		for _, c := range chans {
			c <- true
		}
	}()
}

func appMain(ctx *cli.Context) error {
	log.SetLevel(log.WarnLevel)
	if ctx.Bool("verbose") {
		log.SetLevel(log.TraceLevel)
	}
	c := &conf{
		Timeout: ctx.Duration("timeout"),
		Pandoc:  ctx.String("pandoc"),
		Keep:    ctx.Bool("keep"),
		DestDir: ctx.String("dest"),
	}
	// Ensure we have external commands
	if _, err := exec.LookPath(c.Pandoc); err != nil {
		return err
	}
	// Additional scaffolding
	tmp, err := ioutil.TempDir("", "rmd")
	if err != nil {
		log.WithField("path", tmp).Fatal("failed to create working directory")
	}
	log.WithField("path", tmp).Trace("working directory created")
	defer func() {
		if !c.Keep {
			if err := os.RemoveAll(tmp); err != nil {
				log.WithField("path", tmp).Warn("failed to remove working directory")
			} else {
				log.WithField("path", tmp).Trace("working directory removed")
			}
		}
	}()
	c.WorkDir = tmp
	// Create downstream destination directory
	log.Trace("connecting to reMarkable cloud")
	rmConn, err := rm.NewConnection(ctx.String("rm-device"), ctx.String("rm-user"))
	if err != nil {
		log.WithError(err).
			Fatal("connection to reMarkable cloud failed")
	}
	log.Trace("connected to reMarkable cloud")
	log.WithField("path", c.DestDir).
		Trace("creating reMarkable destination directory")
	if err := rmConn.MkDir(c.DestDir); err != nil {
		log.WithError(err).
			Fatal("creation of reMarkable destination directory failed")
	}
	log.WithField("path", c.DestDir).
		Trace("reMarkable destination directory created")
	// Spawn item producer
	pocketConn := &pocket.Auth{
		ConsumerKey: ctx.String("pocket-key"),
		AccessToken: ctx.String("pocket-token"),
	}
	tailerTick := time.NewTicker(ctx.Duration("interval"))
	tailerStop := make(chan bool, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	uploaderIn, uploaderStop := doUpload(c, rmConn, &wg)
	handleSignals(tailerStop, uploaderStop)
	defer func() {
		uploaderStop <- true
		tailerStop <- true
		tailerTick.Stop()
	}()
	opts := pocket.NewRetrieveOptions(pocket.WithTag("rm"), pocket.Unread)
	var id uint64 = 0
	log.Trace("start listening for new items")
	for item := range pocketConn.Tail(opts, tailerTick.C, tailerStop) {
		switch v := item.(type) {
		case *url.URL:
			wg.Add(1)
			go doRetrieve(id, c, v, uploaderIn, &wg)
			id++
		case error:
			log.WithError(v).Warn("item processing failed, skipping")
		default:
			log.Warn("unexpected item, skipping")
		}
	}
	log.Trace("waiting for remaining workers to exit")
	wg.Wait()
	log.Trace("all workers exited")
	return nil
}

func main() {
	app := &cli.App{
		Name:    "rmd",
		Usage:   "A reMarkable cloud (https://my.remarkable.com) sync daemon",
		Version: "v0.1a",
		Action:  appMain,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "dest",
				Aliases: []string{"d"},
				Usage:   "Use `PATH` as the cloud destination path",
				EnvVars: []string{"RMD_DEST"},
				Value:   "/Pocket",
			},
			&cli.DurationFlag{
				Name:    "interval",
				Aliases: []string{"n"},
				Usage:   "Use `DURATION` as the poll interval",
				EnvVars: []string{"RMD_INTERVAL"},
				Value:   30 * time.Second,
			},
			&cli.StringFlag{
				Name:    "rm-device",
				Usage:   "Use `STRING` as reMarkable cloud API device token",
				EnvVars: []string{"RMD_RM_DEVICE_TOKEN"},
			},
			&cli.StringFlag{
				Name:    "rm-user",
				Usage:   "Use `STRING` as reMarkable cloud API user token",
				EnvVars: []string{"RMD_RM_USER_TOKEN"},
			},
			&cli.StringFlag{
				Name:    "pocket-token",
				Usage:   "Use `STRING` as Pocket API access token",
				EnvVars: []string{"RMD_POCKET_TOKEN"},
			},
			&cli.StringFlag{
				Name:    "pocket-key",
				Usage:   "Use `STRING` as Pocket API consumer key",
				EnvVars: []string{"RMD_POCKET_KEY"},
			},
			&cli.DurationFlag{
				Name:    "timeout",
				Aliases: []string{"t"},
				Usage:   "Use `DURATION` as the hard timeout for external programs",
				EnvVars: []string{"RMD_TIMEOUT"},
				Value:   30 * time.Second,
			},
			&cli.BoolFlag{
				Name:    "keep",
				Usage:   "Keep all temporary files.",
				EnvVars: []string{"RMD_KEEP"},
			},
			&cli.StringFlag{
				Name:    "pandoc",
				Usage:   "Use `EXE` as document format conversion program",
				EnvVars: []string{"RMD_PANDOC"},
				Value:   "pandoc",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Verbose mode. Causes rmd to print debugging messages about its progress.",
				EnvVars: []string{"RMD_VERBOSE"},
			},
		},
	}
	cli.VersionFlag = &cli.BoolFlag{
		Name:  "version",
		Usage: "print the version and exit",
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
