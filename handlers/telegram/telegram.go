package telegram

import (
	"context"
	"encoding/json"
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

var apiURL = "https://api.telegram.org"

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("TG"), "Telegram")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateJSON(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// no message? ignore this
	if payload.Message.MessageID == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
	}

	// create our date from the timestamp
	date := time.Unix(payload.Message.Date, 0).UTC()

	// create our URN
	urn, err := urns.NewTelegramURN(payload.Message.From.ContactID, strings.ToLower(payload.Message.From.Username))
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our name from first and last
	name := handlers.NameFromFirstLastUsername(payload.Message.From.FirstName, payload.Message.From.LastName, payload.Message.From.Username)

	// our text is either "text" or "caption" (or empty)
	text := payload.Message.Text

	// this is a start command, trigger a new conversation
	if text == "/start" {
		event := h.Backend().NewChannelEvent(channel, courier.NewConversation, urn).WithContactName(name).WithOccurredOn(date)
		err = h.Backend().WriteChannelEvent(ctx, event)
		if err != nil {
			return nil, err
		}
		return []courier.Event{event}, courier.WriteChannelEventSuccess(ctx, w, r, event)
	}

	// normal message of some kind
	if text == "" && payload.Message.Caption != "" {
		text = payload.Message.Caption
	}

	// deal with attachments
	mediaURL := ""
	if len(payload.Message.Photo) > 0 {
		// grab the largest photo less than 100k
		photo := payload.Message.Photo[0]
		for i := 1; i < len(payload.Message.Photo); i++ {
			if payload.Message.Photo[i].FileSize > 100000 {
				break
			}
			photo = payload.Message.Photo[i]
		}
		mediaURL, err = resolveFileID(channel, photo.FileID)
	} else if payload.Message.Video != nil {
		mediaURL, err = resolveFileID(channel, payload.Message.Video.FileID)
	} else if payload.Message.Voice != nil {
		mediaURL, err = resolveFileID(channel, payload.Message.Voice.FileID)
	} else if payload.Message.Sticker != nil {
		mediaURL, err = resolveFileID(channel, payload.Message.Sticker.Thumb.FileID)
	} else if payload.Message.Document != nil {
		mediaURL, err = resolveFileID(channel, payload.Message.Document.FileID)
	} else if payload.Message.Venue != nil {
		text = utils.JoinNonEmpty(", ", payload.Message.Venue.Title, payload.Message.Venue.Address)
		mediaURL = fmt.Sprintf("geo:%f,%f", payload.Message.Location.Latitude, payload.Message.Location.Longitude)
	} else if payload.Message.Location != nil {
		text = fmt.Sprintf("%f,%f", payload.Message.Location.Latitude, payload.Message.Location.Longitude)
		mediaURL = fmt.Sprintf("geo:%f,%f", payload.Message.Location.Latitude, payload.Message.Location.Longitude)
	} else if payload.Message.Contact != nil {
		phone := ""
		if payload.Message.Contact.PhoneNumber != "" {
			phone = fmt.Sprintf("(%s)", payload.Message.Contact.PhoneNumber)
		}
		text = utils.JoinNonEmpty(" ", payload.Message.Contact.FirstName, payload.Message.Contact.LastName, phone)
	}

	// we had an error downloading media
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, errors.WrapPrefix(err, "error retrieving media", 0))
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(fmt.Sprintf("%d", payload.Message.MessageID)).WithContactName(name)

	if mediaURL != "" {
		msg.WithAttachment(mediaURL)
	}
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

