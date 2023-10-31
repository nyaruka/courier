package messangi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"encoding/base64"
	"encoding/xml"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
)

const (
	configPublicKey  = "public_key"
	configPrivateKey = "private_key"
	configInstanceId = "instance_id"
	configCarrierId  = "carrier_id"
)

var (
	maxMsgLength = 160
	sendURL      = "https://flow.messangi.me/mmc/rest/api/sendMT"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("MG"), "Messangi")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	receiveHandler := handlers.NewTelReceiveHandler(h, "mobile", "mo")
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, receiveHandler)
	return nil
}

// <response>
//
//	<input>sendMT</input>
//	<status>OK</status>
//	<description>Completed</description>
//
// </response>
type mtResponse struct {
	Input       string `xml:"input"`
	Status      string `xml:"status"`
	Description string `xml:"description"`
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	publicKey := msg.Channel().StringConfigForKey(configPublicKey, "")
	if publicKey == "" {
		return nil, fmt.Errorf("no public_key set for MG channel")
	}

	privateKey := msg.Channel().StringConfigForKey(configPrivateKey, "")
	if privateKey == "" {
		return nil, fmt.Errorf("no private_key set for MG channel")
	}

	instanceId := msg.Channel().IntConfigForKey(configInstanceId, -1)
	if instanceId == -1 {
		return nil, fmt.Errorf("no instance_id set for MG channel")
	}

	carrierId := msg.Channel().IntConfigForKey(configCarrierId, -1)
	if carrierId == -1 {
		return nil, fmt.Errorf("no carrier_id set for MG channel")
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		shortcode := strings.TrimPrefix(msg.Channel().Address(), "+")
		to := strings.TrimPrefix(msg.URN().Path(), "+")
		textBase64 := base64.RawURLEncoding.EncodeToString([]byte(part))
		params := fmt.Sprintf("%d/%s/%d/%s/%s", instanceId, shortcode, carrierId, to, textBase64)
		signature := utils.SignHMAC256(privateKey, params)
		fullURL := fmt.Sprintf("%s/%s/%s/%s", sendURL, params, publicKey, signature)

		req, err := http.NewRequest(http.MethodGet, fullURL, nil)
		if err != nil {
			return nil, err
		}

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// parse our response as XML
		response := &mtResponse{}
		err = xml.Unmarshal(respBody, response)
		if err != nil {
			clog.Error(courier.ErrorResponseUnparseable("XML"))
			break
		}

		// we always get 204 on success
		if response.Status == "OK" {
			status.SetStatus(courier.MsgStatusWired)
		} else {
			status.SetStatus(courier.MsgStatusFailed)
			clog.Error(courier.ErrorResponseValueUnexpected("status", "OK"))
			break
		}
	}

	return status, nil
}
