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
	"errors"

	"github.com/nazavode/rm"
	"github.com/nazavode/rm/pocket"
	log "github.com/sirupsen/logrus"
	cli "github.com/urfave/cli/v2"
)

type conf struct {
	ConnectionAttempts    int
	Keep                  bool
	Timeout               time.Duration
	PollInterval          time.Duration
	WorkDir               string
	DestDir               string
	RemarkableDeviceToken string
	RemarkableUserToken   string
	PocketKey             string
	PocketToken           string
}

type document struct {
	ID       uint64
	FilePath string
}

func doUpload(c *conf, conn *rm.Connection, wg *sync.WaitGroup) (chan<- *document, chan<- bool) {
	in := make(chan *document, 10)
	stop := make(chan bool, 1)
	go func() {
		log.Trace("uploader started")
		defer func() {
			wg.Done()
			log.Trace("uploader done")
		}()
		for {
			select {
			case doc := <-in:
				dlog := log.WithFields(log.Fields{"id": doc.ID, "path": doc.FilePath})
				err := conn.Put(doc.FilePath, c.DestDir)
				if errors.Is(err, rm.ErrAlreadyExists) {
					dlog.Trace("file already exists, skipping")
					continue
				}
				if err != nil {
					dlog.WithError(err).Warn("document upload failed")
					dlog.Trace("refreshing connection tokens")
					conn, err = rmConnect(c)
					if err != nil {
						dlog.WithError(err).Error("cannot refresh connection, skipping document")
						continue
					}
					dlog.Trace("connection tokens refreshed")
					err = conn.Put(doc.FilePath, c.DestDir)
					if errors.Is(err, rm.ErrAlreadyExists) {
						dlog.Trace("file already exists, skipping")
						continue
					}
					if err != nil {
						dlog.WithError(err).Warn("document upload failed, skipping document")
						continue
					}
				}
				dlog.Info("document uploaded")
				if !c.Keep {
					if err := os.Remove(doc.FilePath); err != nil {
						dlog.WithError(err).
							Warn("failed to remove document")
					} else {
						dlog.Trace("document removed")
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

func notifySignals(chans ...chan<- bool) {
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

func rmConnect(c *conf) (*rm.Connection, error) {
	// First attempt with provided user token
	rmConn, err := rm.NewConnection(c.RemarkableDeviceToken, c.RemarkableUserToken)
	if err != nil {
		// First attempt errored, begin subsequent attempts
		log.WithError(err).
			Trace("first attempt at connecting to reMarkable cloud failed")
		for i := 0; i < c.ConnectionAttempts; i++ {
			log := log.WithFields(log.Fields{"attempt": i + 1, "limit": c.ConnectionAttempts})
			log.Trace("requesting a new reMarkable user token")
			c.RemarkableUserToken, err = rm.NewUserToken(c.RemarkableDeviceToken)
			if err != nil {
				log.WithError(err).Trace("new user token request failed")
				continue
			}
			log.Trace("connecting to reMarkable cloud")
			rmConn, err = rm.NewConnection(c.RemarkableDeviceToken, c.RemarkableUserToken)
			if err == nil {
				break
			}
			log.WithError(err).
				Trace("connection to reMarkable cloud failed")
		}
		if err != nil {
			return nil, err
		}
	}
	return rmConn, nil
}

func appMain(c *conf) error {
	// Ensure we have external commands
	if _, err := exec.LookPath("pandoc"); err != nil {
		return err
	}
	log.Trace("connecting to reMarkable cloud")
	rmConn, err := rmConnect(c)
	if err != nil {
		log.WithError(err).Fatal("cannot connect to reMarkable cloud")
	}
	log.Trace("connected to reMarkable cloud")
	// Create downstream destination directory
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
		ConsumerKey: c.PocketKey,
		AccessToken: c.PocketToken,
	}
	tailerTick := time.NewTicker(c.PollInterval)
	tailerStop := make(chan bool, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	uploaderIn, uploaderStop := doUpload(c, rmConn, &wg)
	notifySignals(tailerStop, uploaderStop)
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
		Name:     "rmd",
		Usage:    "A reMarkable cloud (https://my.remarkable.com) sync daemon",
		Version:  "v0.1a",
		Compiled: time.Now(),
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
				Value:   10 * time.Second,
			},
			&cli.StringFlag{
				Name:    "rm-device",
				Usage:   "Use `STRING` as reMarkable cloud API device token",
				EnvVars: []string{"RMD_RM_DEVICE_TOKEN"},
			},
			&cli.StringFlag{
				Name:    "rm-user",
				Usage:   "Use `STRING` as reMarkable cloud API user token; if not provided will be generated",
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
			&cli.IntFlag{
				Name:    "retry",
				Usage:   "Use `NUM` as the maximum number of connection attempts to reMarkable cloud",
				EnvVars: []string{"RMD_RETRY"},
				Value:   3,
			},
			&cli.BoolFlag{
				Name:    "keep",
				Usage:   "Keep all temporary files.",
				EnvVars: []string{"RMD_KEEP"},
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Verbose mode. Causes rmd to print debugging messages about its progress.",
				EnvVars: []string{"RMD_VERBOSE"},
			},
		},
		Action: func(ctx *cli.Context) error {
			log.SetLevel(log.WarnLevel)
			if ctx.Bool("verbose") {
				log.SetLevel(log.TraceLevel)
			}
			tmpdir, err := ioutil.TempDir("", "rmd")
			if err != nil {
				log.WithField("path", tmpdir).Fatal("failed to create working directory")
			}
			log.WithField("path", tmpdir).Trace("working directory created")
			ctx.Deadline()
			c := &conf{
				ConnectionAttempts:    ctx.Int("retry"),
				Keep:                  ctx.Bool("keep"),
				Timeout:               ctx.Duration("timeout"),
				PollInterval:          ctx.Duration("interval"),
				WorkDir:               tmpdir,
				DestDir:               ctx.String("dest"),
				RemarkableDeviceToken: ctx.String("rm-device"),
				RemarkableUserToken:   ctx.String("rm-user"),
				PocketKey:             ctx.String("pocket-key"),
				PocketToken:           ctx.String("pocket-token"),
			}
			if !c.Keep {
				defer func() {
					if err := os.RemoveAll(tmpdir); err != nil {
						log.WithField("path", tmpdir).Warn("failed to remove working directory")
					} else {
						log.WithField("path", tmpdir).Trace("working directory removed")
					}
				}()
			}
			return appMain(c)
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
