package novo

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"net/url"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/urns"
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
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	// check authentication
	secret := c.StringConfigForKey(courier.ConfigSecret, "")
	if secret != "" {
		authorization := r.Header.Get("Authorization")
		if authorization != secret {
			return nil, courier.WriteAndLogUnauthorized(w, r, c, fmt.Errorf("invalid Authorization header"))
		}
	}

	err := r.ParseForm()
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	body := r.Form.Get("text")
	from := r.Form.Get("from")
	if from == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("missing required field 'from'"))
	}

	// create our URN
	urn, err := urns.ParsePhone(from, c.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// create and write the message
	msg := h.Backend().NewIncomingMsg(ctx, c, urn, body, "", clog).WithReceivedOn(time.Now().UTC())
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	merchantID := msg.Channel().StringConfigForKey(configMerchantId, "")
	merchantSecret := msg.Channel().StringConfigForKey(configMerchantSecret, "")
	if merchantID == "" || merchantSecret == "" {
		return courier.ErrChannelConfig
	}
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		from := strings.TrimPrefix(msg.Channel().Address(), "+")
		to := strings.TrimPrefix(msg.URN().Path(), "+")

		form := url.Values{
			"from": []string{from},
			"to":   []string{to},
			"msg":  []string{part},
		}
		form["signature"] = []string{utils.SignHMAC256(merchantSecret, fmt.Sprintf("%s;%s;%s;", from, to, part))}

		partSendURL, _ := url.Parse(fmt.Sprintf(sendURL, merchantID))
		partSendURL.RawQuery = form.Encode()

		req, err := http.NewRequest(http.MethodGet, partSendURL.String(), nil)
		if err != nil {
			return err
		}

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 == 5 {
			return courier.ErrConnectionFailed
		} else if resp.StatusCode/100 != 2 {
			return courier.ErrResponseStatus
		}

		responseMsgStatus, err := jsonparser.GetString(respBody, "status")

		// we always get 204 on success
		if responseMsgStatus != "FINISHED" || err != nil {
			return courier.ErrResponseContent
		}
	}

	return nil
}
