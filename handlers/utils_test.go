package handlers_test

import (
	"testing"

	"github.com/nyaruka/courier/handlers"
	"github.com/stretchr/testify/assert"
)

var urlTestCases = []struct {
	text  string
	valid bool
}{
	// supported by whatsapp
	{"http://foo.com/blah_blah", true},
	{"http://foo.com/blah_blah/", true},
	{"http://foo.com/blah_blah_(wikipedia)", true},
	{"http://foo.com/blah_blah_(wikipedia)_(again)", true},
	{"http://www.example.com/wpstyle/?p=364", true},
	{"https://www.example.com/foo/?bar=baz&inga=42&quux", true},
	{"http://userid:password@example.com:8080", true},
	{"http://userid:password@example.com:8080/", true},
	{"http://userid@example.com", true},
	{"http://userid@example.com/", true},
	{"http://userid@example.com:8080", true},
	{"http://userid@example.com:8080/", true},
	{"http://userid:password@example.com", true},
	{"http://userid:password@example.com/", true},
	{"http://foo.com/blah_(wikipedia)#cite-1", true},
	{"http://foo.com/blah_(wikipedia)_blah#cite-1", true},
	{"http://foo.com/unicode_(✪)_in_parens", true},
	{"http://foo.com/(something)?after=parens", true},
	{"http://code.google.com/events/#&product=browser", true},
	{"http://foo.bar/?q=Test%20URL-encoded%20stuff", true},
	{"http://1337.net", true},
	{"http://a.b-c.de", true},
	{"http://foo.bar?q=Spaces foo bar", true},
	{"http://foo.bar/foo(bar)baz quux", true},
	{"http://a.b--c.de/", true},
	{"http://www.foo.bar./", true},
	// not supported by whatsapp
	{"http://✪df.ws/123", false},
	{"http://142.42.1.1/", false},
	{"http://142.42.1.1:8080/", false},
	{"http://➡.ws/䨹", false},
	{"http://⌘.ws", false},
	{"http://⌘.ws/", false},
	{"http://☺.damowmow.com/", false},
	{"ftp://foo.bar/baz", false},
	{"http://مثال.إختبار", false},
	{"http://例子.测试", false},
	{"http://उदाहरण.परीक्षा", false},
	{"http://-.~_!$&'()*+,;=:%40:80%2f::::::@example.com", false},
	{"http://223.255.255.254", false},
	{"https://foo_bar.example.com/", false},
	{"http://", false},
	{"http://.", false},
	{"http://..", false},
	{"http://../", false},
	{"http://?", false},
	{"http://??", false},
	{"http://??/", false},
	{"http://#", false},
	{"http://##", false},
	{"http://##/", false},
	{"//", false},
	{"//a", false},
	{"///a", false},
	{"///", false},
	{"http:///a", false},
	{"foo.com", false},
	{"rdar://1234", false},
	{"h://test", false},
	{"http:// shouldfail.com", false},
	{":// should fail", false},
	{"ftps://foo.bar/", false},
	{"http://-error-.invalid/", false},
	{"http://-a.b.co", false},
	{"http://a.b-.co", false},
	{"http://0.0.0.0", false},
	{"http://10.1.1.0", false},
	{"http://10.1.1.255", false},
	{"http://224.1.1.1", false},
	{"http://1.1.1.1.1", false},
	{"http://123.123.123", false},
	{"http://3628126748", false},
	{"http://.www.foo.bar/", false},
	{"http://.www.foo.bar./", false},
	{"http://10.1.1.1", false},
	{"http://10.1.1.254", false},
}

func TestIsURL(t *testing.T) {
	for _, tc := range urlTestCases {
		assert.Equal(t, tc.valid, handlers.IsURL(tc.text), "isURL mimatch for input %s", tc.text)
	}
}
