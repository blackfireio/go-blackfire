package blackfire

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

type Profile struct {
	UUID      string
	URL       string
	APIURL    string
	Title     string   `json:"label"`
	CreatedAt BFTime   `json:"created_at"`
	Status    Status   `json:"status"`
	Envelope  Envelope `json:"envelope"`
	Links     linksMap `json:"_links"`

	retries int
	loaded  bool
}

type Envelope struct {
	Ct  int `json:"ct"`
	CPU int `json:"cpu"`
	MU  int `json:"mu"`
	PMU int `json:"pmu"`
}

type Status struct {
	Name          string `json:"name"`
	Code          int    `json:"code"`
	FailureReason string `json:"failure_reason"`
}

type BFTime struct {
	time.Time
}

func (m *BFTime) UnmarshalJSON(b []byte) (err error) {
	s := string(b)
	// Get rid of the quotes "" around the value.
	s = s[1 : len(s)-1]
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.999999999Z0700", s)
	}
	m.Time = t
	return
}

func (p *Profile) load(auth string) error {
	if p.loaded {
		return nil
	}
	p.retries += 1
	if p.retries > 60 {
		p.Status = Status{Name: "errored"}
		p.loaded = true
		return nil
	}
	request, err := http.NewRequest("GET", p.APIURL, nil)
	if err != nil {
		return err
	}
	request.Header.Add("Authorization", auth)
	client := http.DefaultClient
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode == 404 {
		p.Status = Status{Name: "queued"}
		return nil
	}
	if response.StatusCode >= 400 {
		p.Status = Status{Name: "errored"}
		p.loaded = true
		return nil
	}
	responseData, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(responseData, &p); err != nil {
		return fmt.Errorf("JSON error: %v", err)
	}

	if p.Status.Code > 0 {
		p.loaded = true
	}
	return nil
}
