package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
)

var apiURL = "https://api.telegram.org"

// see https://core.telegram.org/bots/api#sending-files
var mediaSupport = map[handlers.MediaType]handlers.MediaTypeSupport{
	handlers.MediaTypeImage:       {MaxBytes: 10 * 1024 * 1024},
	handlers.MediaTypeAudio:       {MaxBytes: 50 * 1024 * 1024},
	handlers.MediaTypeVideo:       {MaxBytes: 50 * 1024 * 1024},
	handlers.MediaTypeApplication: {Types: []string{"application/pdf"}, MaxBytes: 50 * 1024 * 1024},
}

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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, handlers.JSONPayload(h, h.receiveMessage))
	return nil
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, payload *moPayload, clog *courier.ChannelLog) ([]courier.Event, error) {
	// no message? ignore this
	if payload.Message.MessageID == 0 {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, "Ignoring request, no message")
	}

	// create our date from the timestamp
	date := time.Unix(payload.Message.Date, 0).UTC()

	// create our URN
	urn, err := urns.NewFromParts(urns.Telegram.Prefix, strconv.FormatInt(payload.Message.From.ContactID, 10), nil, strings.ToLower(payload.Message.From.Username))
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our name from first and last
	name := handlers.NameFromFirstLastUsername(payload.Message.From.FirstName, payload.Message.From.LastName, payload.Message.From.Username)

	// our text is either "text" or "caption" (or empty)
	text := payload.Message.Text

	// this is a start command, trigger a new conversation
	if text == "/start" {
		event := h.Backend().NewChannelEvent(channel, courier.EventTypeNewConversation, urn, clog).WithContactName(name).WithOccurredOn(date)
		err = h.Backend().WriteChannelEvent(ctx, event, clog)
		if err != nil {
			return nil, err
		}
		return []courier.Event{event}, courier.WriteChannelEventSuccess(w, event)
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
		mediaURL, err = h.resolveFileID(ctx, channel, photo.FileID, clog)
	} else if payload.Message.Video != nil {
		mediaURL, err = h.resolveFileID(ctx, channel, payload.Message.Video.FileID, clog)
	} else if payload.Message.Voice != nil {
		mediaURL, err = h.resolveFileID(ctx, channel, payload.Message.Voice.FileID, clog)
	} else if payload.Message.Sticker != nil {
		mediaURL, err = h.resolveFileID(ctx, channel, payload.Message.Sticker.Thumb.FileID, clog)
	} else if payload.Message.Document != nil {
		mediaURL, err = h.resolveFileID(ctx, channel, payload.Message.Document.FileID, clog)
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
	if err != nil && text == "" {
		return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, w, r, fmt.Sprintf("unable to resolve file: %s", err.Error()))
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, text, fmt.Sprintf("%d", payload.Message.MessageID), clog).WithReceivedOn(date).WithContactName(name)

	if mediaURL != "" {
		msg.WithAttachment(mediaURL)
	}
	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

