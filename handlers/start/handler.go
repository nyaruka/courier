package start

/*
POST /handlers/start/receive/uuid/
<message><service type='sms' timestamp='1493792274' auth='1auth42d6e1aa608b6038' request_id='40599627'/><from>380975831111</from><to>4224</to><body>Msg</body></message>
*/

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/httpx"
)

var (
	maxMsgLength = 1600
	sendURL      = "https://bulk.startmobile.ua/clients.php"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("ST"), "Start Mobile")}
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMessage)
	return nil
}

type moPayload struct {
	XMLName xml.Name `xml:"message"`
	Service struct {
		Timestamp string `xml:"timestamp,attr"`
		RequestID string `xml:"request_id,attr"`
	} `xml:"service"`
	From string `xml:"from"`
	To   string `xml:"to"`
	Body struct {
		Text string `xml:",chardata"`
	} `xml:"body"`
}

// receiveMessage is our HTTP handler function for incoming messages
func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	payload := &moPayload{}
	err := handlers.DecodeAndValidateXML(payload, r)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	if payload.Service.RequestID == "" || payload.From == "" || payload.To == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("missing parameters, must have 'request_id', 'to' and 'body'"))
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(payload.From, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, err)
	}

	// create our date from the timestamp
	ts, err := strconv.ParseInt(payload.Service.Timestamp, 10, 64)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, w, r, fmt.Errorf("invalid timestamp: %s", payload.Service.Timestamp))
	}
	date := time.Unix(ts, 0).UTC()

	// build our msg
	msg := h.Backend().NewIncomingMsg(channel, urn, payload.Body.Text, payload.Service.RequestID, clog).WithReceivedOn(date)

	// and write it
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

// Start Mobile expects a XML response from a message receive request
func (h *handler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, msgs []courier.MsgIn) error {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(200)
	_, err := fmt.Fprint(w, `<answer type="async"><state>Accepted</state></answer>`)
	return err
}

type mtBody struct {
	ContentType string `xml:"content-type,attr"`
	Encoding    string `xml:"encoding,attr"`
	Text        string `xml:",chardata"`
}

type mtService struct {
	ID       string `xml:"id,attr"`
	Source   string `xml:"source,attr"`
	Validity string `xml:"validity,attr"`
}

type mtPayload struct {
	XMLName xml.Name  `xml:"message"`
	Service mtService `xml:"service"`
	To      string    `xml:"to"`
	Body    mtBody    `xml:"body"`
}

type mtResponse struct {
	XMLName xml.Name `xml:"status"`
	ID      string   `xml:"id"`
	State   string   `xml:"state"`
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for ST channel: %s", msg.Channel().UUID())
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for ST channel: %s", msg.Channel().UUID())
	}

	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	parts := handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength)
	for i, part := range parts {

		payload := mtPayload{
			Service: mtService{
				ID:       "single",
				Source:   msg.Channel().Address(),
				Validity: "+12 hours",
			},
			To: msg.URN().Path(),
			Body: mtBody{
				ContentType: "plain/text",
				Encoding:    "plain",
				Text:        part,
			},
		}

		requestBody := &bytes.Buffer{}
		err := xml.NewEncoder(requestBody).Encode(payload)
		if err != nil {
			return nil, err
		}

		// build our request
		req, err := http.NewRequest(http.MethodPost, sendURL, requestBody)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/xml; charset=utf8")
		req.SetBasicAuth(username, password)

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		response := &mtResponse{}
		err = xml.Unmarshal(respBody, response)
		if err == nil {
			status.SetStatus(courier.MsgStatusWired)
			if i == 0 {
				status.SetExternalID(response.ID)
			}
		}
	}

	return status, nil
}

func (h *handler) RedactValues(ch courier.Channel) []string {
	return []string{
		httpx.BasicAuth(ch.StringConfigForKey(courier.ConfigUsername, ""), ch.StringConfigForKey(courier.ConfigPassword, "")),
	}
}
