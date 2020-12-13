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
	Workdir string
	Pandoc  string
	Keep    bool
}

func doWork(id uint64, c *conf, item *url.URL, conn *rm.Connection, wg *sync.WaitGroup) {
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
	outPath := path.Join(c.Workdir, basename)
	defer func() {
		if !c.Keep {
			if err := os.Remove(outPath); err != nil {
				out.WithField("path", outPath).
					WithError(err).
					Warn("failed to remove converted item")
			} else {
				out.WithField("path", outPath).
					Trace("converted item removed")
			}
		}
	}()
	out.WithField("path", outPath).Trace("converting item")
	if err := rm.DocumentToEPUB(doc, outPath, c.Timeout); err != nil {
		out.WithField("path", outPath).
			WithError(err).
			Warn("item conversion failed")
		return
	}
	out.WithField("path", outPath).Trace("item converted")
	// Upload
}

func handleSignals(notify chan<- bool) {
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
		notify <- true
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
	c.Workdir = tmp
	// Create downstream destination directory
	log.Trace("connecting to reMarkable cloud")
	rmConn, err := rm.NewConnection(ctx.String("rm-device"), ctx.String("rm-user"))
	if err != nil {
		log.WithError(err).
			Fatal("connection to reMarkable cloud failed")
	}
	log.Trace("connected to reMarkable cloud")
	log.WithField("path", ctx.String("dest")).
		Trace("creating reMarkable destination directory")
	if err := rmConn.MkDir(ctx.String("dest")); err != nil {
		log.WithError(err).
			Fatal("creation of reMarkable destination directory failed")
	}
	log.WithField("path", ctx.String("dest")).
		Trace("reMarkable destination directory created")
	// Spawn item producer
	pocketConn := &pocket.Auth{
		ConsumerKey: ctx.String("pocket-key"),
		AccessToken: ctx.String("pocket-token"),
	}
	tickTailer := time.NewTicker(ctx.Duration("interval"))
	stopTailer := make(chan bool, 1)
	handleSignals(stopTailer)
	defer func() {
		stopTailer <- true
		tickTailer.Stop()
	}()
	opts := pocket.NewRetrieveOptions(pocket.WithTag("rm"), pocket.Unread)
	var wg sync.WaitGroup
	var id uint64 = 0
	log.Trace("start listening for new items")
	for item := range pocketConn.Tail(opts, tickTailer.C, stopTailer) {
		switch v := item.(type) {
		case *url.URL:
			wg.Add(1)
			go doWork(id, c, v, rmConn, &wg)
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
