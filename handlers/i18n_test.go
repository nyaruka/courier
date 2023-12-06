package handlers_test

import (
	"testing"

	"github.com/nyaruka/courier/handlers"
	"github.com/stretchr/testify/assert"
)

func TestGetText(t *testing.T) {
	assert.Equal(t, "Menu", handlers.GetText("Menu", "eng"))
	assert.Equal(t, "Menú", handlers.GetText("Menu", "spa"))
	assert.Equal(t, "Menú", handlers.GetText("Menu", "spa-MX"))
	assert.Equal(t, "Menyu", handlers.GetText("Menu", "swa"))
	assert.Equal(t, "Foo", handlers.GetText("Foo", "eng"))
}
