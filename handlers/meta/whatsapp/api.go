package whatsapp

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/core/models"
	"github.com/nyaruka/gocommon/urns"
)

// see https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/payload-examples#message-status-updates
var StatusMapping = map[string]models.MsgStatus{
	"sent":      models.MsgStatusSent,
	"delivered": models.MsgStatusDelivered,
	"read":      models.MsgStatusRead,
	"failed":    models.MsgStatusFailed,
}

var IgnoreStatuses = map[string]bool{
	"deleted": true,
}

var WACThrottlingErrorCodes = []int{4, 80007, 130429, 131048, 131056, 133016}

// see https://developers.facebook.com/docs/whatsapp/cloud-api/reference/media#example-2
type MOMedia struct {
	Caption  string `json:"caption"`
	Filename string `json:"filename"`
	ID       string `json:"id"`
	Mimetype string `json:"mime_type"`
	SHA256   string `json:"sha256"`
}

type WAError struct {
	Code  int    `json:"code"`
	Title string `json:"title"`
}

func (e WAError) ErrorChannelLog(clog *courier.ChannelLog) {
	clog.Error(courier.ErrorExternal(strconv.Itoa(e.Code), e.Title))
}

type WAContact struct {
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
	WaID string `json:"wa_id"`
}

type WAMessage struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id,omitempty"`
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
	Errors []WAError `json:"errors"`
}

func (m WAMessage) ExtractData(clog *courier.ChannelLog) (time.Time, urns.URN, string, string, string, error, error) {
	var err error
	var finalErr error
	var date time.Time
	var urn urns.URN
	var text string
	var mediaURL string
	var mediaID string

	// create our date from the timestamp
	ts, err := strconv.ParseInt(m.Timestamp, 10, 64)
	if err != nil {
		finalErr = fmt.Errorf("invalid timestamp: %s", m.Timestamp)
		return date, urn, text, mediaURL, mediaID, err, finalErr
	}
	date = parseTimestamp(ts)

	urn, err = urns.New(urns.WhatsApp, m.From)
	if err != nil {
		finalErr = errors.New("invalid whatsapp id")
		return date, urn, text, mediaURL, mediaID, err, finalErr
	}

	for _, msgError := range m.Errors {
		msgError.ErrorChannelLog(clog)
	}

	text, mediaURL, mediaID, err = m.ExtractTextAndMedia()

	return date, urn, text, mediaURL, mediaID, err, finalErr
}

func parseTimestamp(ts int64) time.Time {
	// sometimes Facebook sends timestamps in milliseconds rather than seconds
	if ts >= 1_000_000_000_000 {
		slog.Error("meta webhook timestamp is in milliseconds instead of seconds", "timestamp", ts)
		return time.Unix(0, ts*1000000).UTC()
	}
	return time.Unix(ts, 0).UTC()
}

func (m WAMessage) ExtractTextAndMedia() (string, string, string, error) {
	text := ""
	mediaURL := ""
	mediaID := ""
	var err error

	if m.Type == "text" {
		text = m.Text.Body
	} else if m.Type == "audio" && m.Audio != nil {
		text = m.Audio.Caption
		mediaID = m.Audio.ID
	} else if m.Type == "voice" && m.Voice != nil {
		text = m.Voice.Caption
		mediaID = m.Voice.ID
	} else if m.Type == "button" && m.Button != nil {
		text = m.Button.Text
	} else if m.Type == "document" && m.Document != nil {
		text = m.Document.Caption
		mediaID = m.Document.ID
	} else if m.Type == "image" && m.Image != nil {
		text = m.Image.Caption
		mediaID = m.Image.ID
	} else if m.Type == "video" && m.Video != nil {
		text = m.Video.Caption
		mediaID = m.Video.ID
	} else if m.Type == "location" && m.Location != nil {
		mediaURL = fmt.Sprintf("geo:%f,%f", m.Location.Latitude, m.Location.Longitude)
	} else if m.Type == "interactive" && m.Interactive.Type == "button_reply" {
		text = m.Interactive.ButtonReply.Title
	} else if m.Type == "interactive" && m.Interactive.Type == "list_reply" {
		text = m.Interactive.ListReply.Title
	} else {
		// we received a message type we do not support.
		err = fmt.Errorf("unsupported message type %s", m.Type)
	}

	return text, mediaURL, mediaID, err

}

type WAStatus struct {
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
	Errors []WAError `json:"errors"`
}

type WAMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

type Change struct {
	Field string `json:"field"`
	Value struct {
		MessagingProduct string      `json:"messaging_product"`
		Metadata         *WAMetadata `json:"metadata"`
		Contacts         []WAContact `json:"contacts"`
		Messages         []WAMessage `json:"messages"`
		Statuses         []WAStatus  `json:"statuses"`
		Errors           []WAError   `json:"errors"`
	} `json:"value"`
}

// see https://developers.facebook.com/docs/whatsapp/cloud-api/guides/send-messages#media-messages
type Media struct {
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
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Payload string `json:"payload,omitempty"`
	Image   *struct {
		Link string `json:"link,omitempty"`
	} `json:"image,omitempty"`
	Video *struct {
		Link string `json:"link,omitempty"`
	} `json:"video,omitempty"`
	Document *struct {
		Link     string `json:"link,omitempty"`
		Filename string `json:"filename,omitempty"`
	} `json:"document,omitempty"`
}

type Component struct {
	Type    string   `json:"type"`
	SubType string   `json:"sub_type,omitempty"`
	Index   string   `json:"index,omitempty"`
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

// see https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#template-object
// e.g. https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#template-messages
type Template struct {
	Name       string       `json:"name"`
	Language   *Language    `json:"language"`
	Components []*Component `json:"components,omitempty"`
}

// see https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#interactive-object
// e.g. https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#interactive-messages
type Interactive struct {
	Type   string `json:"type"`
	Header *struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		Video    *Media `json:"video,omitempty"`
		Image    *Media `json:"image,omitempty"`
		Document *Media `json:"document,omitempty"`
	} `json:"header,omitempty"`
	Body struct {
		Text string `json:"text"`
	} `json:"body" validate:"required"`
	Footer *struct {
		Text string `json:"text"`
	} `json:"footer,omitempty"`
	Action *struct {
		Name     string    `json:"name,omitempty"`
		Button   string    `json:"button,omitempty"`
		Sections []Section `json:"sections,omitempty"`
		Buttons  []Button  `json:"buttons,omitempty"`
	} `json:"action,omitempty"`
}

// see https://developers.facebook.com/docs/whatsapp/cloud-api/guides/send-messages#request-syntax
// e.g. https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#message-object
type SendRequest struct {
	MessagingProduct string `json:"messaging_product"`
	RecipientType    string `json:"recipient_type"`
	To               string `json:"to"`
	Type             string `json:"type"`

	Text *Text `json:"text,omitempty"`

	Document *Media `json:"document,omitempty"`
	Image    *Media `json:"image,omitempty"`
	Audio    *Media `json:"audio,omitempty"`
	Video    *Media `json:"video,omitempty"`

	Interactive *Interactive `json:"interactive,omitempty"`

	Template *Template `json:"template,omitempty"`
}

// see https://developers.facebook.com/docs/whatsapp/cloud-api/guides/send-messages#response-syntax
// e.g. https://developers.facebook.com/docs/whatsapp/cloud-api/reference/messages#successful-response
type SendResponse struct {
	Messages []*struct {
		ID string `json:"id"`
	} `json:"messages"`
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}
