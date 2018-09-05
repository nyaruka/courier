package novo

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"net/url"
	"github.com/buger/jsonparser"
)

const (
	configMerchantId     = "merchant_id"
	configMerchantSecret = "merchant_secret"
)

var (
	maxMsgLength = 160
	sendURL      = "http://novosmstools.com/novo_te/%s/sendSMS"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("NV"), "Novo")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	receiveHandler := handlers.NewTelReceiveHandler(&h.BaseHandler, "from", "mo")
	s.AddHandlerRoute(h, http.MethodPost, "receive", receiveHandler)
	return nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	merchantId := msg.Channel().StringConfigForKey(configMerchantId, "")
	if merchantId == "" {
		return nil, fmt.Errorf("no merchant_id set for NV channel")
	}

	merchantSecret := msg.Channel().StringConfigForKey(configMerchantSecret, "")
	if merchantSecret == "" {
		return nil, fmt.Errorf("no merchant_secret set for NV channel")
	}

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		from := strings.TrimPrefix(msg.Channel().Address(), "+")
		to   := strings.TrimPrefix(msg.URN().Path(), "+")

		form := url.Values{
			"from": []string{from},
			"to":   []string{to},
			"msg":  []string{part},
		}
		form["signature"] = []string{utils.SignHMAC256(merchantSecret, fmt.Sprintf("%s;%s;%s;", from, to, part))}

		partSendURL, _ := url.Parse(fmt.Sprintf(sendURL, merchantId))
		partSendURL.RawQuery = form.Encode()

		req, _ := http.NewRequest(http.MethodGet, partSendURL.String(), nil)
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		responseMsgStatus, _ := jsonparser.GetString(rr.Body, "status")

		// we always get 204 on success
		if responseMsgStatus == "FINISHED" {
			status.SetStatus(courier.MsgWired)
		} else {
			status.SetStatus(courier.MsgFailed)
			log.WithError("Message Send Error", fmt.Errorf("received invalid response"))
			break
		}
	}

	return status, nil
}
