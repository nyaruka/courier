package utils

import "strings"

//-----------------------------------------------------------------------------
// Locale
//-----------------------------------------------------------------------------

// Locale is the combination of a language and optional country, e.g. US English, Brazilian Portuguese, encoded as the
// language code followed by the country code, e.g. eng-US, por-BR
type Locale string

func (l Locale) ToParts() (string, string) {
	if l == NilLocale || len(l) < 3 {
		return "", ""
	}

	parts := strings.SplitN(string(l), "-", 2)
	lang := parts[0]
	country := ""
	if len(parts) > 1 {
		country = parts[1]
	}

	return lang, country
}

var NilLocale = Locale("")
