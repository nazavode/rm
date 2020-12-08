package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"sync"
	"time"
	"net/url"

	"github.com/nazavode/rm"
)

const (
	defaultRetrievalTimeout  time.Duration = 30 * time.Second
	defaultConversionTimeout time.Duration = 10 * time.Second
)

func init() {
	_, err := exec.LookPath("pandoc")
	if err != nil {
		log.Fatal(err)
	}
}

func worker(item *url.URL, dirname string, log *log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	doc, err := rm.Retrieve(item, defaultRetrievalTimeout)
	if err != nil {
		log.Println("WARN: ", err)
		return
	}
	basename := fmt.Sprintf("%s.epub", doc.Slug())
	outPath := path.Join(dirname, basename)
	err = rm.DocumentToEPUB(doc, outPath, defaultConversionTimeout)
	if err != nil {
		log.Println("WARN: ", err)
		return
	}
	fmt.Println(outPath)
}

func main() {
	var log = log.New(os.Stderr, "", 0)
	outDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	flag.StringVar(&outDir, "out", outDir, "Directory for generated EPUB files")
	flag.Parse()
	os.MkdirAll(outDir, os.ModePerm)
	scanner := bufio.NewScanner(os.Stdin)
	var wg sync.WaitGroup
	for scanner.Scan() {
		item, err := url.Parse(string(scanner.Bytes()))
		if err != nil {
			log.Printf("WARN: %s\n", err)
			continue
		}
		wg.Add(1)
		go worker(item, outDir, log, &wg)
	}
	wg.Wait()
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
