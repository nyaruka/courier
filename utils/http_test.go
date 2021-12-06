package utils

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestClient(t *testing.T) {
	client := GetHTTPClient()
	if client == nil {
		t.Error("Client should not be nil")
	}

	insecureClient := GetInsecureHTTPClient()
	if insecureClient == nil {
		t.Error("Insecure client should not be nil")
	}

	if client == insecureClient || client.Transport == insecureClient.Transport {
		t.Error("Client and insecure client should not be the same")
	}

	client2 := GetHTTPClient()
	if client != client2 {
		t.Error("GetHTTPClient should always return same client")
	}
}

func TestMakeHTTPRequestWithClient(t *testing.T) {
	header := map[string][]string{"Content-Type": {"application/json"} }
	client := GetHTTPClient()
	req := &http.Request{
		Header: header,
		URL: &url.URL{Host: "example.com"},
	}
	_, err := MakeHTTPRequestWithClient(req, client)
	assert.EqualError(t, err, "unsupported protocol scheme \"\"")

	req.URL.Scheme = "https"
	_, err = MakeHTTPRequestWithClient(req, client)
	assert.NoError(t, err)
}

func TestNewRRFromRequestAndError(t *testing.T) {
	errInst := fmt.Errorf("failing request")
	header := map[string][]string{"Content-Type": {"application/json"} }
	req := &http.Request{
		Header: header,
		URL: &url.URL{Host: "example.com"},
	}

	_, err := newRRFromRequestAndError(req, "", errInst)
	assert.NoError(t, err)
}

func TestNewRRFromResponse(t *testing.T) {
	header := map[string][]string{"Content-Type": {"application/json"} }
	stringReader := strings.NewReader("shiny!")
	stringReadCloser := ioutil.NopCloser(stringReader)
	req := &http.Request{
		URL: &url.URL{},
	}
	httpResponse := &http.Response{StatusCode: 200, Header: header, ContentLength: 0, Request: req, Body: stringReadCloser}
	_, err := newRRFromResponse("GET", "", httpResponse)
	assert.NoError(t, err)

	httpResponse.StatusCode = 400
	_, err = newRRFromResponse("GET", "", httpResponse)
	assert.EqualError(t, err, "received non 200 status: 400")
}
