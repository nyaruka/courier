package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodePossibleBase64(t *testing.T) {
	assert := assert.New(t)
	assert.Equal("This test\nhas a newline", DecodePossibleBase64("This test\nhas a newline"))
	assert.Equal("Please vote NO on the confirmation of Gorsuch.", DecodePossibleBase64("Please vote NO on the confirmation of Gorsuch."))
	assert.Equal("Bannon Explains The World ...\n“The Camp of the Saints", DecodePossibleBase64("QmFubm9uIEV4cGxhaW5zIFRoZSBXb3JsZCAuLi4K4oCcVGhlIENhbXAgb2YgdGhlIFNhaW50c+KA\r"))
	assert.Equal("the sweat, the tears and the sacrifice of working America", DecodePossibleBase64("dGhlIHN3ZWF0LCB0aGUgdGVhcnMgYW5kIHRoZSBzYWNyaWZpY2Ugb2Ygd29ya2luZyBBbWVyaWNh\r"))
	assert.Contains(DecodePossibleBase64("Tm93IGlzDQp0aGUgdGltZQ0KZm9yIGFsbCBnb29kDQpwZW9wbGUgdG8NCnJlc2lzdC4NCg0KSG93IGFib3V0IGhhaWt1cz8NCkkgZmluZCB0aGVtIHRvIGJlIGZyaWVuZGx5Lg0KcmVmcmlnZXJhdG9yDQoNCjAxMjM0NTY3ODkNCiFAIyQlXiYqKCkgW117fS09Xys7JzoiLC4vPD4/fFx+YA0KQUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVphYmNkZWZnaGlqa2xtbm9wcXJzdHV2d3h5eg=="), "I find them to be friendly")
}
