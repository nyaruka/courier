package clickmobile

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/dates"
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
	s.AddHandlerRoute(h, http.MethodGet, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgStatus, h.receiveMessage)
	return nil
}

//  <request>
//    <shortCode>3014</shortCode>
//    <mobile>2659900993333</mobile>
//    <referenceID>1232434354</referenceID>
//    <text>This is a test message</text>
//  </request>

type moPayload struct {
	XMLName     xml.Name `xml:"request"`
	Shortcode   string   `xml:"shortCode"`
	Mobile      string   `xml:"mobile"`
	ReferenceID string   `xml:"referenceID"`
	Text        string   `xml:"text"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateXML(payload, r)
	if err != nil {
		return nil, err
	}

	if payload.Mobile == "" || payload.Shortcode == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing parameters, must have 'mobile' and 'shortcode'"))
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(payload.Mobile, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Text, payload.ReferenceID, clog)

	// and finally write our message
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
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

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
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

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)

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
		req, err := http.NewRequest(http.MethodPost, cmSendURL, requestBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		responseCode, _ := jsonparser.GetString(respBody, "code")
		if responseCode == "000" {
			status.SetStatus(courier.MsgStatusWired)
		} else {
			clog.Error(courier.ErrorResponseValueUnexpected("code", "000"))
		}
	}
	return status, nil

}
