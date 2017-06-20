package telegram

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
)

func init() {
	courier.RegisterHandler(NewHandler())
}

type telegramHandler struct {
	handlers.BaseHandler
}

// NewHandler returns a new TelegramHandler ready to be registered
func NewHandler() courier.ChannelHandler {
	return &telegramHandler{handlers.NewBaseHandler(courier.ChannelType("TG"), "Telegram")}
}

// Initialize is called by the engine once everything is loaded
func (h *telegramHandler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return s.AddReceiveMsgRoute(h, "POST", "receive", h.ReceiveMessage)
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *telegramHandler) ReceiveMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]*courier.Msg, error) {
	te := &telegramEnvelope{}
	err := handlers.DecodeAndValidateJSON(te, r)
	if err != nil {
		return nil, err
	}

	// no message? ignore this
	if te.Message.MessageID == 0 {
		return nil, courier.WriteIgnored(w, r, "Ignoring request, no message")
	}

	// create our date from the timestamp
	date := time.Unix(te.Message.Date, 0).UTC()

	// create our URN
	urn := courier.NewTelegramURN(te.Message.From.ContactID)

	// build our name from first and last
	name := handlers.NameFromFirstLastUsername(te.Message.From.FirstName, te.Message.From.LastName, te.Message.From.Username)

	// our text is either "text" or "caption" (or empty)
	text := te.Message.Text
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
		return nil, errors.WrapPrefix(err, "error retrieving media", 0)
	}

	// build our msg
	msg := courier.NewIncomingMsg(channel, urn, text).WithReceivedOn(date).WithExternalID(fmt.Sprintf("%d", te.Message.MessageID)).WithContactName(name)

	if mediaURL != "" {
		msg.AddAttachment(mediaURL)
	}

	// queue our message
	err = h.Server().WriteMsg(msg)
	if err != nil {
		return nil, err
	}

	return []*courier.Msg{msg}, courier.WriteReceiveSuccess(w, r, msg)
}

func (h *telegramHandler) sendMsgPart(msg *courier.Msg, token string, path string, form url.Values) (string, *courier.ChannelLog, error) {
	sendURL := fmt.Sprintf("%s/bot%s/%s", telegramAPIURL, token, path)
	req, err := http.NewRequest("POST", sendURL, strings.NewReader(form.Encode()))
	rr, err := utils.MakeHTTPRequest(req)

	// build our channel log
	log := courier.NewChannelLogFromRR(msg.Channel, msg.ID, rr)

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
func (h *telegramHandler) SendMsg(msg *courier.Msg) (*courier.MsgStatusUpdate, error) {
	confAuth := msg.Channel.ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return nil, fmt.Errorf("invalid auth token config")
	}

	// we only caption if there is only a single attachment
	caption := ""
	if len(msg.Attachments) == 1 {
		caption = msg.Text
	}

	// the status that will be written for this message
	status := courier.NewStatusUpdateForID(msg.Channel, msg.ID, courier.NilMsgStatus)

	// whether we encountered any errors sending any parts
	hasError := true

	// if we have text, send that if we aren't sending it as a caption
	if msg.Text != "" && caption == "" {
		form := url.Values{
			"chat_id": []string{msg.URN.Path()},
			"text":    []string{msg.Text},
		}
		externalID, log, err := h.sendMsgPart(msg, authToken, "sendMessage", form)
		status.ExternalID = externalID
		hasError = err != nil
		status.AddLog(log)
	}

	// send each attachment
	for _, attachment := range msg.Attachments {
		mediaParts := strings.SplitN(attachment, "/", 2)
		if len(mediaParts) < 2 {
			status.AddLog(courier.NewChannelLog(msg.Channel, msg.ID, "", courier.NilStatusCode,
				fmt.Errorf("invalid attachment: %s", attachment), "", "", time.Duration(0), time.Now()))
			continue
		}
		switch mediaParts[0] {
		case "image":
			form := url.Values{
				"photo":   []string{mediaParts[1]},
				"caption": []string{caption},
			}
			externalID, log, err := h.sendMsgPart(msg, authToken, "sendPhoto", form)
			status.ExternalID = externalID
			hasError = err != nil
			status.AddLog(log)

		case "video":
			form := url.Values{
				"video":   []string{mediaParts[1]},
				"caption": []string{caption},
			}
			externalID, log, err := h.sendMsgPart(msg, authToken, "sendVideo", form)
			status.ExternalID = externalID
			hasError = err != nil
			status.AddLog(log)

		case "audio":
			form := url.Values{
				"audio":   []string{mediaParts[1]},
				"caption": []string{caption},
			}
			externalID, log, err := h.sendMsgPart(msg, authToken, "sendAudio", form)
			status.ExternalID = externalID
			hasError = err != nil
			status.AddLog(log)

		default:
			status.AddLog(courier.NewChannelLog(msg.Channel, msg.ID, "", courier.NilStatusCode,
				fmt.Errorf("unknown media type: %s", mediaParts[0]), "", "", time.Duration(0), time.Now()))
			hasError = true
		}
	}

	if hasError {
		status.Status = courier.MsgErrored
	} else {
		status.Status = courier.MsgWired
	}

	return status, nil
}

var telegramAPIURL = "https://api.telegram.org"

func resolveFileID(channel courier.Channel, fileID string) (string, error) {
	confAuth := channel.ConfigForKey(courier.ConfigAuthToken, "")
	authToken, isStr := confAuth.(string)
	if !isStr || authToken == "" {
		return "", fmt.Errorf("invalid auth token config")
	}

	fileURL := fmt.Sprintf("%s/bot%s/getFile", telegramAPIURL, authToken)

	form := url.Values{}
	form.Set("file_id", fileID)

	req, err := http.NewRequest("POST", fileURL, strings.NewReader(form.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	if err != nil {
		return "", err
	}

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
	return fmt.Sprintf("%s/file/bot%s/%s", telegramAPIURL, authToken, filePath), nil
}

type telegramFile struct {
	FileID   string `json:"file_id"    validate:"required"`
	FileSize int    `json:"file_size"`
}

type telegramLocation struct {
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
type telegramEnvelope struct {
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
			Thumb telegramFile `json:"thumb"`
		} `json:"sticker"`
		Photo    []telegramFile    `json:"photo"`
		Video    *telegramFile     `json:"video"`
		Voice    *telegramFile     `json:"voice"`
		Document *telegramFile     `json:"document"`
		Location *telegramLocation `json:"location"`
		Venue    *struct {
			Location *telegramLocation `json:"location"`
			Title    string            `json:"title"`
			Address  string            `json:"address"`
		}
		Contact *struct {
			PhoneNumber string `json:"phone_number"`
			FirstName   string `json:"first_name"`
			LastName    string `json:"last_name"`
		}
	} `json:"message"`
}
