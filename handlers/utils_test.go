package handlers

import (
	"github.com/nyaruka/gocommon/urns"
	"github.com/stretchr/testify/assert"
	"testing"
)


func TestSplitAttachment(t *testing.T) {
	split1, split2 := SplitAttachment("test")
	assert.Equal(t, "", split1)
	assert.Equal(t, "test", split2)

	split1, split2 = SplitAttachment("test:last")
	assert.Equal(t, "test", split1)
	assert.Equal(t, "last", split2)
}

func TestNameFromFirstLastUsername(t *testing.T) {
	name := NameFromFirstLastUsername("John", "Doe", "nass101")
	assert.Equal(t, "John Doe", name)

	name = NameFromFirstLastUsername("", "Doe", "nass101")
	assert.Equal(t, "Doe", name)

	name = NameFromFirstLastUsername("", "", "nass101")
	assert.Equal(t, "nass101", name)

	name = NameFromFirstLastUsername("", "", "")
	assert.Equal(t, "", name)
}

func TestStrictTelForCountry(t *testing.T) {
	actualNumber := "08067886565"
	urn, err := StrictTelForCountry("", "NG")
	assert.EqualError(t, err, "scheme or path cannot be empty")
	assert.Equal(t, urns.NilURN, urn)

	urn, err = StrictTelForCountry(actualNumber, "NG")
	assert.Equal(t, urns.URN("tel:+2348067886565"), urn)
}