func (h *handler) sendMsgPart(msg courier.Msg, token string, path string, form url.Values, replies string) (string, *courier.ChannelLog, error) {
	// either include or remove our keyboard depending on whether we have quick replies
	if replies == "" {
		form.Add("reply_markup", `{"remove_keyboard":true}`)
	} else {
		form.Add("reply_markup", replies)
	}

	sendURL := fmt.Sprintf("%s/bot%s/%s", apiURL, token, path)
	req, _ := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
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
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
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

	// whether we encountered any errors sending any parts
	hasError := true

	// figure out whether we have a keyboard to send as well
	qrs := msg.QuickReplies()
	replies := ""

	if len(qrs) > 0 {
		keys := make([]moKey, len(qrs))
		for i, qr := range qrs {
			keys[i].Text = qr
		}

		tk := moKeyboard{true, true, [][]moKey{keys}}
		replyBytes, err := json.Marshal(tk)
		if err != nil {
			return nil, err
		}
		replies = string(replyBytes)
	}

	// if we have text, send that if we aren't sending it as a caption
	if msg.Text() != "" && caption == "" {
		form := url.Values{
			"chat_id": []string{msg.URN().Path()},
			"text":    []string{msg.Text()},
		}

		externalID, log, err := h.sendMsgPart(msg, authToken, "sendMessage", form, replies)
		status.SetExternalID(externalID)
		hasError = err != nil
		status.AddLog(log)

		// clear our replies, they've been sent
		replies = ""
	}

	// send each attachment
	for _, attachment := range msg.Attachments() {
		mediaType, mediaURL := handlers.SplitAttachment(attachment)
		switch strings.Split(mediaType, "/")[0] {
		case "image":
			form := url.Values{
				"chat_id": []string{msg.URN().Path()},
				"photo":   []string{mediaURL},
				"caption": []string{caption},
			}
			externalID, log, err := h.sendMsgPart(msg, authToken, "sendPhoto", form, replies)
			status.SetExternalID(externalID)
			hasError = err != nil
			status.AddLog(log)

		case "video":
			form := url.Values{
				"chat_id": []string{msg.URN().Path()},
				"video":   []string{mediaURL},
				"caption": []string{caption},
			}
			externalID, log, err := h.sendMsgPart(msg, authToken, "sendVideo", form, replies)
			status.SetExternalID(externalID)
			hasError = err != nil
			status.AddLog(log)

		case "audio":
			form := url.Values{
				"chat_id": []string{msg.URN().Path()},
				"audio":   []string{mediaURL},
				"caption": []string{caption},
			}
			externalID, log, err := h.sendMsgPart(msg, authToken, "sendAudio", form, replies)
			status.SetExternalID(externalID)
			hasError = err != nil
			status.AddLog(log)

		default:
			status.AddLog(courier.NewChannelLog("Unknown media type: "+mediaType, msg.Channel(), msg.ID(), "", "", courier.NilStatusCode,
				"", "", time.Duration(0), fmt.Errorf("unknown media type: %s", mediaType)))
			hasError = true

		}

		// clear our replies, we only send it on the first message
		replies = ""
	}

	if !hasError {
		status.SetStatus(courier.MsgWired)
	}

	return status, nil
}

func resolveFileID(channel courier.Channel, fileID string) (string, error) {
	confAuth := channel.ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return "", fmt.Errorf("invalid auth token config")
	}

	fileURL := fmt.Sprintf("%s/bot%s/getFile", apiURL, authToken)

	form := url.Values{}
	form.Set("file_id", fileID)

	req, _ := http.NewRequest(http.MethodPost, fileURL, strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	rr, err := utils.MakeHTTPRequest(req)
	if err != nil {
		return "", err
	}

	// was this request successful?
	ok, err := jsonparser.GetBoolean([]byte(rr.Body), "ok")
	if err != nil {
		return "", errors.Errorf("no 'ok' in response")
	}

	if !ok {
		return "", errors.Errorf("file id '%s' not present", fileID)
	}

	// grab the path for our file
	filePath, err := jsonparser.GetString([]byte(rr.Body), "result", "file_path")
	if err != nil {
		return "", errors.Errorf("no 'result.file_path' in response")
	}

	// return the URL
	return fmt.Sprintf("%s/file/bot%s/%s", apiURL, authToken, filePath), nil
}

type moKeyboard struct {
	ResizeKeyboard  bool      `json:"resize_keyboard"`
	OneTimeKeyboard bool      `json:"one_time_keyboard"`
	Keyboard        [][]moKey `json:"keyboard"`
}

type moKey struct {
	Text string `json:"text"`
}

type moFile struct {
	FileID   string `json:"file_id"    validate:"required"`
	FileSize int    `json:"file_size"`
}

type moLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// {
// 	"update_id": 174114370,
// 	"message": {
// 	  "message_id": 41,
//      "from": {
// 		  "id": 3527065,
// 		  "first_name": "Nic",
// 		  "last_name": "Pottier",
//        "username": "nicpottier"
// 	    },
//     "chat": {
//       "id": 3527065,
// 		 "first_name": "Nic",
//       "last_name": "Pottier",
//       "type": "private"
//     },
// 	   "date": 1454119029,
//     "text": "Hello World"
// 	 }
// }
type moPayload struct {
	UpdateID int64 `json:"update_id" validate:"required"`
	Message  struct {
		MessageID int64 `json:"message_id"`
		From      struct {
			ContactID int64  `json:"id"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Username  string `json:"username"`
		} `json:"from"`
		Date    int64  `json:"date"`
		Text    string `json:"text"`
		Caption string `json:"caption"`
		Sticker *struct {
			Thumb moFile `json:"thumb"`
		} `json:"sticker"`
		Photo    []moFile    `json:"photo"`
		Video    *moFile     `json:"video"`
		Voice    *moFile     `json:"voice"`
		Document *moFile     `json:"document"`
		Location *moLocation `json:"location"`
		Venue    *struct {
			Location *moLocation `json:"location"`
			Title    string      `json:"title"`
			Address  string      `json:"address"`
		}
		Contact *struct {
			PhoneNumber string `json:"phone_number"`
			FirstName   string `json:"first_name"`
			LastName    string `json:"last_name"`
		}
	} `json:"message"`
}
