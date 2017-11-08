package facebook

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/go-errors/errors"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
)

var facebookAPIURL = "https://graph.facebook.com/v2.6/me/messages"

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

const (
	maxLength = 640 // Facebook API says 640 is max for the body
)

// NewHandler returns a new TelegramHandler ready to be registered
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("TG"), "Telegram")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return s.AddReceiveMsgRoute(h, http.MethodPost, "receive", h.ReceiveMessage)
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.ReceiveEvent, error) {
	te := &telegramEnvelope{}
	err := handlers.DecodeAndValidateJSON(te, r)
	if err != nil {
		return nil, courier.WriteError(w, r, err)
	}

	// no message? ignore this
	if te.Message.MessageID == 0 {
		return nil, courier.WriteIgnored(w, r, "Ignoring request, no message")
	}

	// create our date from the timestamp
	date := time.Unix(te.Message.Date, 0).UTC()

	// create our URN
	urn := urns.NewTelegramURN(te.Message.From.ContactID, te.Message.From.Username)

	// build our name from first and last
	name := handlers.NameFromFirstLastUsername(te.Message.From.FirstName, te.Message.From.LastName, te.Message.From.Username)

	// our text is either "text" or "caption" (or empty)
	text := te.Message.Text

	// this is a start command, trigger a new conversation
	if text == "/start" {
		event := h.Backend().NewChannelEvent(channel, courier.NewConversation, urn).WithContactName(name).WithOccurredOn(date)
		err = h.Backend().WriteChannelEvent(event)
		if err != nil {
			return nil, err
		}
		return []courier.ReceiveEvent{event}, courier.WriteChannelEventSuccess(w, r, event)
	}

	// normal message of some kind
	if text == "" && te.Message.Caption != "" {
		text = te.Message.Caption
	}

	// deal with attachments
	mediaURL := ""
	if len(te.Message.Photo) > 0 {
		// grab the largest photo less than 100k
		photo := te.Message.Photo[0]
		for i := 1; i < len(te.Message.Photo); i++ {
			if te.Message.Photo[i].FileSize > 100000 {
				break
			}
			photo = te.Message.Photo[i]
		}
		mediaURL, err = resolveFileID(channel, photo.FileID)
	} else if te.Message.Video != nil {
		mediaURL, err = resolveFileID(channel, te.Message.Video.FileID)
	} else if te.Message.Voice != nil {
		mediaURL, err = resolveFileID(channel, te.Message.Voice.FileID)
	} else if te.Message.Sticker != nil {
		mediaURL, err = resolveFileID(channel, te.Message.Sticker.Thumb.FileID)
	} else if te.Message.Document != nil {
		mediaURL, err = resolveFileID(channel, te.Message.Document.FileID)
	} else if te.Message.Venue != nil {
		text = utils.JoinNonEmpty(", ", te.Message.Venue.Title, te.Message.Venue.Address)
		mediaURL = fmt.Sprintf("geo:%f,%f", te.Message.Location.Latitude, te.Message.Location.Longitude)
	} else if te.Message.Location != nil {
		text = fmt.Sprintf("%f,%f", te.Message.Location.Latitude, te.Message.Location.Longitude)
		mediaURL = fmt.Sprintf("geo:%f,%f", te.Message.Location.Latitude, te.Message.Location.Longitude)
	} else if te.Message.Contact != nil {
		phone := ""
		if te.Message.Contact.PhoneNumber != "" {
			phone = fmt.Sprintf("(%s)", te.Message.Contact.PhoneNumber)
		}
		text = utils.JoinNonEmpty(" ", te.Message.Contact.FirstName, te.Message.Contact.LastName, phone)
	}

	// we had an error downloading media
	if err != nil {
		return nil, courier.WriteError(w, r, errors.WrapPrefix(err, "error retrieving media", 0))
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(fmt.Sprintf("%d", te.Message.MessageID)).WithContactName(name)

	if mediaURL != "" {
		msg.WithAttachment(mediaURL)
	}

	// queue our message
	err = h.Backend().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []courier.ReceiveEvent{msg}, courier.WriteMsgSuccess(w, r, msg)
}

func (h *handler) sendMsgPart(msg courier.Msg, token string, path string, form url.Values) (string, *courier.ChannelLog, error) {
	sendURL := fmt.Sprintf("%s/bot%s/%s", telegramAPIURL, token, path)
	req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rr, err := utils.MakeHTTPRequest(req)

	// build our channel log
	log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)

	// was this request successful?
	ok, err := jsonparser.GetBoolean([]byte(rr.Body), "ok")
	if err != nil || !ok {
		return "", log, errors.Errorf("response not 'ok'")
	}

	// grab our message id
	externalID, err := jsonparser.GetInt([]byte(rr.Body), "result", "message_id")
	if err != nil {
		return "", log, errors.Errorf("no 'result.message_id' in response")
	}

	return strconv.FormatInt(externalID, 10), log, nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	confAuth := msg.Channel().ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return nil, fmt.Errorf("invalid auth token config")
	}

	// we only caption if there is only a single attachment
	caption := ""
	if len(msg.Attachments()) == 1 {
		caption = msg.Text()
	}

	// the status that will be written for this message
	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(courier.Text, maxLength)
	for _, part := range parts {
		envelope := facebookEnvelope{
			Message{
				Text: 
			}
		}

	}

	// whether we encountered any errors sending any parts
	hasError := true

	// if we have text, send that if we aren't sending it as a caption
	if msg.Text() != "" && caption == "" {
		form := url.Values{
			"chat_id": []string{msg.URN().Path()},
			"text":    []string{msg.Text()},
		}
		externalID, log, err := h.sendMsgPart(msg, authToken, "sendMessage", form)
		status.SetExternalID(externalID)
		hasError = err != nil
		status.AddLog(log)
	}

	// send each attachment
	for _, attachment := range msg.Attachments() {
		mediaType, mediaURL := courier.SplitAttachment(attachment)
		switch strings.Split(mediaType, "/")[0] {
		case "image":
			form := url.Values{
				"chat_id": []string{msg.URN().Path()},
				"photo":   []string{mediaURL},
				"caption": []string{caption},
			}
			externalID, log, err := h.sendMsgPart(msg, authToken, "sendPhoto", form)
			status.SetExternalID(externalID)
			hasError = err != nil
			status.AddLog(log)

		case "video":
			form := url.Values{
				"chat_id": []string{msg.URN().Path()},
				"video":   []string{mediaURL},
				"caption": []string{caption},
			}
			externalID, log, err := h.sendMsgPart(msg, authToken, "sendVideo", form)
			status.SetExternalID(externalID)
			hasError = err != nil
			status.AddLog(log)

		case "audio":
			form := url.Values{
				"chat_id": []string{msg.URN().Path()},
				"audio":   []string{mediaURL},
				"caption": []string{caption},
			}
			externalID, log, err := h.sendMsgPart(msg, authToken, "sendAudio", form)
			status.SetExternalID(externalID)
			hasError = err != nil
			status.AddLog(log)

		default:
			status.AddLog(courier.NewChannelLog("Unknown media type: "+mediaType, msg.Channel(), msg.ID(), "", "", courier.NilStatusCode,
				"", "", time.Duration(0), fmt.Errorf("unknown media type: %s", mediaType)))
			hasError = true
		}
	}

	if !hasError {
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

// {
//    "recipient_id": "1008372609250235",
//    "message_id": "mid.1456970487936:c34767dfe57ee6e339"
// }
type facebookResponse struct {
	RecipientID string `json:"recipient_id"`
	MessageID   string `json:"message_id"`
}

// {
//    "recipient": {
// 	    "id": "10041885"
//    }
// 	  "message": {
//      "text": "Hello World",
//      "attachment": {
// 	      "type": "video",
//        "payload": {
//          "url": "https://foo.bar/video.mpeg"
//        }
//      }
//    }
// }

type facebookMessage   struct {
	Text       string `json:"text"`
	Attachment *struct {
		Type    string `json:"type"`
		Payload struct {
			URL string `json:"url"`
		} `json:"payload"`
	} `json:"attachment"`
}

type plainRecipient struct {
	ID string `json:"id"`
}

type referralRecipient struct {
	UserReferral string `json:"user_ref"`
}

type facebookEnvelope struct {
	Recipient interface{} `json:"recipient"`
	Message facebookMessage `json:"message"`	
}

