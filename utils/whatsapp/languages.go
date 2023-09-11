package whatsapp

import "github.com/nyaruka/gocommon/i18n"

func GetSupportedLanguage(lc i18n.Locale) string {
	if lc == i18n.NilLocale {
		return "en"
	}
	return supportedLanguages.ForLocales(lc, "en")
}

// see https://developers.facebook.com/docs/whatsapp/api/messages/message-templates/
var supportedLanguages = i18n.NewBCP47Matcher(
	"af",    // Afrikaans
	"sq",    // Albanian
	"ar",    // Arabic
	"az",    // Azerbaijani
	"bn",    // Bengali
	"bg",    // Bulgarian
	"ca",    // Catalan
	"zh_CN", // Chinese (CHN)
	"zh_HK", // Chinese (HKG)
	"zh_TW", // Chinese (TAI)
	"hr",    // Croatian
	"cs",    // Czech
	"da",    // Danish
	"nl",    // Dutch
	"en",    // English
	"en_GB", // English (UK)
	"en_US", // English (US)
	"et",    // Estonian
	"fil",   // Filipino
	"fi",    // Finnish
	"fr",    // French
	"ka",    // Georgian
	"de",    // German
	"el",    // Greek
	"gu",    // Gujarati
	"ha",    // Hausa
	"he",    // Hebrew
	"hi",    // Hindi
	"hu",    // Hungarian
	"id",    // Indonesian
	"ga",    // Irish
	"it",    // Italian
	"ja",    // Japanese
	"kn",    // Kannada
	"kk",    // Kazakh
	"rw_RW", // Kinyarwanda
	"ko",    // Korean
	"ky_KG", // Kyrgyzstan
	"lo",    // Lao
	"lv",    // Latvian
	"lt",    // Lithuanian
	"mk",    // Macedonian
	"ms",    // Malay
	"ml",    // Malayalam
	"mr",    // Marathi
	"nb",    // Norwegian
	"fa",    // Persian
	"pl",    // Polish
	"pt_BR", // Portuguese (BR)
	"pt_PT", // Portuguese (POR)
	"pa",    // Punjabi
	"ro",    // Romanian
	"ru",    // Russian
	"sr",    // Serbian
	"sk",    // Slovak
	"sl",    // Slovenian
	"es",    // Spanish
	"es_AR", // Spanish (ARG)
	"es_ES", // Spanish (SPA)
	"es_MX", // Spanish (MEX)
	"sw",    // Swahili
	"sv",    // Swedish
	"ta",    // Tamil
	"te",    // Telugu
	"th",    // Thai
	"tr",    // Turkish
	"uk",    // Ukrainian
	"ur",    // Urdu
	"uz",    // Uzbek
	"vi",    // Vietnamese
	"zu",    // Zulu
)

func GetMenuButton(lang string) string {
	if trans := menuTranslations[lang]; trans != "" {
		return trans
	}
	return "Menu"
}

var menuTranslations = map[string]string{
	"af":    "Kieslys",
	"ar":    "قائمة",
	"zh_CN": "菜单",
	"zh_HK": "菜单",
	"zh_TW": "菜单",
	"he":    "תפריט",
	"ga":    "Roghchlár",
	"es":    "Menú",
	"es_AR": "Menú",
	"es_ES": "Menú",
	"es_MX": "Menú",
	"sw":    "Menyu",
}
