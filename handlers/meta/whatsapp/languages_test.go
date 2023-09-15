package whatsapp_test

import (
	"testing"

	"github.com/nyaruka/courier/handlers/meta/whatsapp"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/stretchr/testify/assert"
)

func TestGetSupportedLanguage(t *testing.T) {
	assert.Equal(t, "en", whatsapp.GetSupportedLanguage(i18n.NilLocale))
	assert.Equal(t, "en", whatsapp.GetSupportedLanguage("eng"))
	assert.Equal(t, "en_US", whatsapp.GetSupportedLanguage("eng-US"))
	assert.Equal(t, "pt_BR", whatsapp.GetSupportedLanguage("por"))
	assert.Equal(t, "pt_PT", whatsapp.GetSupportedLanguage("por-PT"))
	assert.Equal(t, "pt_BR", whatsapp.GetSupportedLanguage("por-BR"))
	assert.Equal(t, "fil", whatsapp.GetSupportedLanguage("fil"))
	assert.Equal(t, "fr", whatsapp.GetSupportedLanguage("fra-CA"))
	assert.Equal(t, "en", whatsapp.GetSupportedLanguage("run"))
}

func TestGetMenuButton(t *testing.T) {
	assert.Equal(t, "Menu", whatsapp.GetMenuButton("en"))
	assert.Equal(t, "Menú", whatsapp.GetMenuButton("es_MX"))
	assert.Equal(t, "Menyu", whatsapp.GetMenuButton("sw"))
}
