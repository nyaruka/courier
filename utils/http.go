package utils

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"
)

// GetHTTPClient returns the shared HTTP client used by all Courier threads
func GetHTTPClient() *http.Client {
	once.Do(func() {
		transport = http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConns = 64
		transport.MaxIdleConnsPerHost = 8
		transport.IdleConnTimeout = 15 * time.Second
		client = &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
		}
	})

	return client
}

// GetInsecureHTTPClient returns the shared HTTP client used by all Courier threads
func GetInsecureHTTPClient() *http.Client {
	insecureOnce.Do(func() {
		insecureTransport = http.DefaultTransport.(*http.Transport).Clone()
		insecureTransport.MaxIdleConns = 64
		insecureTransport.MaxIdleConnsPerHost = 8
		insecureTransport.IdleConnTimeout = 15 * time.Second
		insecureTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		insecureClient = &http.Client{
			Transport: insecureTransport,
			Timeout:   60 * time.Second,
		}
	})

	return insecureClient
}

var (
	transport *http.Transport
	client    *http.Client
	once      sync.Once

	insecureTransport *http.Transport
	insecureClient    *http.Client
	insecureOnce      sync.Once

	HTTPUserAgent = "Courier/vDev"
)
