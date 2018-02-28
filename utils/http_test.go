package utils

import "testing"

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
