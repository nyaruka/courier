package courier_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/test"
	"github.com/stretchr/testify/assert"
)

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	err := courier.WriteError(w, 406, errors.New("boom"))
	assert.NoError(t, err)
	assert.Equal(t, 406, w.Code)
	assert.Equal(t, "{\"message\":\"Error\",\"data\":[{\"type\":\"error\",\"error\":\"boom\"}]}\n", w.Body.String())
}

func TestWriteIgnored(t *testing.T) {
	w := httptest.NewRecorder()

	err := courier.WriteIgnored(w, "why you calling")
	assert.NoError(t, err)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "{\"message\":\"Ignored\",\"data\":[{\"type\":\"info\",\"info\":\"why you calling\"}]}\n", w.Body.String())
}

func TestWriteAndLogUnauthorized(t *testing.T) {
	ch := test.NewMockChannel("5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec", "NX", "+1234567890", "US", nil)
	r, _ := http.NewRequest("GET", "http://example.com", nil)
	w := httptest.NewRecorder()

	err := courier.WriteAndLogUnauthorized(w, r, ch, errors.New("wrong password"))
	assert.NoError(t, err)
	assert.Equal(t, 401, w.Code)
	assert.Equal(t, "{\"message\":\"Unauthorized\",\"data\":[{\"type\":\"error\",\"error\":\"wrong password\"}]}\n", w.Body.String())
}

func TestWriteMsgSuccess(t *testing.T) {
	ch := test.NewMockChannel("5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec", "NX", "+1234567890", "US", nil)
	msg := test.NewMockBackend().NewIncomingMsg(ch, "tel:+0987654321", "hi there", "", nil).(*test.MockMsg).WithUUID("588aafc4-ab5c-48ce-89e8-05c9fdeeafb7")
	w := httptest.NewRecorder()

	err := courier.WriteMsgSuccess(w, []courier.MsgIn{msg.(courier.MsgIn)})
	assert.NoError(t, err)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "{\"message\":\"Message Accepted\",\"data\":[{\"type\":\"msg\",\"channel_uuid\":\"5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec\",\"msg_uuid\":\"588aafc4-ab5c-48ce-89e8-05c9fdeeafb7\",\"text\":\"hi there\",\"urn\":\"tel:+0987654321\"}]}\n", w.Body.String())
}

func TestWriteChannelEventSuccess(t *testing.T) {
	ch := test.NewMockChannel("5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec", "NX", "+1234567890", "US", nil)
	evt := test.NewMockBackend().NewChannelEvent(ch, courier.EventTypeStopContact, "tel:+0987654321", nil).WithOccurredOn(time.Date(2022, 9, 15, 12, 7, 30, 0, time.UTC))
	w := httptest.NewRecorder()

	err := courier.WriteChannelEventSuccess(w, evt)
	assert.NoError(t, err)
	assert.Equal(t, 200, w.Code)
	assert.Equal(t, "{\"message\":\"Event Accepted\",\"data\":[{\"type\":\"event\",\"channel_uuid\":\"5fccf4b6-48d7-4f5a-bce8-b0d1fd5342ec\",\"event_type\":\"stop_contact\",\"urn\":\"tel:+0987654321\",\"received_on\":\"2022-09-15T12:07:30Z\"}]}\n", w.Body.String())
}
