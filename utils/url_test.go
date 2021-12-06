package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAddURLPath(t *testing.T)  {
	url, err := AddURLPath("", "")

	assert.NoError(t, err)
	assert.Equal(t, "", url)

	url, err = AddURLPath("<>--", "&&&&")

	assert.NoError(t, err)
	assert.Equal(t, "/%3C%3E--/&&&&", url)

	url, err = AddURLPath("://name.com", "fake-path")
	assert.EqualError(t,err, "parse \"://name.com\": missing protocol scheme")
}
