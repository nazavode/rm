package pocket

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"time"
)

type retrieveOptions struct {
	State       string `json:"state,omitempty"`
	Favorite    uint8  `json:"favorite,omitempty"`
	Tag         string `json:"tag,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Sort        string `json:"sort,omitempty"`
	DetailType  string `json:"detailType,omitempty"`
	Search      string `json:"search,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Since       int64  `json:"since,omitempty"`
	Count       int64  `json:"count,omitempty"`
	Offset      int64  `json:"offset,omitempty"`
}

type item struct {
	ItemID        int    `json:"item_id,string"`
	ResolvedID    int    `json:"resolved_id,string"`
	GivenURL      string `json:"given_url"`
	ResolvedURL   string `json:"resolved_url"`
	GivenTitle    string `json:"given_title"`
	ResolvedTitle string `json:"resolved_title"`
	Favorite      string `json:"favorite"`
	Status        string `json:"status"`
	SortID        int    `json:"sort_id"`
}

func NewRetrieveOptions(opts ...RetrieveOpt) *retrieveOptions {
	c := &retrieveOptions{
		ContentType: "article",
		Sort:        "oldest",
		DetailType:  "simple",
	}
	for _, f := range opts {
		f(c)
	}
	return c
}

type RetrieveOpt func(*retrieveOptions)

func Since(since int64) RetrieveOpt {
	return func(c *retrieveOptions) {
		c.Since = since
	}
}

func WithTag(tag string) RetrieveOpt {
	return func(c *retrieveOptions) {
		c.Tag = tag
	}
}

func Unread(c *retrieveOptions) {
	c.State = "unread"
}

func Archived(c *retrieveOptions) {
	c.State = "archive"
}

func All(c *retrieveOptions) {
	c.State = "all"
}

type retrievePayload struct {
	*Auth
	*retrieveOptions
}

type RetrieveResultMeta struct {
	Since int64 `json:"since,omitempty"`
}

type RetrieveResultItems struct {
	Items []item `json:"list"`
}

type apiRetrieveResultItems struct {
	Items map[string]item `json:"list"`
}

type apiRetrieveResult struct {
	apiRetrieveResultItems
	RetrieveResultMeta
}

type RetrieveResult struct {
	RetrieveResultItems
	RetrieveResultMeta
}

func (r *apiRetrieveResult) UnmarshalJSON(data []byte) error {
	// Unmarshall metadata
	if err := json.Unmarshal(data, &r.RetrieveResultMeta); err != nil {
		return err
	}
	// Unmarshal items
	itemsError := json.Unmarshal(data, &r.apiRetrieveResultItems)
	if itemsError == nil {
		return nil
	}
	// If we get an error here, we could be in two different
	// situations:
	// Case 1. 'list' is empty, so the parser finds an empty array
	//         instead of a map: no error
	// Case 2. 'list' is actually something unknown: error
	var dummy struct {
		List interface{} `json:"list"`
	}
	if err := json.Unmarshal(data, &dummy); err != nil {
		return err
	}
	switch dv := dummy.List.(type) {
	case []interface{}:
		if len(dv) == 0 {
			// Case 1
			r.Items = make(map[string]item) // enforce an empty result set
			return nil
		}
	}
	// Everything else is considered Case 2
	return fmt.Errorf("unexpected json type for apiRetrieveResult")
}

func (a *Auth) Retrieve(conf *retrieveOptions) (*RetrieveResult, error) {
	args := retrievePayload{a, conf}
	res := &apiRetrieveResult{}
	err := postJSON("/v3/get", args, &res)
	if err != nil {
		return nil, err
	}
	// Unpack and sort results
	items := []item{}
	for _, v := range res.Items {
		items = append(items, v)
	}
	sort.Sort(itemList(items))
	urls := []*url.URL{}
	for _, item := range items {
		result, err := url.Parse(item.ResolvedURL)
		if err != nil {
			return nil, err
		}
		urls = append(urls, result)
	}
	ret := &RetrieveResult{}
	ret.Items = items
	ret.RetrieveResultMeta = res.RetrieveResultMeta
	return ret, nil
}

func (a *Auth) Tail(conf *retrieveOptions, tick <-chan time.Time, done <-chan bool) <-chan interface{} {
	out := make(chan interface{}, 1)
	go func() {
		defer close(out)
		for {
			select {
			case <-done:
				return
			case <-tick:
				res, err := a.Retrieve(conf)
				if err != nil {
					out <- err
					continue
				}
				conf.Since = res.Since + 1
				for _, item := range res.Items {
					itemURL := item.ResolvedURL
					if len(itemURL) <= 0 {
						itemURL = item.GivenURL
					}
					itemResult, err := url.Parse(itemURL)
					if err != nil {
						out <- err
						continue
					}
					out <- itemResult
				}
			}
		}
	}()
	return out
}

type itemList []item

func (s itemList) Len() int           { return len(s) }
func (s itemList) Less(i, j int) bool { return s[i].SortID < s[j].SortID }
func (s itemList) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
