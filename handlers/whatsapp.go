package handlers

import (
	"encoding/json"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/utils"
	"github.com/pkg/errors"
)

func WAGetSupportedLanguage(lc courier.Locale) LanguageInfo {
	// look for exact match
	if lang := supportedLanguages[lc]; lang.Code != "" {
		return lang
	}

	// if we have a country, strip that off and look again for a match
	l, c := lc.ToParts()
	if c != "" {
		if lang := supportedLanguages[courier.Locale(l)]; lang.Code != "" {
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
var supportedLanguages = map[courier.Locale]LanguageInfo{
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

type WAMsgTemplating struct {
	Template struct {
		Name string `json:"name" validate:"required"`
		UUID string `json:"uuid" validate:"required"`
	} `json:"template" validate:"required,dive"`
	Namespace string   `json:"namespace"`
	Variables []string `json:"variables"`
}

func GetTemplating(msg courier.Msg) (*WAMsgTemplating, error) {
	if len(msg.Metadata()) == 0 {
		return nil, nil
	}

	metadata := &struct {
		Templating *WAMsgTemplating `json:"templating"`
	}{}
	if err := json.Unmarshal(msg.Metadata(), metadata); err != nil {
		return nil, err
	}

	if metadata.Templating == nil {
		return nil, nil
	}

	if err := utils.Validate(metadata.Templating); err != nil {
		return nil, errors.Wrapf(err, "invalid templating definition")
	}

	return metadata.Templating, nil
}

var WaStatusMapping = map[string]courier.MsgStatusValue{
	"sending":   courier.MsgWired,
	"sent":      courier.MsgSent,
	"delivered": courier.MsgDelivered,
	"read":      courier.MsgDelivered,
	"failed":    courier.MsgFailed,
}

var WaIgnoreStatuses = map[string]bool{
	"deleted": true,
}

type Sender struct {
	ID      string `json:"id"`
	UserRef string `json:"user_ref,omitempty"`
}

type User struct {
	ID string `json:"id"`
}

// {
//   "object":"page",
//   "entry":[{
//     "id":"180005062406476",
//     "time":1514924367082,
//     "messaging":[{
//       "sender":  {"id":"1630934236957797"},
//       "recipient":{"id":"180005062406476"},
//       "timestamp":1514924366807,
//       "message":{
//         "mid":"mid.$cAAD5QiNHkz1m6cyj11guxokwkhi2",
//         "seq":33116,
//         "text":"65863634"
//       }
//     }]
//   }]
// }

type WacMedia struct {
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	ID       string `json:"id"`
	Mimetype string `json:"mime_type"`
	SHA256   string `json:"sha256"`
}
type WACMOPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		ID      string `json:"id"`
		Time    int64  `json:"time"`
		Changes []struct {
			Field string `json:"field"`
			Value struct {
				MessagingProduct string `json:"messaging_product"`
				Metadata         *struct {
					DisplayPhoneNumber string `json:"display_phone_number"`
					PhoneNumberID      string `json:"phone_number_id"`
				} `json:"metadata"`
				Contacts []struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
					WaID string `json:"wa_id"`
				} `json:"contacts"`
				Messages []struct {
					ID        string `json:"id"`
					From      string `json:"from"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Context   *struct {
						Forwarded           bool   `json:"forwarded"`
						FrequentlyForwarded bool   `json:"frequently_forwarded"`
						From                string `json:"from"`
						ID                  string `json:"id"`
					} `json:"context"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
					Image    *WacMedia `json:"image"`
					Audio    *WacMedia `json:"audio"`
					Video    *WacMedia `json:"video"`
					Document *WacMedia `json:"document"`
					Voice    *WacMedia `json:"voice"`
					Location *struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
						Name      string  `json:"name"`
						Address   string  `json:"address"`
					} `json:"location"`
					Button *struct {
						Text    string `json:"text"`
						Payload string `json:"payload"`
					} `json:"button"`
					Interactive struct {
						Type        string `json:"type"`
						ButtonReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"button_reply,omitempty"`
						ListReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"list_reply,omitempty"`
					} `json:"interactive,omitempty"`
					Errors []struct {
						Code  int    `json:"code"`
						Title string `json:"title"`
					} `json:"errors"`
				} `json:"messages"`
				Statuses []struct {
					ID           string `json:"id"`
					RecipientID  string `json:"recipient_id"`
					Status       string `json:"status"`
					Timestamp    string `json:"timestamp"`
					Type         string `json:"type"`
					Conversation *struct {
						ID     string `json:"id"`
						Origin *struct {
							Type string `json:"type"`
						} `json:"origin"`
					} `json:"conversation"`
					Pricing *struct {
						PricingModel string `json:"pricing_model"`
						Billable     bool   `json:"billable"`
						Category     string `json:"category"`
					} `json:"pricing"`
					Errors []struct {
						Code  int    `json:"code"`
						Title string `json:"title"`
					} `json:"errors"`
				} `json:"statuses"`
				Errors []struct {
					Code  int    `json:"code"`
					Title string `json:"title"`
				} `json:"errors"`
			} `json:"value"`
		} `json:"changes"`
		Messaging []struct {
			Sender    Sender `json:"sender"`
			Recipient User   `json:"recipient"`
			Timestamp int64  `json:"timestamp"`

			OptIn *struct {
				Ref     string `json:"ref"`
				UserRef string `json:"user_ref"`
			} `json:"optin"`

			Referral *struct {
				Ref    string `json:"ref"`
				Source string `json:"source"`
				Type   string `json:"type"`
				AdID   string `json:"ad_id"`
			} `json:"referral"`

			Postback *struct {
				MID      string `json:"mid"`
				Title    string `json:"title"`
				Payload  string `json:"payload"`
				Referral struct {
					Ref    string `json:"ref"`
					Source string `json:"source"`
					Type   string `json:"type"`
					AdID   string `json:"ad_id"`
				} `json:"referral"`
			} `json:"postback"`

			Message *struct {
				IsEcho      bool   `json:"is_echo"`
				MID         string `json:"mid"`
				Text        string `json:"text"`
				IsDeleted   bool   `json:"is_deleted"`
				Attachments []struct {
					Type    string `json:"type"`
					Payload *struct {
						URL         string `json:"url"`
						StickerID   int64  `json:"sticker_id"`
						Coordinates *struct {
							Lat  float64 `json:"lat"`
							Long float64 `json:"long"`
						} `json:"coordinates"`
					}
				} `json:"attachments"`
			} `json:"message"`

			Delivery *struct {
				MIDs      []string `json:"mids"`
				Watermark int64    `json:"watermark"`
			} `json:"delivery"`
		} `json:"messaging"`
	} `json:"entry"`
}

