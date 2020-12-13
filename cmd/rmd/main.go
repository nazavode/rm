package main

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
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

func doWork(id uint64, c *conf, item *url.URL, wg *sync.WaitGroup) {
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

func main() {
	app := &cli.App{
		Name:    "rmd",
		Usage:   "A reMarkable cloud (https://my.remarkable.com) sync daemon",
		Version: "v0.1a",
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
			&cli.StringFlag{
				Name:    "rmapi",
				Value:   "rmapi",
				Usage:   "Use `EXE` as reMarkable cloud uploader program",
				EnvVars: []string{"RMD_RMAPI"},
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
			} else {
				log.WithField("path", tmp).Trace("working directory created")
			}
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
			// 1. Spawn item producer
			conn := &pocket.Auth{
				ConsumerKey: ctx.String("pocket-key"),
				AccessToken: ctx.String("pocket-token"),
			}
			// Input channels
			interval := time.NewTicker(ctx.Duration("interval"))
			stop := make(chan bool)
			// Actually this never stops, so the following
			// defer is practically useless.
			defer func() {
				interval.Stop()
				stop <- true
			}()
			opts := pocket.NewRetrieveOptions(pocket.WithTag("rm"), pocket.Unread)
			var wg sync.WaitGroup
			var id uint64 = 0
			for item := range conn.Tail(opts, interval.C, stop) {
				switch v := item.(type) {
				case *url.URL:
					wg.Add(1)
					go doWork(id, c, v, &wg)
					id++
				case error:
					log.WithError(v).Warn("item retrieval failed")
				}
			}
			wg.Wait()
			return nil
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
