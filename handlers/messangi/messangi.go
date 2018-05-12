package messangi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"encoding/base64"
	"encoding/hex"
	"github.com/nyaruka/courier/utils"
	"encoding/xml"
)

const (
	configPublicKey  = "messangi_public_key"
	configPrivateKey = "messangi_private_key"
	configInstanceId = "messangi_instance_id"
	configCarrierId  = "messangi_carrier_id"
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
	receiveHandler := handlers.NewTelReceiveHandler(&h.BaseHandler, "mobile", "mo")
	s.AddHandlerRoute(h, http.MethodPost, "receive", receiveHandler)
	return nil
}

//<response>
//	<input>sendMT</input>
//	<status>OK</status>
//	<description>Completed</description>
//</response>
type mtResponse struct {
	Input		string `xml:"input"`
	Status		string `xml:"status"`
	Description	string `xml:"description"`
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
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

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored)
	parts := handlers.SplitMsg(handlers.GetTextAndAttachments(msg), maxMsgLength)
	for _, part := range parts {
		shortcode  := strings.TrimPrefix(msg.Channel().Address(), "+")
		to         := strings.TrimPrefix(msg.URN().Path(), "+")
		textBase64 := base64.StdEncoding.EncodeToString([]byte(part))
		params     := fmt.Sprintf("%d/%s/%d/%s/%s", instanceId, shortcode, carrierId, to, textBase64)
		signature  := signHmacSha256(privateKey, params)
		fullURL    := fmt.Sprintf("%s/%s/%s/%s", sendURL, params, publicKey, signature)

		fmt.Println(fullURL)
		req, _ := http.NewRequest(http.MethodGet, fullURL, nil)
		rr, err := utils.MakeHTTPRequest(req)

		// record our status and log
		log := courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err)
		status.AddLog(log)
		if err != nil {
			return status, nil
		}

		// parse our response as XML
		response := &mtResponse{}
		err = xml.Unmarshal(rr.Body, response)
		if err != nil {
			log.WithError("Message Send Error", err)
			break
		}

		// we always get 204 on success
		if response.Status == "OK" {
			status.SetStatus(courier.MsgWired)
		} else {
			status.SetStatus(courier.MsgFailed)
			log.WithError("Message Send Error", fmt.Errorf("Received invalid response description: %s", response.Description))
			break
		}
	}

	return status, nil
}

func signHmacSha256(privateKey string, params string) string {
	hash := hmac.New(sha256.New, []byte(privateKey))
	hash.Write([]byte(params))

	signedParams := hex.EncodeToString(hash.Sum(nil))
	return signedParams
}
