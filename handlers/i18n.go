package handlers

import "github.com/nyaruka/gocommon/i18n"

func GetText(text string, locale i18n.Locale) string {
	if set, ok := translations[text]; ok {
		lang, _ := locale.Split()
		if trans := set[lang]; trans != "" {
			return trans
		}
	}
	return text
}

var translations = map[string]map[i18n.Language]string{
	"Menu": {
		"afr": "Kieslys",
		"ara": "قائمة",
		"zho": "菜单",
		"heb": "תפריט",
		"gle": "Roghchlár",
		"spa": "Menú",
		"swa": "Menyu",
	},
}