type WACMTResponse struct {
	Messages []*struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

type WACMTMedia struct {
	ID       string `json:"id,omitempty"`
	Link     string `json:"link,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type WACMTSection struct {
	Title string            `json:"title,omitempty"`
	Rows  []WACMTSectionRow `json:"rows" validate:"required"`
}

type WACMTSectionRow struct {
	ID          string `json:"id" validate:"required"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type WACMTButton struct {
	Type  string `json:"type" validate:"required"`
	Reply struct {
		ID    string `json:"id" validate:"required"`
		Title string `json:"title" validate:"required"`
	} `json:"reply" validate:"required"`
}

type WACParam struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type WACComponent struct {
	Type    string      `json:"type"`
	SubType string      `json:"sub_type"`
	Index   string      `json:"index"`
	Params  []*WACParam `json:"parameters"`
}

type WACText struct {
	Body       string `json:"body"`
	PreviewURL bool   `json:"preview_url"`
}

type WACLanguage struct {
	Policy string `json:"policy"`
	Code   string `json:"code"`
}

type WACTemplate struct {
	Name       string          `json:"name"`
	Language   *WACLanguage    `json:"language"`
	Components []*WACComponent `json:"components"`
}

type WACInteractive struct {
	Type   string `json:"type"`
	Header *struct {
		Type     string      `json:"type"`
		Text     string      `json:"text,omitempty"`
		Video    *WACMTMedia `json:"video,omitempty"`
		Image    *WACMTMedia `json:"image,omitempty"`
		Document *WACMTMedia `json:"document,omitempty"`
	} `json:"header,omitempty"`
	Body struct {
		Text string `json:"text"`
	} `json:"body" validate:"required"`
	Footer *struct {
		Text string `json:"text"`
	} `json:"footer,omitempty"`
	Action *struct {
		Button   string         `json:"button,omitempty"`
		Sections []WACMTSection `json:"sections,omitempty"`
		Buttons  []WACMTButton  `json:"buttons,omitempty"`
	} `json:"action,omitempty"`
}

type WACMTPayload struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type"`
	To               string `json:"to"`
	Type             string `json:"type"`

	Text *WACText `json:"text,omitempty"`

	Document *WACMTMedia `json:"document,omitempty"`
	Image    *WACMTMedia `json:"image,omitempty"`
	Audio    *WACMTMedia `json:"audio,omitempty"`
	Video    *WACMTMedia `json:"video,omitempty"`

	Interactive *WACInteractive `json:"interactive,omitempty"`

	Template *WACTemplate `json:"template,omitempty"`
}
