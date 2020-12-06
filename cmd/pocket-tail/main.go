package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"time"
	"github.com/nazavode/rm"
	pocket "github.com/bvp/go-pocket/api"
)

type Auth struct {
	Token       string `json:"token,omitempty"`
	ConsumerKey string `json:"consumer_key,omitempty"`
	User        string `json:"user,omitempty"`
}

const (
	defaultAuthPath string = "~/.pocket"
	defaultTag      string = "rm"
)

type ItemList []pocket.Item

func (s ItemList) Len() int           { return len(s) }
func (s ItemList) Less(i, j int) bool { return s[i].SortId < s[j].SortId }
func (s ItemList) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

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
	auth := Auth{}
	if err := json.Unmarshal(jsonBytes, &auth); err != nil {
		log.Fatal(err)
	}
	pocketClient := pocket.NewClient(auth.ConsumerKey, auth.Token)
	pocketOpts := &pocket.RetrieveOption{
		// Favorite: pocketApi.FavoriteFilterFavorited,
		Tag:   defaultTag,
		State: pocket.StateUnread,
		// Sort:  pocket.SortOldest, // doesn't seem to be working right
	}
	// Loop forever
	pocketOpts.Since = 0 // first iteration retrieves everything since epoch
	sleepInterval := 0 * time.Second // first iteration checks immediately
	for {
		time.Sleep(sleepInterval)
		res, err := pocketClient.Retrieve(pocketOpts)
		pocketOpts.Since = int(time.Now().Unix()) // TODO fix upstream, .Since must be int64
		sleepInterval = interval // from now on we abide by the requested interval
		if err != nil {
			log.Printf("WARN: %s\n", err)
			continue
		}
		if len(res.List) <= 0 {
			continue
		}
		items := []pocket.Item{}
		for _, item := range res.List {
			items = append(items, item)
		}
		sort.Sort(ItemList(items))
		for _, item := range items {
			bytes, err := json.Marshal(&rm.Item{
				Id:  item.ItemID,
				Url: item.ResolvedURL,
			})
			if err != nil {
				log.Printf("WARN: %s\n", err)
				continue
			}
			fmt.Println(string(bytes))
		}
	}
}
