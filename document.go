package rm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"time"

	readability "github.com/go-shiori/go-readability"
	"github.com/kennygrant/sanitize"
)

type Document interface {
	Slug() string
	Title() string
	Content() string
	Format() string
}

type htmlDocument struct {
	article readability.Article
}

func (h *htmlDocument) Slug() string {
	source := h.article.Title
	if len(source) <= 0 {
		source = h.article.SiteName
	}
	if len(source) <= 0 {
		source = "Untitled"
	}
	return sanitize.Name(source)
}

func (h *htmlDocument) Title() string {
	source := h.article.Title
	if len(source) <= 0 {
		source = h.article.SiteName
	}
	if len(source) <= 0 {
		source = "Untitled"
	}
	return sanitize.HTML(source)
}

func (h *htmlDocument) Format() string {
	return "html"
}

func (h *htmlDocument) Content() string {
	return h.article.Content
}

func Retrieve(target *url.URL, timeout time.Duration) (Document, error) {
	article, err := readability.FromURL(target.String(), timeout)
	if err != nil {
		return nil, err
	}
	return &htmlDocument{article: article}, nil
}

func DocumentToEPUB(d Document, filename string, timeout time.Duration) error {
	var meta struct {
		Title string `json:"title"`
	}
	meta.Title = d.Title()
	metafile, err := ioutil.TempFile("", "epub.*.json")
	if err != nil {
		return fmt.Errorf("cannot create epub metadata temporary file: %w", err)
	}
	defer os.Remove(metafile.Name())
	metaContent, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("cannot marshal epub metadata: %s", err)
	}
	if err := ioutil.WriteFile(metafile.Name(), metaContent, 0644); err != nil {
		return fmt.Errorf("cannot write epub metadata temporary file: %w", err)
	}
	err = command(d.Content(), timeout, "pandoc",
		"-o", filename, "-f", d.Format(), "--metadata-file", metafile.Name())
	return err
}

func command(toStdin string, timeout time.Duration, exe string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := io.WriteString(stdin, toStdin); err != nil {
		return err
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		return err
	}
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timed out (> %s): %s", timeout, cmd)
	}
	return nil
}
