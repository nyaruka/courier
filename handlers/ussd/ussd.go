package ussd

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/courier/utils"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	configStartMsg    = "ussd_start_msg"
	configTimeOut     = "ussd_request_time_out"
	configStripPrefix = "ussd_strip_prefix"
	configPushUrl = "ussd_push_url"
	SessionStatusWaiting     = "W"
	ussdSessionTimeOut = 600 // A USSD session, we assume, shouldn't last more than 5 minutes
)

const (

)
func init() {
	courier.RegisterHandler(newHandler())
}

type responseData struct {
	response        string
	expectsResponse bool
}


// Every ussd session has a channel on which responses from flow engine are posted, and
// an entry in our priority queue, used for tracking timed-out sessions
type sessionData struct {
	r string
	s time.Time
	c chan responseData
	i *pQitem
}

type handler struct {
	handlers.BaseHandler
	requests  map[string]sessionData // the request waiters, indexed by from+sessionID
	expirationQueue *pQueue          // When last
}

func newHandler() courier.ChannelHandler {
	pq := make(pQueue,0)
	heap.Init(&pq)
	return &handler{
		BaseHandler:      handlers.NewBaseHandler(courier.ChannelType("US"), "USSD"),
		requests:         make(map[string]sessionData),
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
	ID       string `validate:"required" name:"sessionID"`
	Input    string `validate:"required" name:"ussdString"`
	Sender   string `validate:"required" name:"from"`
	USSDcode string `validate:"required" name:"to"`
	MsgID    string `name:"messageID"`
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
		r = sessionData{
			 c: make(chan responseData, 100), // For waiting for the response from rapidPro
			 i: &pQitem{
			 	fkey: fkey,
			 	expiresOn: expiresOn,
			 },
			 s: time.Now(),
			 r: input,
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
		v = res.response
		if res.expectsResponse {
			status = http.StatusAccepted
		} else {
			status = http.StatusOK
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
		item := heap.Pop(h.expirationQueue).(*pQitem)
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
	var externalSessionID = msg.ResponseToExternalID()
	var expectsResponse = msg.SessionStatus() == SessionStatusWaiting
	var msgText = handlers.GetTextAndAttachments(msg)

	status := h.Backend().NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgFailed)
	if len(externalSessionID) > 0 { // A response...
		var resp = responseData{
			response: msgText ,
			expectsResponse: expectsResponse,
		}

		r, ok := h.requests[makeKey(sender, externalSessionID)]
		var err error
		var request string
		var duration time.Duration
		var httpCode int
		// Push out.
		if ok {
			r.c <- resp
			status.SetStatus(courier.MsgWired)
			request = r.r
			duration = time.Now().Sub(r.s)
			if expectsResponse {
				httpCode = http.StatusAccepted
			} else {
				httpCode = http.StatusOK
			}
		} else {
			httpCode = http.StatusGatewayTimeout
			status.SetStatus(courier.MsgFailed)
			err = errors.New("timeout waiting for response")
		}
		// Log it.
		status.AddLog(courier.NewChannelLog("Message Sent",msg.Channel(),msg.ID(),"GET","", httpCode,request,msgText,duration,err))
	} else {
		// Must be a push message...
		pushUrl := msg.Channel().StringConfigForKey(configPushUrl,"")
		if len(pushUrl) > 0 {
			var r string
			if expectsResponse {
				r = "yes"
			} else  {
				r = "no"
			}

			form := url.Values{
				"to" : []string{sender},
				"message": []string{msgText},
				"respond": []string{r},
			}
			encodedForm := form.Encode()
			if strings.Contains(pushUrl, "?") {
				pushUrl = fmt.Sprintf("%s&%s", pushUrl, encodedForm)
			} else {
				pushUrl = fmt.Sprintf("%s?%s", pushUrl, encodedForm)
			}
			var rr *utils.RequestResponse
			req, err := http.NewRequest(http.MethodGet, pushUrl, nil)
			rr, err = utils.MakeHTTPRequest(req)
			status.AddLog(courier.NewChannelLogFromRR("Message Sent", msg.Channel(), msg.ID(), rr).WithError("Message Send Error", err))
			if err == nil {
				status.SetStatus(courier.MsgWired)
			} else {
				status.SetStatus(courier.MsgFailed)
			}
		} else {
			status.SetStatus(courier.MsgFailed)
			status.AddLog(courier.NewChannelLogFromError("Message cannot be sent, channel does not support USSD PUSH",msg.Channel(),msg.ID(),0,errors.New("Please set the push URL")))
		}
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

// Priority queue used to ensure timed-out session do not stick around too long
type pQitem struct {
	fkey string
	index int
	expiresOn time.Time
}

type pQueue []*pQitem
func (pq pQueue) Len() int {return len(pq)}
func (pq pQueue) Less (i, j int) bool {
	return pq[i].expiresOn.Before(pq[j].expiresOn)
}
func (pq pQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *pQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*pQitem)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *pQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

func (pq *pQueue) update(item *pQitem, expiresOn time.Time) {
	item.expiresOn = expiresOn
	heap.Fix(pq, item.index)
}
