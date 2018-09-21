package utils

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSignHMAC256(t *testing.T) {
	assert.Equal(t, "ce9a66626ee60f41beb538bbbafbf308cb8462a495c7abc6d04762ef9982f1e1",
		SignHMAC256("DkGBlzdnzYeb2nm0", "valueToEncrypt"))
	assert.Len(t, SignHMAC256("ZXwAumfRSejDxJGa", "newValueToEncrypt"), 64)
}

func TestMapAsJSON(t *testing.T) {
	assert.Equal(t, "{}", string(MapAsJSON(map[string]string{})))
	assert.Equal(t, "{\"foo\":\"bar\"}", string(MapAsJSON(map[string]string{"foo": "bar"})))
}

func TestJoinNonEmpty(t *testing.T) {
	assert.Equal(t, "", JoinNonEmpty(" "))
	assert.Equal(t, "hello world", JoinNonEmpty(" ", "", "hello", "", "world"))
}

func TestStringArrayContains(t *testing.T) {
	assert.False(t, StringArrayContains([]string{}, "x"))
	assert.False(t, StringArrayContains([]string{"a", "b"}, "x"))
	assert.True(t, StringArrayContains([]string{"a", "b", "x", "y"}, "x"))
}

func TestCleanString(t *testing.T) {
	assert.Equal(t, "\x41hello", CleanString("\x02\x41hello"))
	assert.Equal(t, "😅 happy!", CleanString("😅 happy!"))
	assert.Equal(t, "Hello  There", CleanString("Hello \x00 There"))
	assert.Equal(t, "Hello There", CleanString("Hello There\u0000"))
	assert.Equal(t, "Hello z There", CleanString("Hello \xc5z There"))

	text, _ := url.PathUnescape("hi%1C%00%00%00%00%00%07%E0%00")
	assert.Equal(t, "hi\x1c\a", CleanString(text))
}
