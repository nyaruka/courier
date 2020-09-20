package ussd

import (
	"container/heap"
	"context"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"net/http"
	"strings"
	"time"
)

const (
	configStartMsg    = "start_msg"
	configTimeOut     = "time_out"
	configStripPrefix = "strip_prefix"
	SessionStatusWaiting     = "W"
	ussdSessionTimeOut = 600 // 5 minutes
)

const (

)
func init() {
	courier.RegisterHandler(newHandler())
}

type response struct {
	resp         string
	wantResponse bool
}

type PqItem struct {
	fkey string
	index int
	expiresOn time.Time
}

type sessions struct {
	c chan response
	i *PqItem
}

type handler struct {
	handlers.BaseHandler

	requests  map[string] sessions  // map[string]chan response // the request waiters, indexed by from+sessionID

	expirationQueue *PQueue // When last
}

func newHandler() courier.ChannelHandler {
	pq := make(PQueue,0)
	heap.Init(&pq) // init...
	return &handler{
		BaseHandler:      handlers.NewBaseHandler(courier.ChannelType("US"), "USSD"),
		requests:         make(map[string]sessions),
		expirationQueue: &pq,
	}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", h.receiveMessage)
	s.AddHandlerRoute(h, http.MethodGet, "status", h.receiveStatus)
	return nil
}

type moForm struct {
	ID          string `validate:"required" name:"sessionID"`
	Input       string `validate:"required" name:"ussdString"`
	Sender      string `validate:"required" name:"from"`
	ServiceCode string `validate:"required" name:"to"`
	MsgID       string `name:"messageID"`
}

// Make the key into the handler requests.
func makeKey(sender string, id string) string {
	return sender + "-" + id // Concatenate them
}

func (h *handler) receiveMessage(ctx context.Context, channel courier.Channel, writer http.ResponseWriter, request *http.Request) ([]courier.Event, error) {
	form := &moForm{}
	err := handlers.DecodeAndValidateForm(form, request)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, writer, request, err)
	}
	// create our URN
	urn, err := handlers.StrictTelForCountry(form.Sender, channel.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, channel, writer, request, err)
	}
	date := time.Now().UTC() // Current time...
	var timeout = channel.IntConfigForKey(configTimeOut, 30)
	var expiresOn = date.Add(time.Duration(ussdSessionTimeOut)*time.Second)
	// build our msg
	var input = form.Input

	var fkey = makeKey(urn.Path(), form.ID) // Use canonical phone number in key...
	r,ok := h.requests[fkey]
	if !ok { // New session
		r = sessions {
			 c: make(chan response, 100), // For waiting for the response from rapidPro
			 i: &PqItem{
			 	fkey: fkey,
			 	expiresOn: expiresOn,
			 },
		}
		h.requests[fkey] = r
		heap.Push(h.expirationQueue,r.i)
		var smsg = channel.StringConfigForKey(configStartMsg, "")
		if len(smsg) > 0 { // Use provided start message
			input = smsg
		}
	} else {
		var strip_prefix = channel.BoolConfigForKey(configStripPrefix, false)

		if strip_prefix {
			var idx = strings.LastIndex(input, "*")
			if idx > -1 {
				input = input[idx+1:] // Everything after the *
			}
		}
		// update expires time.
		h.expirationQueue.update(r.i,expiresOn)
	}

	msg := h.Backend().NewIncomingMsg(channel, urn, input).WithExternalID(form.ID).WithReceivedOn(date)
	events, err := writeMsgs(ctx, h, []courier.Msg{msg})

	// Now wait for the response from the  and send it back
	var v string
	var status int
	select {
	case res := <-r.c:
		v = res.resp
		if res.wantResponse {
			status = http.StatusAccepted
		} else {
			heap.Remove(h.expirationQueue,r.i.index)
		}
	case <-time.After(time.Second * time.Duration(timeout)):
		status = http.StatusGatewayTimeout
		v = "time out waiting for response"
		heap.Remove(h.expirationQueue,r.i.index) // Clear it. Right?
	}

	// Send HTTP response.
	_, err = writeTextResponse(writer,status,v)

	// Delete expired ones...
	for h.expirationQueue.Len() > 0 {
		item := heap.Pop(h.expirationQueue).(*PqItem)
		if item.expiresOn.After(date) {
			// put it back, we are done
			heap.Push(h.expirationQueue,item)
			break
		}
		// clear the session
		delete( h.requests,item.fkey)
	}
	return events, err
}

func (h *handler) receiveStatus(ctx context.Context, channel courier.Channel, writer http.ResponseWriter, request *http.Request) ([]courier.Event, error) {

	return nil, handlers.WriteAndLogRequestIgnored(ctx, h, channel, writer, request, "shouldn't happen.")
}

func (h *handler) SendMsg(ctx context.Context, msg courier.Msg) (courier.MsgStatus, error) {
	var sender = msg.URN().Path()
	var sessionID = msg.ResponseToExternalID()

	var resp = response{
		resp:         handlers.GetTextAndAttachments(msg),
		wantResponse: msg.SessionStatus() == SessionStatusWaiting,
	}

	fkey := makeKey(sender, sessionID)

	r,ok := h.requests[fkey]

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgFailed)

	// Push out.
	if ok {
		r.c <- resp
		status.SetStatus(courier.MsgSent)
	}
	return status, nil
}

// Write message to backend, do not send http response.
func writeMsgs(ctx context.Context, h handlers.ResponseWriter, msgs []courier.Msg) ([]courier.Event, error) {
	events := make([]courier.Event, len(msgs), len(msgs))
	for i, m := range msgs {
		err := h.Backend().WriteMsg(ctx, m)
		if err != nil {
			return nil, err
		}
		events[i] = m
	}

	return events, nil
}

func writeTextResponse(w http.ResponseWriter, statusCode int, response string) (int,error) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(statusCode)
	return w.Write([]byte(response))
}

// Priority queue used to ensure timed-out session do not stick around
type PQueue []*PqItem
func (pq PQueue) Len() int {return len(pq)}
func (pq PQueue) Less (i, j int) bool {
	return pq[i].expiresOn.Before(pq[j].expiresOn)
}
func (pq PQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*PqItem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

func (pq *PQueue) update(item *PqItem, expiresOn time.Time) {
	item.expiresOn = expiresOn
	heap.Fix(pq, item.index)
}
