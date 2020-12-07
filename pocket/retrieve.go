package pocket

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
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

func NewRetrieveOptions() *retrieveOptions {
	return &retrieveOptions{
		ContentType: "article",
		Sort:        "oldest",
		DetailType:  "simple",
	}
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
	// Excerpt       string
	// IsArticle     int    `json:"is_article,string"`
	// HasImage      string `json:"has_image,string"`
	// HasVideo      string `json:"has_video,string"`
	// WordCount     int    `json:"word_count,string"`

	// // Fields for detailed response
	// Tags    map[string]map[string]interface{}
	// Authors map[string]map[string]interface{}
	// Images  map[string]map[string]interface{}
	// Videos  map[string]map[string]interface{}

	// Fields that are not documented but exist
	SortId int `json:"sort_id"`
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
	return fmt.Errorf("unexpected json type for retrieveResult")
}

func (a *Auth) Retrieve(opts ...RetrieveOpt) (*RetrieveResult, error) {
	conf := NewRetrieveOptions()
	for _, f := range opts {
		f(conf)
	}
	// Do API call
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

type itemList []item

func (s itemList) Len() int           { return len(s) }
func (s itemList) Less(i, j int) bool { return s[i].SortId < s[j].SortId }
func (s itemList) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
