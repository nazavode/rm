package pocket

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Auth struct {
	ConsumerKey string `json:"consumer_key"`
	AccessToken string `json:"access_token"`
}

func doJSON(req *http.Request, res interface{}) error {
	req.Header.Add("X-Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("got response %d; X-Error=[%s]", resp.StatusCode, resp.Header.Get("X-Error"))
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(res)
}

func postJSON(action string, data, res interface{}) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", "https://getpocket.com"+action, bytes.NewReader(body))
	if err != nil {
		return err
	}
	return doJSON(req, res)
}
