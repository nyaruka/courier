package gsm7

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecode(t *testing.T) {
	tcs := []struct {
		encoded string
		decoded string
	}{
		{"basic", "basic"},
		{"\x00\x0Fspecial", "@åspecial"},
		{"\x1B\x28extended\x1B\x29", "{extended}"},
		{"\x20space", " space"},
	}
	for _, tc := range tcs {
		assert.Equal(t, tc.decoded, Decode([]byte(tc.encoded)))
		assert.Equal(t, []byte(tc.encoded), Encode(tc.decoded))
	}

	assert.Equal(t, "?invalid?", Decode([]byte("\x1B\x50invalid\x1B\x50")))
	assert.Equal(t, "?toobig", Decode([]byte("\x81toobig")))

	assert.Equal(t, []byte("hi!\x20\x3F"), Encode("hi! ☺"))
}

func TestValid(t *testing.T) {
	tcs := []struct {
		str   string
		valid bool
	}{
		{" basic", true},
		{"@åspecial", true},
		{"{extended}", true},
		{"hi! ☺", false},
	}
	for _, tc := range tcs {
		assert.Equal(t, tc.valid, IsValid(tc.str), tc.str)
	}
}

func TestSubstitutions(t *testing.T) {
	tcs := []struct {
		str string
		exp string
	}{
		{" basic", " basic"},
		{"êxtended", "extended"},
		{"“quoted”", `"quoted"`},
		{"\x09tab", " tab"},
	}
	for _, tc := range tcs {
		assert.Equal(t, tc.exp, ReplaceSubstitutions(tc.str), tc.str)
	}
}
