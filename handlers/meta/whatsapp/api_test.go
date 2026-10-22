package whatsapp_test

import (
	"testing"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/handlers/meta/whatsapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSendResponse(t *testing.T) {
	// valid response with a sent message
	resp, err := whatsapp.ParseSendResponse([]byte(`{"messages":[{"id":"wamid.123"}],"contacts":[{"wa_id":"5678","user_id":"US.999"}]}`))
	assert.NoError(t, err)
	assert.Equal(t, "wamid.123", resp.ExternalID())
	assert.Equal(t, "US.999", resp.UserID())

	// null elements in the arrays shouldn't panic
	resp, err = whatsapp.ParseSendResponse([]byte(`{"messages":[null],"contacts":[null]}`))
	assert.NoError(t, err)
	assert.Equal(t, "", resp.ExternalID())
	assert.Equal(t, "", resp.UserID())

	// empty response
	resp, err = whatsapp.ParseSendResponse([]byte(`{}`))
	assert.NoError(t, err)
	assert.Equal(t, "", resp.ExternalID())
	assert.Equal(t, "", resp.UserID())

	// unparseable response
	_, err = whatsapp.ParseSendResponse([]byte(`not json`))
	assert.Equal(t, channels.ErrResponseUnparseable, err)

	// error object responses map to the appropriate send error
	_, err = whatsapp.ParseSendResponse([]byte(`{"error":{"code":130429,"message":"Rate limit hit"}}`))
	assert.Equal(t, channels.ErrConnectionThrottled, err)

	_, err = whatsapp.ParseSendResponse([]byte(`{"error":{"code":131053,"message":"Media upload error"}}`))
	require.Error(t, err)
	assert.Equal(t, channels.ErrRetryableWithReason("131053", "Media upload error"), err)

	_, err = whatsapp.ParseSendResponse([]byte(`{"error":{"code":100,"message":"Invalid parameter"}}`))
	assert.Equal(t, channels.ErrFailedWithReason("100", "Invalid parameter"), err)
}

func TestParseSendResponseErrorDetails(t *testing.T) {
	// generic errors have the useful part in error_data.details
	_, err := whatsapp.ParseSendResponse([]byte(`{"error":{"message":"(#100) Invalid parameter","code":100,"error_data":{"details":"Parameter Invalid: The 'category' value requires Direct Send, which isn't enabled for this account."}}}`))
	require.Error(t, err)
	assert.Equal(t, "channel rejected send with reason", err.Error())
	assert.Equal(t, "(#100) Invalid parameter: Parameter Invalid: The 'category' value requires Direct Send, which isn't enabled for this account.", err.(*channels.SendError).ClogError().Message)
	assert.Equal(t, "100", err.(*channels.SendError).ClogError().ExtCode)
}
