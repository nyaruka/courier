package handlers

import (
	"fmt"
	"net/http"

	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
)

// MakeHTTPRequest makes the given request and returns the trace
func MakeHTTPRequest(req *http.Request) (*httpx.Trace, error) {
	return MakeHTTPRequestWithClient(utils.GetHTTPClient(), req)
}

// MakeInsecureHTTPRequest makes the given request using an insecure client that does not validate
// SSL certificates, and returns the trace
func MakeInsecureHTTPRequest(req *http.Request) (*httpx.Trace, error) {
	return MakeHTTPRequestWithClient(utils.GetInsecureHTTPClient(), req)
}

// MakeHTTPRequestWithClient makes the given request using the given client, and returns the trace
func MakeHTTPRequestWithClient(client *http.Client, req *http.Request) (*httpx.Trace, error) {
	trace, err := httpx.DoTrace(client, req, nil, nil, 0)
	if err != nil {
		return nil, err
	}

	// return an error if we got a non-200 status
	if trace.Response != nil && trace.Response.StatusCode/100 != 2 {
		return trace, fmt.Errorf("received non 200 status: %d", trace.Response.StatusCode)
	}

	return trace, nil
}
