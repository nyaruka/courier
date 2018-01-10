package start

/*
POST /handlers/start/receive/uuid/
<message><service type='sms' timestamp='1493792274' auth='1auth42d6e1aa608b6038' request_id='40599627'/><from>380975831111</from><to>4224</to><body>Msg</body></message>
*/

import (
	"strconv"
	"time"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

func init() {
	courier.RegisterHandler(NewHandler())
}

type handler struct {
	handlers.BaseHandler
}

// NewHandler returns a new Zenvia handler
func NewHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("ST"), "Start Mobile")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, "POST", "receive", h.ReceiveMessage)
	return nil
}

type moMessage struct {
	XMLName xml.Name `xml:"message"`
	Service struct {
		Timestamp string `xml:"timestamp,attr"`
		RequestID string `xml:"request_id,attr"`
	} `xml:"service"`
	From  string `xml:"from"`
	To string `xml:"to"`
	Body struct {
		Text        string `xml:",chardata"`
	} `xml:"body"`
}


// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	mo := &moMessage{}
	err := handlers.DecodeAndValidateXML(mo, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	if mo.Service.RequestID == "" || mo.From == "" {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("missing parameters, must have 'request_id', 'to' and 'body'"))
	}

	// create our URN
	urn := urns.NewTelURNForCountry(mo.From, channel.Country())
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	// create our date from the timestamp
	ts, err := strconv.ParseInt(mo.Service.Timestamp, 10, 64)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, fmt.Errorf("invalid timestamp: %s", mo.Service.Timestamp))
	}
	date := time.Unix(ts, 0).UTC()

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, mo.Body.Text).WithReceivedOn(date)

	// and write it
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("ST sending via Courier not yet implemented")
}
