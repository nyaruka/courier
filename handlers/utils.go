package handlers

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
)

var (
	urlRegex = regexp.MustCompile(`https?:\/\/(www\.)?[^\W][-a-zA-Z0-9@:%.\+~#=]{1,256}[^\W]\.[a-zA-Z()]{1,6}\b([-a-zA-Z0-9()@:%_\+.~#?&//=]*)`)
)

// GetTextAndAttachments returns both the text of our message as well as any attachments, newline delimited
func GetTextAndAttachments(m courier.MsgOut) string {
	buf := bytes.NewBuffer([]byte(m.Text()))
	for _, a := range m.Attachments() {
		_, url := SplitAttachment(a)
		buf.WriteString("\n")
		buf.WriteString(url)
	}
	return buf.String()
}

// SplitAttachment takes an attachment string and returns the media type and URL for the attachment
func SplitAttachment(attachment string) (string, string) {
	parts := strings.SplitN(attachment, ":", 2)
	if len(parts) < 2 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

// TextOnlyQuickReplies returns the text of a list of quick replies
func TextOnlyQuickReplies(qrs []courier.QuickReply) []string {
	t := make([]string, len(qrs))
	for i, qr := range qrs {
		t[i] = qr.Text
	}
	return t
}

// NameFromFirstLastUsername is a utility function to build a contact's name from the passed
// in values, all of which can be empty
func NameFromFirstLastUsername(first string, last string, username string) string {
	if first != "" && last != "" {
		return fmt.Sprintf("%s %s", first, last)
	} else if first != "" {
		return first
	} else if last != "" {
		return last
	} else if username != "" {
		return username
	}
	return ""
}

var base64Regex, _ = regexp.Compile("^([a-zA-Z0-9+/=]{4})+$")
var base64Encoding = base64.StdEncoding.Strict()

// DecodePossibleBase64 detects and decodes a possibly base64 encoded messages by doing:
//   - check it's at least 60 characters
//   - check its length is divisible by 4
//   - check that there's no whitespace
//   - check the decoded string contains at least 50% ascii
func DecodePossibleBase64(original string) string {
	stripped := strings.TrimSpace(strings.Replace(strings.Replace(original, "\r", "", -1), "\n", "", -1))
	length := len([]rune(stripped))

	if length < 60 || length%4 != 0 {
		return original
	}

	if !base64Regex.MatchString(stripped[:length-4]) {
		return original
	}

	decodedBytes, err := base64Encoding.DecodeString(stripped)
	if err != nil {
		return original
	}

	decoded := utils.DecodeUTF8(decodedBytes)
	numASCIIChars := 0
	for _, c := range decoded {
		if c <= 127 {
			numASCIIChars++
		}
	}

	if float32(numASCIIChars)/float32(len([]rune(decoded))) < 0.5 {
		return original
	}

	return decoded
}

func IsURL(s string) bool {
	return urlRegex.MatchString(s)
}
