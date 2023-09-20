package whatsapp_test

import (
	"encoding/json"
	"testing"

	"github.com/nyaruka/courier/handlers/meta/whatsapp"
	"github.com/nyaruka/courier/test"
	"github.com/stretchr/testify/assert"
)

func TestGetTemplating(t *testing.T) {
	msg := test.NewMockMsg(1, "87995844-2017-4ba0-bc73-f3da75b32f9b", nil, "tel:+1234567890", "hi", nil)

	// no metadata, no templating
	tpl, err := whatsapp.GetTemplating(msg)
	assert.NoError(t, err)
	assert.Nil(t, tpl)

	msg.WithMetadata(json.RawMessage(`{}`))

	// no templating in metadata, no templating
	tpl, err = whatsapp.GetTemplating(msg)
	assert.NoError(t, err)
	assert.Nil(t, tpl)

	msg.WithMetadata(json.RawMessage(`{"templating": {"foo": "bar"}}`))

	// invalid templating in metadata, error
	tpl, err = whatsapp.GetTemplating(msg)
	assert.Error(t, err, "x")
	assert.Nil(t, tpl)

	msg.WithMetadata(json.RawMessage(`{"templating": {"template": {"uuid": "4ed5000f-5c94-4143-9697-b7cbd230a381", "name": "Update"}}}`))

	// invalid templating in metadata, error
	tpl, err = whatsapp.GetTemplating(msg)
	assert.NoError(t, err)
	assert.Equal(t, "4ed5000f-5c94-4143-9697-b7cbd230a381", tpl.Template.UUID)
	assert.Equal(t, "Update", tpl.Template.Name)
}
