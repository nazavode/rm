package rm

import (
	"time"
	"fmt"
	"net/url"

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
	err := Command(d.Content(), timeout, "pandoc",
		"-o", filename, "-f", d.Format(), "--metadata", fmt.Sprintf("title='%s'", d.Title()))
	if err != nil {
		return err
	}
	return nil
}
