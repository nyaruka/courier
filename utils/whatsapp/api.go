package whatsapp

import "github.com/nyaruka/courier"

// API docs https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/payload-examples#message-status-updates
var StatusMapping = map[string]courier.MsgStatus{
	"sent":      courier.MsgStatusSent,
	"delivered": courier.MsgStatusDelivered,
	"read":      courier.MsgStatusDelivered,
	"failed":    courier.MsgStatusFailed,
}

var IgnoreStatuses = map[string]bool{
	"deleted": true,
}

type MsgTemplating struct {
	Template struct {
		Name string `json:"name" validate:"required"`
		UUID string `json:"uuid" validate:"required"`
	} `json:"template" validate:"required,dive"`
	Namespace string   `json:"namespace"`
	Variables []string `json:"variables"`
}

// API docs https://developers.facebook.com/docs/whatsapp/cloud-api/reference/media#example-2
type MOMedia struct {
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	ID       string `json:"id"`
	Mimetype string `json:"mime_type"`
	SHA256   string `json:"sha256"`
}

// API docs https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/components#notification-payload-object
type MOPayload struct {
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
					Image    *MOMedia `json:"image"`
					Audio    *MOMedia `json:"audio"`
					Video    *MOMedia `json:"video"`
					Document *MOMedia `json:"document"`
					Voice    *MOMedia `json:"voice"`
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
			Sender *struct {
				ID      string `json:"id"`
				UserRef string `json:"user_ref,omitempty"`
			} `json:"sender"`
			Recipient *struct {
				ID string `json:"id"`
			} `json:"recipient"`
			Timestamp int64 `json:"timestamp"`

			OptIn *struct {
				Type                          string `json:"type"`
				Payload                       string `json:"payload"`
				NotificationMessagesToken     string `json:"notification_messages_token"`
				NotificationMessagesTimezone  string `json:"notification_messages_timezone"`
				NotificationMessagesFrequency string `json:"notification_messages_frequency"`
				NotificationMessagesStatus    string `json:"notification_messages_status"`
				TokenExpiryTimestamp          int64  `json:"token_expiry_timestamp"`
				UserTokenStatus               string `json:"user_token_status"`
				Title                         string `json:"title"`

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

// API docs https://developers.facebook.com/docs/whatsapp/cloud-api/guides/send-messages#media-messages
type MTMedia struct {
	ID       string `json:"id,omitempty"`
	Link     string `json:"link,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type Section struct {
	Title string       `json:"title,omitempty"`
	Rows  []SectionRow `json:"rows" validate:"required"`
}

type SectionRow struct {
	ID          string `json:"id" validate:"required"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

type Button struct {
	Type  string `json:"type" validate:"required"`
	Reply struct {
		ID    string `json:"id" validate:"required"`
		Title string `json:"title" validate:"required"`
	} `json:"reply" validate:"required"`
}

type Param struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Component struct {
	Type    string   `json:"type"`
	SubType string   `json:"sub_type"`
	Index   string   `json:"index"`
	Params  []*Param `json:"parameters"`
}

type Text struct {
	Body       string `json:"body"`
	PreviewURL bool   `json:"preview_url"`
}

type Language struct {
	Policy string `json:"policy"`
	Code   string `json:"code"`
}

// API docs https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#template-object
// Example https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#template-messages
type Template struct {
	Name       string       `json:"name"`
	Language   *Language    `json:"language"`
	Components []*Component `json:"components"`
}

// API docs https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#interactive-object
// Example https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#interactive-messages
type Interactive struct {
	Type   string `json:"type"`
	Header *struct {
		Type     string   `json:"type"`
		Text     string   `json:"text,omitempty"`
		Video    *MTMedia `json:"video,omitempty"`
		Image    *MTMedia `json:"image,omitempty"`
		Document *MTMedia `json:"document,omitempty"`
	} `json:"header,omitempty"`
	Body struct {
		Text string `json:"text"`
	} `json:"body" validate:"required"`
	Footer *struct {
		Text string `json:"text"`
	} `json:"footer,omitempty"`
	Action *struct {
		Button   string    `json:"button,omitempty"`
		Sections []Section `json:"sections,omitempty"`
		Buttons  []Button  `json:"buttons,omitempty"`
	} `json:"action,omitempty"`
}

// API docs https://developers.facebook.com/docs/whatsapp/cloud-api/guides/send-messages#request-syntax
// Example https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#message-object
type MTPayload struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type"`
	To               string `json:"to"`
	Type             string `json:"type"`

	Text *Text `json:"text,omitempty"`

	Document *MTMedia `json:"document,omitempty"`
	Image    *MTMedia `json:"image,omitempty"`
	Audio    *MTMedia `json:"audio,omitempty"`
	Video    *MTMedia `json:"video,omitempty"`

	Interactive *Interactive `json:"interactive,omitempty"`

	Template *Template `json:"template,omitempty"`
}

// API docs https://developers.facebook.com/docs/whatsapp/cloud-api/guides/send-messages#response-syntax
// Example https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#successful-response
type MTResponse struct {
	Messages []*struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}
