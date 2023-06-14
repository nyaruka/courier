package utils

func GetSupportedLanguage(lc Locale) LanguageInfo {
	// look for exact match
	if lang := supportedLanguages[lc]; lang.Code != "" {
		return lang
	}

	// if we have a country, strip that off and look again for a match
	l, c := lc.ToParts()
	if c != "" {
		if lang := supportedLanguages[Locale(l)]; lang.Code != "" {
			return lang
		}
	}
	return supportedLanguages["eng"] // fallback to English
}

type LanguageInfo struct {
	Code string
	Menu string // translation of "Menu"
}

// Mapping from engine locales to supported languages. Note that these are not all valid BCP47 Codes, e.g. fil
// see https://developers.facebook.com/docs/whatsapp/api/messages/message-templates/
var supportedLanguages = map[Locale]LanguageInfo{
	"afr":    {Code: "af", Menu: "Kieslys"},   // Afrikaans
	"sqi":    {Code: "sq", Menu: "Menu"},      // Albanian
	"ara":    {Code: "ar", Menu: "قائمة"},     // Arabic
	"aze":    {Code: "az", Menu: "Menu"},      // Azerbaijani
	"ben":    {Code: "bn", Menu: "Menu"},      // Bengali
	"bul":    {Code: "bg", Menu: "Menu"},      // Bulgarian
	"cat":    {Code: "ca", Menu: "Menu"},      // Catalan
	"zho":    {Code: "zh_CN", Menu: "菜单"},     // Chinese
	"zho-CN": {Code: "zh_CN", Menu: "菜单"},     // Chinese (CHN)
	"zho-HK": {Code: "zh_HK", Menu: "菜单"},     // Chinese (HKG)
	"zho-TW": {Code: "zh_TW", Menu: "菜单"},     // Chinese (TAI)
	"hrv":    {Code: "hr", Menu: "Menu"},      // Croatian
	"ces":    {Code: "cs", Menu: "Menu"},      // Czech
	"dah":    {Code: "da", Menu: "Menu"},      // Danish
	"nld":    {Code: "nl", Menu: "Menu"},      // Dutch
	"eng":    {Code: "en", Menu: "Menu"},      // English
	"eng-GB": {Code: "en_GB", Menu: "Menu"},   // English (UK)
	"eng-US": {Code: "en_US", Menu: "Menu"},   // English (US)
	"est":    {Code: "et", Menu: "Menu"},      // Estonian
	"fil":    {Code: "fil", Menu: "Menu"},     // Filipino
	"fin":    {Code: "fi", Menu: "Menu"},      // Finnish
	"fra":    {Code: "fr", Menu: "Menu"},      // French
	"kat":    {Code: "ka", Menu: "Menu"},      // Georgian
	"deu":    {Code: "de", Menu: "Menü"},      // German
	"ell":    {Code: "el", Menu: "Menu"},      // Greek
	"guj":    {Code: "gu", Menu: "Menu"},      // Gujarati
	"hau":    {Code: "ha", Menu: "Menu"},      // Hausa
	"enb":    {Code: "he", Menu: "תפריט"},     // Hebrew
	"hin":    {Code: "hi", Menu: "Menu"},      // Hindi
	"hun":    {Code: "hu", Menu: "Menu"},      // Hungarian
	"ind":    {Code: "id", Menu: "Menu"},      // Indonesian
	"gle":    {Code: "ga", Menu: "Roghchlár"}, // Irish
	"ita":    {Code: "it", Menu: "Menu"},      // Italian
	"jpn":    {Code: "ja", Menu: "Menu"},      // Japanese
	"kan":    {Code: "kn", Menu: "Menu"},      // Kannada
	"kaz":    {Code: "kk", Menu: "Menu"},      // Kazakh
	"kin":    {Code: "rw_RW", Menu: "Menu"},   // Kinyarwanda
	"kor":    {Code: "ko", Menu: "Menu"},      // Korean
	"kir":    {Code: "ky_KG", Menu: "Menu"},   // Kyrgyzstan
	"lao":    {Code: "lo", Menu: "Menu"},      // Lao
	"lav":    {Code: "lv", Menu: "Menu"},      // Latvian
	"lit":    {Code: "lt", Menu: "Menu"},      // Lithuanian
	"mal":    {Code: "ml", Menu: "Menu"},      // Malayalam
	"mkd":    {Code: "mk", Menu: "Menu"},      // Macedonian
	"msa":    {Code: "ms", Menu: "Menu"},      // Malay
	"mar":    {Code: "mr", Menu: "Menu"},      // Marathi
	"nob":    {Code: "nb", Menu: "Menu"},      // Norwegian
	"fas":    {Code: "fa", Menu: "Menu"},      // Persian
	"pol":    {Code: "pl", Menu: "Menu"},      // Polish
	"por":    {Code: "pt_PT", Menu: "Menu"},   // Portuguese
	"por-BR": {Code: "pt_BR", Menu: "Menu"},   // Portuguese (BR)
	"por-PT": {Code: "pt_PT", Menu: "Menu"},   // Portuguese (POR)
	"pan":    {Code: "pa", Menu: "Menu"},      // Punjabi
	"ron":    {Code: "ro", Menu: "Menu"},      // Romanian
	"rus":    {Code: "ru", Menu: "Menu"},      // Russian
	"srp":    {Code: "sr", Menu: "Menu"},      // Serbian
	"slk":    {Code: "sk", Menu: "Menu"},      // Slovak
	"slv":    {Code: "sl", Menu: "Menu"},      // Slovenian
	"spa":    {Code: "es", Menu: "Menú"},      // Spanish
	"spa-AR": {Code: "es_AR", Menu: "Menú"},   // Spanish (ARG)
	"spa-ES": {Code: "es_ES", Menu: "Menú"},   // Spanish (SPA)
	"spa-MX": {Code: "es_MX", Menu: "Menú"},   // Spanish (MEX)
	"swa":    {Code: "sw", Menu: "Menyu"},     // Swahili
	"swe":    {Code: "sv", Menu: "Menu"},      // Swedish
	"tam":    {Code: "ta", Menu: "Menu"},      // Tamil
	"tel":    {Code: "te", Menu: "Menu"},      // Telugu
	"tha":    {Code: "th", Menu: "Menu"},      // Thai
	"tur":    {Code: "tr", Menu: "Menu"},      // Turkish
	"ukr":    {Code: "uk", Menu: "Menu"},      // Ukrainian
	"urd":    {Code: "ur", Menu: "Menu"},      // Urdu
	"uzb":    {Code: "uz", Menu: "Menu"},      // Uzbek
	"vie":    {Code: "vi", Menu: "Menu"},      // Vietnamese
	"zul":    {Code: "zu", Menu: "Menu"},      // Zulu
}
