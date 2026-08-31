package whatsapp_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseChanges runs the shared parser over the given changes JSON, with a media resolver that resolves every
// id to the same URL, and returns the batch it filled in along with any error.
func parseChanges(t *testing.T, changesJSON string) (*channels.Received, error) {
	t.Helper()

	ch := test.NewMockChannel("dbc126ed-66bc-4e28-b67b-81dc3327c95d", "WAC", "12345", "US", []string{urns.WhatsApp.Prefix}, nil)
	clog := models.NewChannelLog(models.ChannelLogTypeReceive, ch, nil, nil)
	in := channels.NewReceived(ch)
	r := httptest.NewRequest("POST", "/c/wac/dbc126ed-66bc-4e28-b67b-81dc3327c95d/receive", nil)

	var changes []whatsapp.Change
	require.NoError(t, json.Unmarshal([]byte(changesJSON), &changes))

	resolveMedia := func(mediaID string) (string, error) { return "https://example.com/" + mediaID + ".jpg", nil }

	return in, whatsapp.ParseChanges(ch, changes, resolveMedia, r, in, clog)
}

func TestParseChanges(t *testing.T) {
	// a message and a status from one change both land in the batch
	in, err := parseChanges(t, `[{"field":"messages","value":{
		"contacts":[{"profile":{"name":"Jim"},"wa_id":"250788123123"}],
		"messages":[{"id":"wamid.1","from":"250788123123","timestamp":"1454119029","type":"text","text":{"body":"hello"}}],
		"statuses":[{"id":"wamid.9","status":"sent","timestamp":"1454119029"}]
	}}]`)
	assert.NoError(t, err)
	assert.Equal(t, 2, in.Len())

	// the same message id twice is only taken once, including across changes
	in, err = parseChanges(t, `[{"field":"messages","value":{
		"messages":[{"id":"wamid.1","from":"250788123123","timestamp":"1454119029","type":"text","text":{"body":"hello"}}]
	}},{"field":"messages","value":{
		"messages":[{"id":"wamid.1","from":"250788123123","timestamp":"1454119029","type":"text","text":{"body":"hello"}}]
	}}]`)
	assert.NoError(t, err)
	assert.Equal(t, 1, in.Len())

	// a group message is noted as ignored rather than received
	in, err = parseChanges(t, `[{"field":"messages","value":{
		"messages":[{"id":"wamid.1","from":"250788123123","group_id":"g1","timestamp":"1454119029","type":"text","text":{"body":"hello"}}]
	}}]`)
	assert.NoError(t, err)
	assert.Equal(t, 1, in.Len())

	// a status we deliberately ignore, and one we don't recognize at all, are both noted as ignored
	in, err = parseChanges(t, `[{"field":"messages","value":{
		"statuses":[{"id":"wamid.9","status":"deleted","timestamp":"1454119029"},{"id":"wamid.8","status":"in_orbit","timestamp":"1454119029"}]
	}}]`)
	assert.NoError(t, err)
	assert.Equal(t, 2, in.Len())

	// a message whose timestamp can't be read fails the whole batch, because asking for it again wouldn't get
	// any further - this is the error each handler answers in the way its provider needs
	_, err = parseChanges(t, `[{"field":"messages","value":{
		"messages":[{"id":"wamid.1","from":"250788123123","timestamp":"notatimestamp","type":"text","text":{"body":"hello"}}]
	}}]`)
	assert.EqualError(t, err, "invalid timestamp: notatimestamp")

	// as does one we can't read a sender from
	_, err = parseChanges(t, `[{"field":"messages","value":{
		"messages":[{"id":"wamid.1","timestamp":"1454119029","type":"text","text":{"body":"hello"}}]
	}}]`)
	assert.EqualError(t, err, "invalid whatsapp id")
}
