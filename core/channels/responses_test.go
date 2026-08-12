package channels_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
)

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	err := channels.WriteError(w, 406, errors.New("boom"))
	assert.NoError(t, err)
	assert.Equal(t, 406, w.Code)
	assert.Equal(t, "{\"message\":\"Error\",\"data\":[{\"type\":\"error\",\"error\":\"boom\"}]}\n", w.Body.String())
}

func TestWriteIgnored(t *testing.T) {
	w := httptest.NewRecorder()

	err := channels.WriteIgnored(w, "why you calling")
	assert.NoError(t, err)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "{\"message\":\"Ignored\",\"data\":[{\"type\":\"info\",\"info\":\"why you calling\"}]}\n", w.Body.String())
}

func TestWriteAndLogUnauthorized(t *testing.T) {
	ch := test.NewMockChannel("5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec", "NX", "+1234567890", "US", []string{urns.Phone.Prefix}, nil)
	r, _ := http.NewRequest("GET", "http://example.com", nil)
	w := httptest.NewRecorder()

	err := channels.WriteAndLogUnauthorized(w, r, ch, errors.New("wrong password"))
	assert.NoError(t, err)
	assert.Equal(t, 401, w.Code)
	assert.Equal(t, "{\"message\":\"Unauthorized\",\"data\":[{\"type\":\"error\",\"error\":\"wrong password\"}]}\n", w.Body.String())
}

func TestWriteMsgSuccess(t *testing.T) {
	ch := test.NewMockChannel("5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec", "NX", "+1234567890", "US", []string{urns.Phone.Prefix}, nil)
	msg := &models.MsgIn{
		UUID_:        "588aafc4-ab5c-48ce-89e8-05c9fdeeafb7",
		ChannelUUID_: ch.UUID(),
		URN_:         "tel:+0987654321",
		Text_:        "hi there",
		Channel_:     ch,
	}
	w := httptest.NewRecorder()

	err := channels.WriteMsgSuccess(w, []*models.MsgIn{msg})
	assert.NoError(t, err)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "{\"message\":\"Message Accepted\",\"data\":[{\"type\":\"msg\",\"channel_uuid\":\"5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec\",\"msg_uuid\":\"588aafc4-ab5c-48ce-89e8-05c9fdeeafb7\",\"text\":\"hi there\",\"urn\":\"tel:+0987654321\"}]}\n", w.Body.String())
}

func TestWriteChannelEventSuccess(t *testing.T) {
	ch := test.NewMockChannel("5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec", "NX", "+1234567890", "US", []string{urns.Phone.Prefix}, nil)
	evt := &models.ChannelEvent{
		UUID_:        "0199df03-621a-7e52-a6b0-7086c8b1a86a",
		ChannelUUID_: ch.UUID(),
		EventType_:   models.EventTypeStopContact,
		URN_:         "tel:+0987654321",
		OccurredOn_:  time.Date(2022, 9, 15, 12, 7, 30, 0, time.UTC),
		Channel_:     ch,
	}
	w := httptest.NewRecorder()

	err := channels.WriteChannelEventSuccess(w, evt)
	assert.NoError(t, err)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "{\"message\":\"Event Accepted\",\"data\":[{\"type\":\"event\",\"channel_uuid\":\"5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec\",\"event_type\":\"stop_contact\",\"urn\":\"tel:+0987654321\",\"received_on\":\"2022-09-15T12:07:30Z\"}]}\n", w.Body.String())
}
