package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/nazavode/rm/pocket"
	"io/ioutil"
	"log"
	"os"
	"time"
)

const (
	defaultAuthPath string = "~/.pocket"
	defaultTag      string = "rm"
)

func main() {
	var log = log.New(os.Stderr, "", 0)
	authPath := flag.String("auth", defaultAuthPath, "Authentication file.")
	intervalStr := flag.String("interval", "60s", "Update interval, in nanoseconds if a time unit is not specified.")
	flag.Parse()
	interval, err := time.ParseDuration(*intervalStr)
	if err != nil {
		log.Fatal(err)
	}
	// Make sure the configuration file is private,
	// similar to what ssh does with secrets
	info, err := os.Stat(*authPath)
	if err != nil {
		log.Fatal(err)
	}
	perm := info.Mode().Perm()
	if perm&0b000111111 != 0 {
		log.Fatalf("Permissions %s for %#v are too open (contains secrets).\n", perm, *authPath)
	}
	jsonFile, err := os.Open(*authPath)
	if err != nil {
		log.Fatal(err)
	}
	defer jsonFile.Close()
	jsonBytes, _ := ioutil.ReadAll(jsonFile)
	auth := pocket.Auth{}
	if err := json.Unmarshal(jsonBytes, &auth); err != nil {
		log.Fatal(err)
	}
	// Loop forever
	var since int64
	since = 0                        // first iteration retrieves everything since epoch
	sleepInterval := 0 * time.Second // first iteration checks immediately
	for {
		time.Sleep(sleepInterval)
		res, err := auth.Retrieve(pocket.Since(since), pocket.WithTag(defaultTag), pocket.Unread)
		since = res.Since + 1
		sleepInterval = interval // from now on we abide by the requested interval
		if err != nil {
			log.Printf("WARN: %s\n", err)
			continue
		}
		for _, item := range res.Items {
			url := item.ResolvedURL
			if len(url) <= 0 {
				url = item.GivenURL
			}
			fmt.Println(url)
		}
	}
}
