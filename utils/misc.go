package utils

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path"
	"reflect"
	"unicode/utf8"

	validator "gopkg.in/go-playground/validator.v9"
)

var (
	validate = validator.New()
)

func Validate(obj any) error {
	return validate.Struct(obj)
}

// SignHMAC256 encrypts value with HMAC256 by using a private key
func SignHMAC256(privateKey string, value string) string {
	hash := hmac.New(sha256.New, []byte(privateKey))
	hash.Write([]byte(value))

	signedParams := hex.EncodeToString(hash.Sum(nil))
	return signedParams
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

func ChunkSlice[T any](slice []T, size int) [][]T {
	chunks := make([][]T, 0, len(slice)/size+1)

	for i := 0; i < len(slice); i += size {
		end := i + size
		if end > len(slice) {
			end = len(slice)
		}
		chunks = append(chunks, slice[i:end])
	}
	return chunks
}

// MapContains returns whether m1 contains all the key value pairs in m2
func MapContains[K comparable, V comparable, M ~map[K]V](m1 M, m2 M) bool {
	for k, v2 := range m2 {
		v1, ok := m1[k]
		if !ok || v1 != v2 {
			return false
		}
	}
	return true
}

// MapUpdate updates map m1 to contain the key value pairs in m2 - deleting any pairs in m1 which have zero values in m2.
func MapUpdate[K comparable, V comparable, M ~map[K]V](m1 M, m2 M) {
	for k, v := range m2 {
		if reflect.ValueOf(v).IsZero() {
			delete(m1, k)
		} else {
			m1[k] = v
		}
	}
}
