package utils_test

import (
	"testing"

	"github.com/nyaruka/courier/utils"
	"github.com/stretchr/testify/assert"
)

func TestGetSupportedLanguage(t *testing.T) {
	assert.Equal(t, utils.LanguageInfo{"en", "Menu"}, utils.GetSupportedLanguage(utils.NilLocale))
	assert.Equal(t, utils.LanguageInfo{"en", "Menu"}, utils.GetSupportedLanguage(utils.Locale("eng")))
	assert.Equal(t, utils.LanguageInfo{"en_US", "Menu"}, utils.GetSupportedLanguage(utils.Locale("eng-US")))
	assert.Equal(t, utils.LanguageInfo{"pt_PT", "Menu"}, utils.GetSupportedLanguage(utils.Locale("por")))
	assert.Equal(t, utils.LanguageInfo{"pt_PT", "Menu"}, utils.GetSupportedLanguage(utils.Locale("por-PT")))
	assert.Equal(t, utils.LanguageInfo{"pt_BR", "Menu"}, utils.GetSupportedLanguage(utils.Locale("por-BR")))
	assert.Equal(t, utils.LanguageInfo{"fil", "Menu"}, utils.GetSupportedLanguage(utils.Locale("fil")))
	assert.Equal(t, utils.LanguageInfo{"fr", "Menu"}, utils.GetSupportedLanguage(utils.Locale("fra-CA")))
	assert.Equal(t, utils.LanguageInfo{"en", "Menu"}, utils.GetSupportedLanguage(utils.Locale("run")))
}
