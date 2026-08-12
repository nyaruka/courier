package messangi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"encoding/base64"
	"encoding/xml"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/utils"
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
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("MG"), "Messangi")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	receiveHandler := handlers.NewTelReceiveHandler(h, "mobile", "mo")
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, receiveHandler)
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

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	publicKey := msg.Channel().StringConfigForKey(configPublicKey, "")
	privateKey := msg.Channel().StringConfigForKey(configPrivateKey, "")
	instanceId := msg.Channel().IntConfigForKey(configInstanceId, -1)
	carrierId := msg.Channel().IntConfigForKey(configCarrierId, -1)
	if publicKey == "" || privateKey == "" || instanceId == -1 || carrierId == -1 {
		return channels.ErrChannelConfig
	}

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
			return err
		}

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		// parse our response as XML
		response := &mtResponse{}
		err = xml.Unmarshal(respBody, response)
		if err != nil {
			return channels.ErrResponseUnparseable
		}

		// we always get 204 on success
		if response.Status != "OK" {
			return channels.ErrResponseStatus
		}
	}

	return nil
}
