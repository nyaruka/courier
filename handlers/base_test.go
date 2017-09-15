package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	assert.Equal("This test\nhas a newline", DecodePossibleBase64("This test\nhas a newline"))
	assert.Equal("Please vote NO on the confirmation of Gorsuch.", DecodePossibleBase64("Please vote NO on the confirmation of Gorsuch."))
	assert.Equal("Bannon Explains The World ...\n“The Camp of the Saints", DecodePossibleBase64("QmFubm9uIEV4cGxhaW5zIFRoZSBXb3JsZCAuLi4K4oCcVGhlIENhbXAgb2YgdGhlIFNhaW50c+KA\r"))
	assert.Equal("the sweat, the tears and the sacrifice of working America", DecodePossibleBase64("dGhlIHN3ZWF0LCB0aGUgdGVhcnMgYW5kIHRoZSBzYWNyaWZpY2Ugb2Ygd29ya2luZyBBbWVyaWNh\r"))
	assert.Contains(DecodePossibleBase64("Tm93IGlzDQp0aGUgdGltZQ0KZm9yIGFsbCBnb29kDQpwZW9wbGUgdG8NCnJlc2lzdC4NCg0KSG93IGFib3V0IGhhaWt1cz8NCkkgZmluZCB0aGVtIHRvIGJlIGZyaWVuZGx5Lg0KcmVmcmlnZXJhdG9yDQoNCjAxMjM0NTY3ODkNCiFAIyQlXiYqKCkgW117fS09Xys7JzoiLC4vPD4/fFx+YA0KQUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xtbm9wcXJzdHV2d3h5eg=="), "I find them to be friendly")
	assert.Contains(DecodePossibleBase64(test6), "I received your letter today")
}

func TestSplitMsg(t *testing.T) {
	assert := assert.New(t)
	assert.Equal([]string{""}, SplitMsg("", 160))
	assert.Equal([]string{"Simple message"}, SplitMsg("Simple message", 160))
	assert.Equal([]string{"This is a message", "longer than 10"}, SplitMsg("This is a message longer than 10", 20))
	assert.Equal([]string{" "}, SplitMsg(" ", 20))
	assert.Equal([]string{"This is a message", "longer than 10"}, SplitMsg("This is a message   longer than 10", 20))
}
