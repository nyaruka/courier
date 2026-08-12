package novo

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"net/url"
	"time"

	"github.com/buger/jsonparser"
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/courier/v26/utils"
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
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("NV"), "Novo")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
	// check authentication
	secret := c.StringConfigForKey(models.ConfigSecret, "")
	if secret != "" {
		authorization := r.Header.Get("Authorization")
		if authorization != secret {
			return nil, channels.WriteAndLogUnauthorized(w, r, c, fmt.Errorf("invalid Authorization header"))
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
	msg := models.NewIncomingMsg(c, urn, body, "", clog).WithReceivedOn(time.Now().UTC())
	return handlers.WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	merchantID := msg.Channel().StringConfigForKey(configMerchantId, "")
	merchantSecret := msg.Channel().StringConfigForKey(configMerchantSecret, "")
	if merchantID == "" || merchantSecret == "" {
		return channels.ErrChannelConfig
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
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
		}

		responseMsgStatus, err := jsonparser.GetString(respBody, "status")

		// we always get 204 on success
		if responseMsgStatus != "FINISHED" || err != nil {
			return channels.ErrResponseContent
		}
	}

	return nil
}
