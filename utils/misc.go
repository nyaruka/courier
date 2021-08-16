package utils

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SignHMAC256 encrypts value with HMAC256 by using a private key
func SignHMAC256(privateKey string, value string) string {
	hash := hmac.New(sha256.New, []byte(privateKey))
	hash.Write([]byte(value))

	signedParams := hex.EncodeToString(hash.Sum(nil))
	return signedParams
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

// BasePathForURL, parse static URL, and return filename + format
func BasePathForURL(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, err
	}
	return path.Base(parsedURL.Path), nil
}

// StringsToRows takes a slice of strings and re-organizes it into rows and columns
func StringsToRows(strs []string, maxRows, maxRowRunes, paddingRunes int) [][]string {
	// calculate rune length if it's all one row
	totalRunes := 0
	for i := range strs {
		totalRunes += utf8.RuneCountInString(strs[i]) + paddingRunes*2
	}

	if totalRunes <= maxRowRunes {
		// if all strings fit on a single row, do that
		return [][]string{strs}
	} else if len(strs) <= maxRows {
		// if each string can be a row, do that
		rows := make([][]string, len(strs))
		for i := range strs {
			rows[i] = []string{strs[i]}
		}
		return rows
	}

	rows := [][]string{{}}
	curRow := 0
	rowRunes := 0

	for _, str := range strs {
		strRunes := utf8.RuneCountInString(str) + paddingRunes*2

		// take a new row if we can't fit this string and the current row isn't empty and we haven't hit the row limit
		if rowRunes+strRunes > maxRowRunes && len(rows[curRow]) > 0 && len(rows) < maxRows {
			rows = append(rows, []string{})
			curRow += 1
			rowRunes = 0
		}

		rows[curRow] = append(rows[curRow], str)
		rowRunes += strRunes
	}
	return rows
}
