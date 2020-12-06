package pocket

import (
	pocketApi "github.com/bvp/go-pocket/api"
	"github.com/nazavode/rm"
	"sort"
	"time"
)

type Auth struct {
	Token       string `json:"token,omitempty"`
	ConsumerKey string `json:"consumer_key,omitempty"`
}

type itemList []pocketApi.Item

func (s itemList) Len() int           { return len(s) }
func (s itemList) Less(i, j int) bool { return s[i].SortId < s[j].SortId }
func (s itemList) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type Connection struct {
	conn *pocketApi.Client
}

func NewConnection(auth Auth) (*Connection, error) {
	return &Connection{
		conn: pocketApi.NewClient(auth.ConsumerKey, auth.Token)}, nil
}

type RetrieveConfig struct {
	apiConfig pocketApi.RetrieveOption
}

type RetrieveOpt func(*RetrieveConfig)

func WithEpoch(since int64) RetrieveOpt {
	return func(c *RetrieveConfig) {
		c.apiConfig.Since = int(since) // TODO fix upstream, since should be int64
	}
}

func WithTag(tag string) RetrieveOpt {
	return func(c *RetrieveConfig) {
		c.apiConfig.Tag = tag
	}
}

func WithUnreadState(c *RetrieveConfig) {
	c.apiConfig.State = pocketApi.StateUnread
}

func (c *Connection) Retrieve(opts ...RetrieveOpt) ([]string, error) {
	conf := &RetrieveConfig{}
	for _, f := range opts {
		f(conf)
	}
	res, err := c.conn.Retrieve(&conf.apiConfig)
	if err != nil {
		return nil, err
	}
	items := []pocketApi.Item{}
	for _, item := range res.List {
		items = append(items, item)
	}
	sort.Sort(itemList(items))
	urls := []string{}
	for _, item := range items {
		urls = append(urls, item.ResolvedURL)
	}
	return urls, nil
}
