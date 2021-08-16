package utils_test

import (
	"net/url"
	"testing"

	"github.com/nyaruka/courier/utils"
	"github.com/stretchr/testify/assert"
)

func TestSignHMAC256(t *testing.T) {
	assert.Equal(t, "ce9a66626ee60f41beb538bbbafbf308cb8462a495c7abc6d04762ef9982f1e1",
		utils.SignHMAC256("DkGBlzdnzYeb2nm0", "valueToEncrypt"))
	assert.Len(t, utils.SignHMAC256("ZXwAumfRSejDxJGa", "newValueToEncrypt"), 64)
}

func TestMapAsJSON(t *testing.T) {
	assert.Equal(t, "{}", string(utils.MapAsJSON(map[string]string{})))
	assert.Equal(t, "{\"foo\":\"bar\"}", string(utils.MapAsJSON(map[string]string{"foo": "bar"})))
}

func TestJoinNonEmpty(t *testing.T) {
	assert.Equal(t, "", utils.JoinNonEmpty(" "))
	assert.Equal(t, "hello world", utils.JoinNonEmpty(" ", "", "hello", "", "world"))
}

func TestStringArrayContains(t *testing.T) {
	assert.False(t, utils.StringArrayContains([]string{}, "x"))
	assert.False(t, utils.StringArrayContains([]string{"a", "b"}, "x"))
	assert.True(t, utils.StringArrayContains([]string{"a", "b", "x", "y"}, "x"))
}

func TestCleanString(t *testing.T) {
	assert.Equal(t, "\x41hello", utils.CleanString("\x02\x41hello"))
	assert.Equal(t, "😅 happy!", utils.CleanString("😅 happy!"))
	assert.Equal(t, "Hello  There", utils.CleanString("Hello \x00 There"))
	assert.Equal(t, "Hello There", utils.CleanString("Hello There\u0000"))
	assert.Equal(t, "Hello z There", utils.CleanString("Hello \xc5z There"))

	text, _ := url.PathUnescape("hi%1C%00%00%00%00%00%07%E0%00")
	assert.Equal(t, "hi\x1c\a", utils.CleanString(text))
}

func TestURLGetFile(t *testing.T) {
	test1, err := utils.BasePathForURL("https://example.com/test.pdf")
	assert.Equal(t, nil, err)
	assert.Equal(t, "test.pdf", test1)

	test2, err := utils.BasePathForURL("application/pdf:https://some-url.host.service.com/media/999/zz99/9999/da514731-4bed-428c-afb9-860dd94530cc.xlsx")
	assert.Equal(t, nil, err)
	assert.Equal(t, "da514731-4bed-428c-afb9-860dd94530cc.xlsx", test2)
}

func TestStringsToRows(t *testing.T) {
	tcs := []struct {
		replies      []string
		maxRows      int
		maxRowRunes  int
		paddingRunes int
		expected     [][]string
	}{
		{

			replies:      []string{"OK"},
			maxRows:      3,
			maxRowRunes:  30,
			paddingRunes: 2,
			expected: [][]string{
				{"OK"},
			},
		},
		{
			// all values fit in single row
			replies:      []string{"Yes", "No", "Maybe"},
			maxRows:      3,
			maxRowRunes:  30,
			paddingRunes: 2,
			expected: [][]string{
				{"Yes", "No", "Maybe"},
			},
		},
		{
			// all values can be their own row
			replies:      []string{"12345678901234567890", "23456789012345678901", "34567890123456789012"},
			maxRows:      3,
			maxRowRunes:  25,
			paddingRunes: 2,
			expected: [][]string{
				{"12345678901234567890"},
				{"23456789012345678901"},
				{"34567890123456789012"},
			},
		},
		{
			replies:      []string{"1234567890", "2345678901", "3456789012", "4567890123"},
			maxRows:      3,
			maxRowRunes:  25,
			paddingRunes: 1,
			expected: [][]string{
				{"1234567890", "2345678901"},
				{"3456789012", "4567890123"},
			},
		},
		{
			// we break chars per row limit rather than row limit
			replies:      []string{"Vanilla", "Chocolate", "Strawberry", "Lemon Sorbet", "Ecuadorian Amazonian Papayas", "Mint"},
			maxRows:      3,
			maxRowRunes:  30,
			paddingRunes: 2,
			expected: [][]string{
				{"Vanilla", "Chocolate"},
				{"Strawberry", "Lemon Sorbet"},
				{"Ecuadorian Amazonian Papayas", "Mint"},
			},
		},
	}

	for _, tc := range tcs {
		rows := utils.StringsToRows(tc.replies, tc.maxRows, tc.maxRowRunes, tc.paddingRunes)
		assert.Equal(t, tc.expected, rows, "rows mismatch for replies %v", tc.replies)
	}
}