type mtResponse struct {
	Ok          bool   `json:"ok" validate:"required"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Result      struct {
		MessageID int64 `json:"message_id"`
	} `json:"result"`
}

func (h *handler) sendMsgPart(msg courier.MsgOut, token, path string, form url.Values, keyboard *ReplyKeyboardMarkup, clog *courier.ChannelLog) (string, error) {
	// either include or remove our keyboard
	form.Add("parse_mode", "Markdown")
	if keyboard == nil {
		form.Add("reply_markup", `{"remove_keyboard":true}`)
	} else {
		form.Add("reply_markup", string(jsonx.MustMarshal(keyboard)))
	}

	sendURL := fmt.Sprintf("%s/bot%s/%s", apiURL, token, path)
	req, err := http.NewRequest(http.MethodPost, sendURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, respBody, _ := h.RequestHTTP(req, clog)
	if err != nil || resp.StatusCode/100 == 5 {
		return "", courier.ErrConnectionFailed
	}

	response := &mtResponse{}
	err = json.Unmarshal(respBody, response)

	if err != nil || resp.StatusCode/100 != 2 || !response.Ok {
		if response.ErrorCode == 403 && response.Description == "Forbidden: bot was blocked by the user" {
			return "", courier.ErrContactStopped
		} else if response.ErrorCode > 0 {
			return "", courier.ErrFailedWithReason(strconv.Itoa(response.ErrorCode), response.Description)
		}

		return "", courier.ErrResponseStatus
	}

	if response.Result.MessageID > 0 {
		return strconv.FormatInt(response.Result.MessageID, 10), nil
	}
	return "", courier.ErrResponseUnexpected
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	authToken := msg.Channel().StringConfigForKey(courier.ConfigAuthToken, "")
	if authToken == "" {
		return courier.ErrChannelConfig
	}

	attachments, err := handlers.ResolveAttachments(ctx, h.Backend(), msg.Attachments(), mediaSupport, true, clog)
	if err != nil {
		return fmt.Errorf("error resolving attachments: %w", err)
	}

	// we only caption if there is only a single attachment
	caption := ""
	if len(attachments) == 1 {
		caption = msg.Text()
	}

	// figure out whether we have a keyboard to send as well
	qrs := msg.QuickReplies()
	var keyboard *ReplyKeyboardMarkup
	if len(qrs) > 0 {
		keyboard = NewKeyboardFromReplies(qrs)
	}

	// if we have text, send that if we aren't sending it as a caption
	if msg.Text() != "" && caption == "" {
		var msgKeyBoard *ReplyKeyboardMarkup
		if len(attachments) == 0 {
			msgKeyBoard = keyboard
		}

		form := url.Values{"chat_id": []string{msg.URN().Path()}, "text": []string{msg.Text()}}

		externalID, err := h.sendMsgPart(msg, authToken, "sendMessage", form, msgKeyBoard, clog)
		if err != nil {
			return err
		}

		res.AddExternalID(externalID)
	}

	// send each attachment
	for i, attachment := range attachments {
		var attachmentKeyBoard *ReplyKeyboardMarkup
		if i == len(msg.Attachments())-1 {
			attachmentKeyBoard = keyboard
		}

		switch attachment.Type {
		case handlers.MediaTypeImage:
			form := url.Values{
				"chat_id": []string{msg.URN().Path()},
				"photo":   []string{attachment.URL},
				"caption": []string{caption},
			}
			externalID, err := h.sendMsgPart(msg, authToken, "sendPhoto", form, attachmentKeyBoard, clog)
			if err != nil {
				return err
			}
			res.AddExternalID(externalID)

		case handlers.MediaTypeVideo:
			form := url.Values{
				"chat_id": []string{msg.URN().Path()},
				"video":   []string{attachment.URL},
				"caption": []string{caption},
			}
			externalID, err := h.sendMsgPart(msg, authToken, "sendVideo", form, attachmentKeyBoard, clog)
			if err != nil {
				return err
			}
			res.AddExternalID(externalID)

		case handlers.MediaTypeAudio:
			form := url.Values{
				"chat_id": []string{msg.URN().Path()},
				"audio":   []string{attachment.URL},
				"caption": []string{caption},
			}
			externalID, err := h.sendMsgPart(msg, authToken, "sendAudio", form, attachmentKeyBoard, clog)
			if err != nil {
				return err
			}
			res.AddExternalID(externalID)

		case handlers.MediaTypeApplication:
			form := url.Values{
				"chat_id":  []string{msg.URN().Path()},
				"document": []string{attachment.URL},
				"caption":  []string{caption},
			}
			externalID, err := h.sendMsgPart(msg, authToken, "sendDocument", form, attachmentKeyBoard, clog)
			if err != nil {
				return err
			}
			res.AddExternalID(externalID)

		default:
			clog.Error(courier.ErrorMediaUnsupported(attachment.ContentType))
		}
	}

	return nil
}

type fileResponse struct {
	Ok          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Result      struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
}

func (h *handler) resolveFileID(ctx context.Context, channel courier.Channel, fileID string, clog *courier.ChannelLog) (string, error) {
	confAuth := channel.ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return "", fmt.Errorf("invalid auth token config")
	}

	fileURL := fmt.Sprintf("%s/bot%s/getFile", apiURL, authToken)

	form := url.Values{}
	form.Set("file_id", fileID)

	req, err := http.NewRequest(http.MethodPost, fileURL, strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	if err != nil {
		courier.LogRequestError(req, channel, err)
	}

	resp, respBody, _ := h.RequestHTTP(req, clog)

	respPayload := &fileResponse{}
	err = json.Unmarshal(respBody, respPayload)
	if err != nil {
		clog.Error(courier.ErrorResponseUnparseable("JSON"))
		return "", errors.New("unable to resolve file")
	}

	if resp.StatusCode/100 != 2 || respPayload.ErrorCode != 0 {
		clog.Error(courier.ErrorExternal(strconv.Itoa(respPayload.ErrorCode), respPayload.Description))
		return "", errors.New("unable to resolve file")
	}

	if !respPayload.Ok {
		return "", fmt.Errorf("file id '%s' not present", fileID)
	}

	filePath := respPayload.Result.FilePath
	if filePath == "" {
		return "", fmt.Errorf("no 'result.file_path' in response")
	}
	// return the URL
	return fmt.Sprintf("%s/file/bot%s/%s", apiURL, authToken, filePath), nil
}

type moFile struct {
	FileID   string `json:"file_id"    validate:"required"`
	FileSize int    `json:"file_size"`
}

type moLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

//	{
//	  "update_id": 174114370,
//	  "message": {
//	    "message_id": 41,
//	    "from": {
//	      "id": 3527065,
//	      "first_name": "Nic",
//	      "last_name": "Pottier",
//	      "username": "nicpottier"
//	    },
//	    "chat": {
//	      "id": 3527065,
//	      "first_name": "Nic",
//	      "last_name": "Pottier",
//	      "type": "private"
//	    },
//	    "date": 1454119029,
//	    "text": "Hello World"
//	   }
//	}
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
