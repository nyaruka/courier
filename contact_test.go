package courier

import (
	"testing"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/stretchr/testify/assert"
)

func TestContact(t *testing.T) {
	UUID := string(uuids.New())
	contactUUID, err := NewContactUUID(UUID)

	assert.Equal(t, UUID, contactUUID.UUID.String())
	assert.NoError(t, err)

	_, err = NewContactUUID(UUID + "1234")
	assert.Error(t, err)
}
