package clickmobile

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/courier/utils/dates"
)

var (
	sendURL      = "http://206.225.81.36/ucm_api/index.php"
	maxMsgLength = 160

	configAppID = "app_id"
	configOrgID = "org_id"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("CM"), "Click Mobile")}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodGet, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	return nil
}

type moForm struct {
	To   string `name:"to"`
	Text string `validate:"required" name:"text"`
	From string `validate:"required" name:"from"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	// get our params
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, r)
	if err != nil {
		return nil, err
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(form.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, form.Text)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.Msg{msg}, w, r)
}

type mtPayload struct {
	AppID       string `json:"app_id"`
	OrgID       string `json:"org_id"`
	UserID      string `json:"user_id"`
	Timestamp   string `json:"timestamp"`
	AuthKey     string `json:"auth_key"`
	Operation   string `json:"operation"`
	Reference   string `json:"reference"`
	MessageType string `json:"message_type"`
	From        string `json:"src_address"`
	To          string `json:"dst_address"`
	Message     string `json:"message"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for CM channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for CM channel")
	}

	appID := msg.Channel().StringConfigForKey(configAppID, "")
	if appID == "" {
		return nil, fmt.Errorf("no app_id set for CM channel")
	}

	orgID := msg.Channel().StringConfigForKey(configOrgID, "")
	if orgID == "" {
		return nil, fmt.Errorf("no org_id key set for CM channel")
	}

	cmSendURL := msg.Channel().StringConfigForKey(courier.ConfigSendURL, sendURL)

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)

	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {

		timestamp := dates.Now().UTC().Format("20060102150405")

		hasher := md5.New()
		hasher.Write([]byte(appID + timestamp + password))
		hash := hex.EncodeToString(hasher.Sum(nil))

		payload := mtPayload{
			AppID:       appID,
			OrgID:       orgID,
			UserID:      username,
			Timestamp:   timestamp,
			AuthKey:     hash,
			Operation:   "send",
			Reference:   msg.ID().String(),
			MessageType: "1",
			From:        msg.Channel().Address(),
			To:          msg.URN().Path(),
			Message:     part,
		}

		requestBody := &bytes.Buffer{}
		json.NewEncoder(requestBody).Encode(payload)

		// build our request
		req, _ := http.NewRequest(http.MethodPost, cmSendURL, requestBody)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		rr, err := utils.MakeHTTPRequest(req)

		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		responseCode, err := jsonparser.GetString(rr.Body, "code")
		if responseCode == "000" {
			status.SetStatus(courier.MsgWired)
		} else {
			log.WithError("Message Send Error", fmt.Errorf("Received invalid response content: %s", string(rr.Body)))
		}
	}
	return status, nil

}
