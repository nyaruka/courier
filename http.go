package courier

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"
)

// MakeHTTPRequest fires the passed in http request using our shared client, returning the response and any errors
func MakeHTTPRequest(req *http.Request) (*http.Response, []byte, error) {
	resp, err := getClient().Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	// detect a non-20* response and throw
	if resp.StatusCode/100 != 2 {
		return nil, nil, fmt.Errorf("got non 200 status (%d) from request", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	return resp, body, err
}

var (
	transport *http.Transport
	client    *http.Client
	once      sync.Once
)

func getClient() *http.Client {
	once.Do(func() {
		timeout := time.Duration(30 * time.Second)
		transport = &http.Transport{
			MaxIdleConns:    10,
			IdleConnTimeout: 30 * time.Second,
		}
		client = &http.Client{Transport: transport, Timeout: timeout}
	})

	return client
}
