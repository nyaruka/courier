package nexmo

import (
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

/*
GET /handlers/nexmo/status/uuid/?msisdn=4527631111&to=Tak&network-code=23820&messageId=0C0000002EEBDA56&price=0.01820000&status=delivered&scts=1705021324&err-code=0&message-timestamp=2017-05-02+11%3A24%3A03
GET /handlers/nexmo/receive/uuid/?msisdn=15862151111&to=12812581111&messageId=0B0000004B65F62F&text=Msg&type=text&keyword=Keyword&message-timestamp=2017-05-01+21%3A52%3A49
*/

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Infobip handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("NX"), "Nexmo")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return s.AddUpdateStatusRoute(h, "GET", "status", h.StatusMessage)
}

type nexmoDeliveryReport struct {
	To        string `name:"to" validate:"required"`
	MessageID string
	Status    string
}

var statusMappings = map[string]courier.MsgStatusValue{
	"failed":    courier.MsgFailed,
	"expired":   courier.MsgFailed,
	"rejected":  courier.MsgFailed,
	"buffered":  courier.MsgSent,
	"accepted":  courier.MsgSent,
	"unknown":   courier.MsgWired,
	"delivered": courier.MsgDelivered,
}

// StatusMessage is our HTTP handler function for status updates
func (h *handler) StatusMessage(channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.MsgStatus, error) {
	return nil, nil
}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(msg courier.Msg) (courier.MsgStatus, error) {
	return nil, nil
}
