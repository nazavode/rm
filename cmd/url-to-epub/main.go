package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"sync"
	"time"

	readability "github.com/go-shiori/go-readability"
	"github.com/kennygrant/sanitize"
	"github.com/nazavode/rm"
)

const (
	defaultRetrievalTimeout  time.Duration = 30 * time.Second
	defaultConversionTimeout time.Duration = 10 * time.Second
	defaultConverter         string        = "pandoc"
)

func init() {
	_, err := exec.LookPath(defaultConverter)
	if err != nil {
		log.Fatal(err)
	}
}

func makeTitle(article *readability.Article) string {
	source := article.Title
	if len(source) <= 0 {
		source = article.SiteName
	}
	if len(source) <= 0 {
		source = "No Title"
	}
	return sanitize.HTML(source)
}

func makeSlug(article *readability.Article) string {
	source := article.Title
	if len(source) <= 0 {
		source = article.SiteName
	}
	if len(source) <= 0 {
		source = "No Title"
	}
	return sanitize.Name(source)
}

func item2epub(item rm.Item, dirname string, retrieveTimeout time.Duration, convertTimeout time.Duration) (string, error) {
	article, err := readability.FromURL(item.Url, retrieveTimeout)
	if err != nil {
		return "", err
	}
	title := makeTitle(&article)
	basename := fmt.Sprintf("%d-%s.epub", item.Id, makeSlug(&article))
	outPath := path.Join(dirname, basename)
	err = command(defaultConverter, article.Content, convertTimeout,
		"-o", outPath, "-f", "html", "--metadata", fmt.Sprintf("title=%s", title))
	if err != nil {
		return "", err
	}
	return outPath, nil
}

func worker(item rm.Item, dirname string, log *log.Logger, wg *sync.WaitGroup) {
	defer wg.Done()
	outPath, err := item2epub(item, dirname, defaultRetrievalTimeout, defaultConversionTimeout)
	if err != nil {
		log.Println("WARN: ", err)
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
	scanner := bufio.NewScanner(os.Stdin)
	var wg sync.WaitGroup
	for scanner.Scan() {
		item := &rm.Item{}
		if err := json.Unmarshal(scanner.Bytes(), item); err != nil {
			log.Printf("WARN: %s\n", err)
			continue
		}
		wg.Add(1)
		go worker(*item, outDir, log, &wg)
	}
	wg.Wait()
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
