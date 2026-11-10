package yo

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

var testChannel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "YO", "2020", "UG", []string{urns.Phone.Prefix}, map[string]any{"username": "yo-username", "password": "yo-password"})

func TestIncoming(t *testing.T) {
	RunIncomingTests(t, []*models.Channel{testChannel}, newHandler, "testdata/incoming.json", nil)
}

func TestOutgoing(t *testing.T) {
	RunOutgoingTests(t, testChannel, newHandler, "testdata/outgoing.json", &OutgoingOptions{CheckRedacted: []string{"yo-password"}})
}

func TestIsUnreachable(t *testing.T) {
	dial := &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	read := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")}

	assert.True(t, isUnreachable(&url.Error{Op: "Get", URL: "http://x", Err: dial}))
	assert.True(t, isUnreachable(&url.Error{Op: "Get", URL: "http://x", Err: &net.DNSError{Err: "no such host"}}))
	assert.False(t, isUnreachable(&url.Error{Op: "Get", URL: "http://x", Err: read}))
	assert.False(t, isUnreachable(&url.Error{Op: "Get", URL: "http://x", Err: context.DeadlineExceeded}))
	assert.False(t, isUnreachable(nil))
}
