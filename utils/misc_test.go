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

func TestURLGetFile(t *testing.T) {
	test1, err := BasePathForURL("https://example.com/test.pdf")
	assert.Equal(t, nil, err)
	assert.Equal(t, "test.pdf", test1)

	test2, err := BasePathForURL("application/pdf:https://some-url.host.service.com/media/999/zz99/9999/da514731-4bed-428c-afb9-860dd94530cc.xlsx")
	assert.Equal(t, nil, err)
	assert.Equal(t, "da514731-4bed-428c-afb9-860dd94530cc.xlsx", test2)
}
