package handlers_test

import (
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/test"
	"github.com/stretchr/testify/assert"
)

func TestSplitMsg(t *testing.T) {
	var channel = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AC", "2020", "US", nil)

	tcs := []struct {
		msg           courier.MsgOut
		opts          handlers.SplitOptions
		expectedParts []handlers.MsgPart
	}{
		{
			msg:  test.NewMockMsg(1001, "b6454f25-e5b9-4795-a180-b9e35ca3a523", channel, "tel+1234567890", "This is a message longer than 10", nil),
			opts: handlers.SplitOptions{MaxTextLen: 20},
			expectedParts: []handlers.MsgPart{
				{Type: handlers.MsgPartTypeText, Text: "This is a message", IsFirst: true},
				{Type: handlers.MsgPartTypeText, Text: "longer than 10", IsLast: true},
			},
		},
		{
			msg:  test.NewMockMsg(1001, "b6454f25-e5b9-4795-a180-b9e35ca3a523", channel, "tel+1234567890", "Lovely image", []string{"image/jpeg:http://test.jpg"}),
			opts: handlers.SplitOptions{MaxTextLen: 20},
			expectedParts: []handlers.MsgPart{
				{Type: handlers.MsgPartTypeAttachment, Attachment: "image/jpeg:http://test.jpg", IsFirst: true},
				{Type: handlers.MsgPartTypeText, Text: "Lovely image", IsLast: true},
			},
		},
		{
			msg:  test.NewMockMsg(1001, "b6454f25-e5b9-4795-a180-b9e35ca3a523", channel, "tel+1234567890", "Lovely image", []string{"image/jpeg:http://test.jpg"}),
			opts: handlers.SplitOptions{MaxTextLen: 20, Captionable: []handlers.MediaType{handlers.MediaTypeImage}},
			expectedParts: []handlers.MsgPart{
				{Type: handlers.MsgPartTypeCaptionedAttachment, Text: "Lovely image", Attachment: "image/jpeg:http://test.jpg", IsFirst: true, IsLast: true},
			},
		},
	}

	for _, tc := range tcs {
		actualParts := handlers.SplitMsg(tc.msg, tc.opts)
		assert.Equal(t, tc.expectedParts, actualParts)
	}

}

func TestSplitMsgByChannel(t *testing.T) {
	var channelWithMaxLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AC", "2020", "US",
		map[string]any{
			courier.ConfigUsername:  "user1",
			courier.ConfigPassword:  "pass1",
			courier.ConfigMaxLength: 25,
		})
	var channelWithoutMaxLength = test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "AC", "2020", "US",
		map[string]any{
			courier.ConfigUsername: "user1",
			courier.ConfigPassword: "pass1",
		})

	assert.Equal(t, []string{""}, handlers.SplitMsgByChannel(channelWithoutMaxLength, "", 160))
	assert.Equal(t, []string{"Simple message"}, handlers.SplitMsgByChannel(channelWithoutMaxLength, "Simple message", 160))
	assert.Equal(t, []string{"This is a message", "longer than 10"}, handlers.SplitMsgByChannel(channelWithoutMaxLength, "This is a message longer than 10", 20))
	assert.Equal(t, []string{" "}, handlers.SplitMsgByChannel(channelWithoutMaxLength, " ", 20))
	assert.Equal(t, []string{"This is a message", "longer than 10"}, handlers.SplitMsgByChannel(channelWithoutMaxLength, "This is a message   longer than 10", 20))

	// Max length should be the one configured on the channel
	assert.Equal(t, []string{""}, handlers.SplitMsgByChannel(channelWithMaxLength, "", 160))
	assert.Equal(t, []string{"Simple message"}, handlers.SplitMsgByChannel(channelWithMaxLength, "Simple message", 160))
	assert.Equal(t, []string{"This is a message longer", "than 10"}, handlers.SplitMsgByChannel(channelWithMaxLength, "This is a message longer than 10", 20))
	assert.Equal(t, []string{" "}, handlers.SplitMsgByChannel(channelWithMaxLength, " ", 20))
	assert.Equal(t, []string{"This is a message", "longer than 10"}, handlers.SplitMsgByChannel(channelWithMaxLength, "This is a message   longer than 10", 20))
}

func TestSplitText(t *testing.T) {
	assert.Equal(t, []string{""}, handlers.SplitText("", 160))
	assert.Equal(t, []string{"Simple message"}, handlers.SplitText("Simple message", 160))
	assert.Equal(t, []string{"This is a message", "longer than 10"}, handlers.SplitText("This is a message longer than 10", 20))
	assert.Equal(t, []string{" "}, handlers.SplitText(" ", 20))
	assert.Equal(t, []string{"This is a message", "longer than 10"}, handlers.SplitText("This is a message   longer than 10", 20))
}
