package handlers

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gorilla/schema"
	"github.com/nyaruka/courier"
	validator "gopkg.in/go-playground/validator.v9"
)

var base64Regex, _ = regexp.Compile("^([a-zA-Z0-9+/=]{4})+$")
var base64Encoding = base64.StdEncoding.Strict()

type BaseHandler struct {
	channelType courier.ChannelType
	name        string
	server      courier.Server
}

func NewBaseHandler(channelType courier.ChannelType, name string) BaseHandler {
	return BaseHandler{channelType: channelType, name: name}
}

func (h *BaseHandler) SetServer(server courier.Server) {
	h.server = server
}

func (h *BaseHandler) Server() courier.Server {
	return h.server
}

// ChannelType returns the channel type that this handler deals with
func (h *BaseHandler) ChannelType() courier.ChannelType {
	return h.channelType
}

// ChannelName returns the name of the channel this handler deals with
func (h *BaseHandler) ChannelName() string {
	return h.name
}

var (
	decoder  = schema.NewDecoder()
	validate = validator.New()
)

func init() {
	decoder.IgnoreUnknownKeys(true)
	decoder.SetAliasTag("name")
}

func mapAsJSON(m map[string]string) []byte {
	bytes, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return bytes
}

// JoinNonEmpty takes a vararg of strings and return the join of all the non-empty strings with a space between them
func JoinNonEmpty(delim string, strings ...string) string {
	var buf bytes.Buffer
	for _, s := range strings {
		if s != "" {
			if buf.Len() > 0 {
				buf.WriteString(delim)
			}
			buf.WriteString(s)
		}
	}
	return buf.String()
}

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

func DecodeAndValidateForm(data interface{}, r *http.Request) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	err = decoder.Decode(data, r.Form)
	if err != nil {
		return err
	}

	// check our input is valid
	err = validate.Struct(data)
	if err != nil {
		return err
	}

	return nil
}

func DecodeAndValidateQueryParams(data interface{}, r *http.Request) error {
	err := decoder.Decode(data, r.URL.Query())
	if err != nil {
		return err
	}

	// check our input is valid
	err = validate.Struct(data)
	if err != nil {
		return err
	}

	return nil
}

func DecodeAndValidateJSON(envelope interface{}, r *http.Request) error {
	// read our body
	body, err := ioutil.ReadAll(io.LimitReader(r.Body, 100000))
	defer r.Body.Close()
	if err != nil {
		return fmt.Errorf("unable to read request body: %s", err)
	}

	// try to decode our envelope
	if err = json.Unmarshal(body, envelope); err != nil {
		return fmt.Errorf("unable to parse request JSON: %s", err)
	}

	// check our input is valid
	err = validate.Struct(envelope)
	if err != nil {
		return fmt.Errorf("request JSON doesn't match required schema: %s", err)
	}

	return nil
}

/*
DecodePossibleBase64 detects and decodes a possibly base64 encoded messages by doing:
 * check it's at least 60 characters
 * check its length is divisible by 4
 * check that there's no whitespace
 * check the decoded string contains at least 50% ascii
*/
func DecodePossibleBase64(original string) string {
	stripped := strings.TrimSpace(strings.Replace(strings.Replace(original, "\r", "", -1), "\r", "", -1))
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

	decoded := decodeUTF8(decodedBytes)
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

// decodeUTF8 is equivalent to .decode('utf-8', 'ignore') in Python
func decodeUTF8(bytes []byte) string {
	s := string(bytes)
	if !utf8.ValidString(s) {
		v := make([]rune, 0, len(s))
		for i, r := range s {
			if r == utf8.RuneError {
				_, size := utf8.DecodeRuneInString(s[i:])
				if size == 1 {
					continue
				}
			}
			v = append(v, r)
		}
		s = string(v)
	}
	return s
}
