package handlers

import (
	"fmt"
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
)

// RequestHTTP does the given request, logging the trace, and returns the response
func RequestHTTP(req *http.Request, clog *courier.ChannelLog) (*http.Response, []byte, error) {
	return RequestHTTPWithClient(utils.GetHTTPClient(), req, clog)
}

// RequestHTTPInsecure does the given request using an insecure client that does not validate SSL certificates,
// logging the trace, and returns the response
func RequestHTTPInsecure(req *http.Request, clog *courier.ChannelLog) (*http.Response, []byte, error) {
	return RequestHTTPWithClient(utils.GetInsecureHTTPClient(), req, clog)
}

// RequestHTTP does the given request using the given client, logging the trace, and returns the response
func RequestHTTPWithClient(client *http.Client, req *http.Request, clog *courier.ChannelLog) (*http.Response, []byte, error) {
	var resp *http.Response
	var body []byte

	trace, err := httpx.DoTrace(client, req, nil, nil, 0)
	if trace != nil {
		clog.HTTP(trace)
		resp = trace.Response
		body = trace.ResponseBody
	}
	if err != nil {
		return nil, nil, err
	}

	return resp, body, nil
}

// Deprecated: use RequestHTTP instead
func MakeHTTPRequest(req *http.Request) (*httpx.Trace, error) {
	trace, err := httpx.DoTrace(utils.GetHTTPClient(), req, nil, nil, 0)
	if err != nil {
		return trace, err
	}

	// return an error if we got a non-200 status
	if trace.Response != nil && trace.Response.StatusCode/100 != 2 {
		return trace, fmt.Errorf("received non 200 status: %d", trace.Response.StatusCode)
	}

	return trace, nil
}
