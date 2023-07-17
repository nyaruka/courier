package handlers_test

import (
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/stretchr/testify/assert"
)

func TestWAGetSupportedLanguage(t *testing.T) {
	assert.Equal(t, handlers.LanguageInfo{"en", "Menu"}, handlers.WAGetSupportedLanguage(courier.NilLocale))
	assert.Equal(t, handlers.LanguageInfo{"en", "Menu"}, handlers.WAGetSupportedLanguage(courier.Locale("eng")))
	assert.Equal(t, handlers.LanguageInfo{"en_US", "Menu"}, handlers.WAGetSupportedLanguage(courier.Locale("eng-US")))
	assert.Equal(t, handlers.LanguageInfo{"pt_PT", "Menu"}, handlers.WAGetSupportedLanguage(courier.Locale("por")))
	assert.Equal(t, handlers.LanguageInfo{"pt_PT", "Menu"}, handlers.WAGetSupportedLanguage(courier.Locale("por-PT")))
	assert.Equal(t, handlers.LanguageInfo{"pt_BR", "Menu"}, handlers.WAGetSupportedLanguage(courier.Locale("por-BR")))
	assert.Equal(t, handlers.LanguageInfo{"fil", "Menu"}, handlers.WAGetSupportedLanguage(courier.Locale("fil")))
	assert.Equal(t, handlers.LanguageInfo{"fr", "Menu"}, handlers.WAGetSupportedLanguage(courier.Locale("fra-CA")))
	assert.Equal(t, handlers.LanguageInfo{"en", "Menu"}, handlers.WAGetSupportedLanguage(courier.Locale("run")))
}
