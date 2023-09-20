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

var test6 = `
SSByZWNlaXZlZCB5b3VyIGxldHRlciB0b2RheSwgaW4gd2hpY2ggeW91IHNheSB5b3Ugd2FudCB0
byByZXNjdWUgTm9ydGggQ2Fyb2xpbmlhbnMgZnJvbSB0aGUgQUNBLCBvciBPYmFtYWNhcmUgYXMg
eW91IG9kZGx5IGluc2lzdCBvbiBjYWxsaW5nIGl0LiAKCkkgaGF2ZSB0byBjYWxsIHlvdXIgYXR0
ZW50aW9uIHRvIHlvdXIgc2luIG9mIG9taXNzaW9uLiBZb3Ugc2F5IHRoYXQgd2UgYXJlIGRvd24g
dG8gb25lIGluc3VyZXIgYmVjYXVzZSBvZiBPYmFtYWNhcmUuIERpZCB5b3UgZm9yZ2V0IHRoYXQg
VGhlIEJhdGhyb29tIFN0YXRlIGhhcyBkb25lIGV2ZXJ5dGhpbmcgcG9zc2libGUgdG8gbWFrZSBU
aGUgQUNBIGZhaWw/ICBJbmNsdWRpbmcgbWlsbGlvbnMgb2YgZG9sbGFycyBmcm9tIHRoZSBmZWQ/
CgpXZSBkb24ndCBuZWVkIHRvIGJlIHNhdmVkIGZyb20gYSBwcm9ncmFtIHRoYXQgaGFzIGhlbHBl
ZCB0aG91c2FuZHMuIFdlIG5lZWQgeW91IHRvIGJ1Y2tsZSBkb3duIGFuZCBpbXByb3ZlIHRoZSBB
Q0EuIFlvdSBoYWQgeWVhcnMgdG8gY29tZSB1cCB3aXRoIGEgcGxhbi4gWW91IGZhaWxlZC4gCgpU
aGUgbGF0ZXN0IHZlcnNpb24geW91ciBwYXJ0eSBoYXMgY29tZSB1cCB3aXRoIHVzIHdvcnNlIHRo
YW4gdGhlIGxhc3QuIFBsZWFzZSB2b3RlIGFnYWluc3QgaXQuIERvbid0IGNvbmRlbW4gdGhlIGdv
b2Qgb2YgcGVvcGxlIG9mIE5DIHRvIGxpdmVzIHRoYXQgYXJlIG5hc3R5LCBicnV0aXNoIGFuZCBz
aG9ydC4gSSdtIG9uZSBvZiB0aGUgZm9sa3Mgd2hvIHdpbGwgZGllIGlmIHlvdSByaXAgdGhlIHBy
b3RlY3Rpb25zIGF3YXkuIAoKVm90ZSBOTyBvbiBhbnkgYmlsbCB0aGF0IGRvZXNuJ3QgY29udGFp
biBwcm90ZWN0aW9ucyBpbnN0ZWFkIG9mIHB1bmlzaG1lbnRzLiBXZSBhcmUgd2F0Y2hpbmcgY2xv
c2VseS4g`

func TestDecodePossibleBase64(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("This test\nhas a newline", handlers.DecodePossibleBase64("This test\nhas a newline"))
	assert.Equal("Please vote NO on the confirmation of Gorsuch.", handlers.DecodePossibleBase64("Please vote NO on the confirmation of Gorsuch."))
	assert.Equal("Bannon Explains The World ...\n“The Camp of the Saints", handlers.DecodePossibleBase64("QmFubm9uIEV4cGxhaW5zIFRoZSBXb3JsZCAuLi4K4oCcVGhlIENhbXAgb2YgdGhlIFNhaW50c+KA\r"))
	assert.Equal("the sweat, the tears and the sacrifice of working America", handlers.DecodePossibleBase64("dGhlIHN3ZWF0LCB0aGUgdGVhcnMgYW5kIHRoZSBzYWNyaWZpY2Ugb2Ygd29ya2luZyBBbWVyaWNh\r"))
	assert.Contains(handlers.DecodePossibleBase64("Tm93IGlzDQp0aGUgdGltZQ0KZm9yIGFsbCBnb29kDQpwZW9wbGUgdG8NCnJlc2lzdC4NCg0KSG93IGFib3V0IGhhaWt1cz8NCkkgZmluZCB0aGVtIHRvIGJlIGZyaWVuZGx5Lg0KcmVmcmlnZXJhdG9yDQoNCjAxMjM0NTY3ODkNCiFAIyQlXiYqKCkgW117fS09Xys7JzoiLC4vPD4/fFx+YA0KQUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xtbm9wcXJzdHV2d3h5eg=="), "I find them to be friendly")
	assert.Contains(handlers.DecodePossibleBase64(test6), "I received your letter today")
}
