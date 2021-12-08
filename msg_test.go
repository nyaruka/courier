package courier

import (
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

func TestNewMsgID(t *testing.T) {
	id := int64(200)
	msgID := NewMsgID(id)
	assert.NotEqual(t, msgID, id)
	assert.Equal(t, MsgID(id), msgID)

	expectedID := []byte(strconv.FormatInt(id, 10))
	idJSON, err := msgID.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, expectedID, idJSON)

	err = msgID.UnmarshalJSON([]byte("{}"))
	assert.Error(t, err)

	value, err := msgID.Value()
	assert.NoError(t, err)
	assert.Equal(t, id, value)

	err = msgID.Scan("")
	assert.EqualError(t, err, "converting driver.Value type string (\"\") to a int64: invalid syntax")
}

func TestNewMsgUUID(t *testing.T) {
	msgUUID := NewMsgUUID()

	msgStr := msgUUID.String()
	msgUUID2 := NewMsgUUIDFromString(msgStr)

	assert.Equal(t, msgUUID, msgUUID2)
}
