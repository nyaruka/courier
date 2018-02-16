package highconnection

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/urns"
)

/*
GET /handlers/hcnx/status/uuid?push_id=1164711372&status=6&to=%2B33611441111&ret_id=19128317&text=Msg

POST /handlers/hcnx/receive/uuid?FROM=+33644961111
ID=1164708294&FROM=%2B33644961111&TO=36105&MESSAGE=Msg&VALIDITY_DATE=2017-05-03T21%3A13%3A13&GET_STATUS=0&CLIENT=LEANCONTACTFAST&CLASS_TYPE=0&RECEPTION_DATE=2017-05-02T21%3A13%3A13&TO_OP_ID=20810&INITIAL_OP_ID=20810&STATUS=POSTING_30179_1410&EMAIL=&BINARY=0&PARAM=%7C%7C%7C%7CP223%2F03%2F03&USER_DATA=LEANCONTACTFAST&USER_DATA_2=jours+pas+r%E9gl%E9&BULK_ID=0&MO_ID=0&APPLICATION_ID=0&ACCOUNT_ID=39&GW_MESSAGE_ID=0&READ_STATUS=0&TARIFF=0&REQUEST_ID=33609002123&TAC=%28null%29&REASON=2017-05-02+23%3A13%3A13&FORMAT=&MVNO=&ORIG_ID=1164708215&ORIG_MESSAGE=Msg&RET_ID=123456&ORIG_DATE=2017-05-02T21%3A11%3A44
*/

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("HX"), "High Connection")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	err := s.AddHandlerRoute(h, http.MethodGet, "receive", h.ReceiveMessage)
	if err != nil {
		return err
	}
	return s.AddHandlerRoute(h, http.MethodPost, "receive", h.ReceiveMessage)
}

type moMsg struct {
	To          string `name:"TO" validate:"required"`
	From        string `name:"FROM" validate:"required"`
	Message     string `name:"MESSAGE" validate:"required"`
	ReceiveDate string `name:"RECEPTION_DATE"`
}

// ReceiveMessage is our HTTP handler function for incoming messages
func (h *handler) ReceiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request) ([]courier.Event, error) {
	hxRequest := &moMsg{}
	err := handlers.DecodeAndValidateForm(hxRequest, r)
	if err != nil {
		return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
	}

	date := time.Now()
	if hxRequest.ReceiveDate != "" {
		date, err = time.Parse("2006-01-02T15:04:05", hxRequest.ReceiveDate)
		if err != nil {
			return nil, courier.WriteAndLogRequestError(ctx, w, r, channel, err)
		}
	}

	// create our URN
	urn := urns.NewTelURNForCountry(hxRequest.From, channel.Country())

	// build our infobipMessage
	msg := h.Backend().NewIncomingMsg(channel, urn, hxRequest.Message).WithReceivedOn(date.UTC())

	// and write it
	err = h.Backend().WriteMsg(ctx, msg)
	if err != nil {
		return nil, err
	}
	return []courier.Event{msg}, courier.WriteMsgSuccess(ctx, w, r, []courier.Msg{msg})

}

// SendMsg sends the passed in message, returning any error
func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	return nil, fmt.Errorf("HX sending via courier not yet implemented")
}
