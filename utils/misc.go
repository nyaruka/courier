package utils

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gofrs/uuid"
)

// SignHMAC256 encrypts value with HMAC256 by using a private key
func SignHMAC256(privateKey string, value string) string {
	hash := hmac.New(sha256.New, []byte(privateKey))
	hash.Write([]byte(value))

	signedParams := hex.EncodeToString(hash.Sum(nil))
	return signedParams
}

// NewUUID generates a new v4 UUID
func NewUUID() string {
	u, err := uuid.NewV4()
	if err != nil {
		// if we can't generate a UUID.. we're done
		panic(fmt.Sprintf("unable to generate UUID: %s", err))
	}
	return u.String()
}

// MapAsJSON serializes the given map as a JSON string
func MapAsJSON(m map[string]string) []byte {
	bytes, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return bytes
}

// JoinNonEmpty takes a vararg of strings and return the join of all the non-empty strings with a delimiter between them
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

// DecodeUTF8 is equivalent to .decode('utf-8', 'ignore') in Python
func DecodeUTF8(bytes []byte) string {
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

// StringArrayContains returns whether a given string array contains the given element
func StringArrayContains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

var invalidChars = regexp.MustCompile("([\u0000-\u0008]|[\u000B-\u000C]|[\u000E-\u001F])")

// CleanString removes any control characters from the passed in string
func CleanString(s string) string {
	cleaned := invalidChars.ReplaceAllString(s, "")

	// check whether this is valid UTF8
	if !utf8.ValidString(cleaned) || strings.Contains(cleaned, "\x00") {
		v := make([]rune, 0, len(cleaned))
		for i, r := range s {
			if r == utf8.RuneError {
				_, size := utf8.DecodeRuneInString(s[i:])
				if size == 1 {
					continue
				}
			}

			if r != 0 {
				v = append(v, r)
			}
		}
		cleaned = string(v)
	}

	return cleaned
}
