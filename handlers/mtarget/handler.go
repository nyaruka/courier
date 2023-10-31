package mtarget

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
)

var (
	sendURL      = "https://api-public.mtarget.fr/api-sms.json"
	maxMsgLength = 765
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("MT"), "Mtarget")}
}

var statusMapping = map[string]courier.MsgStatus{
	"0": courier.MsgStatusWired,
	"1": courier.MsgStatusWired,
	"2": courier.MsgStatusSent,
	"3": courier.MsgStatusDelivered,
	"4": courier.MsgStatusFailed,
	"6": courier.MsgStatusFailed,
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	s.AddHandlerRoute(h, http.MethodPost, "receive", courier.ChannelLogTypeMsgReceive, h.receiveMsg)

	statusHandler := handlers.NewExternalIDStatusHandler(h, statusMapping, "MsgId", "Status")
	s.AddHandlerRoute(h, http.MethodPost, "status", courier.ChannelLogTypeMsgStatus, statusHandler)
	return nil
}

// ReceiveMsg handles both MO messages and Stop commands
func (h *handler) receiveMsg(ctx context.Context, c courier.Channel, w http.ResponseWriter, r *http.Request, clog *courier.ChannelLog) ([]courier.Event, error) {
	err := r.ParseForm()
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	text := r.Form.Get("Content")
	from := r.Form.Get("Msisdn")
	keyword := r.Form.Get("Keyword")
	msgID := r.Form.Get("MsgId")

	if from == "" {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("missing required field 'Msisdn'"))
	}

	// if we have a long message id, then this is part of a multipart message, we don't write the message until
	// we have received all parts, which we buffer in Redis
	longID := r.Form.Get("msglong.id")
	if longID != "" {
		longCount, _ := strconv.Atoi(r.Form.Get("msglong.msgcount"))
		longRef, _ := strconv.Atoi(r.Form.Get("msglong.msgref"))

		if longCount == 0 || longRef == 0 {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("invalid or missing 'msglong.msgcount' or 'msglong.msgref' parameters"))
		}

		if longRef < 1 || longRef > longCount {
			return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, fmt.Errorf("'msglong.msgref' needs to be between 1 and 'msglong.msgcount' inclusive"))
		}

		rc := h.Backend().RedisPool().Get()
		defer rc.Close()

		// first things first, populate the new part we just received
		mapKey := fmt.Sprintf("%s:%s", c.UUID(), longID)
		rc.Send("MULTI")
		rc.Send("HSET", mapKey, longRef, text)
		rc.Send("EXPIRE", mapKey, 300)
		_, err := rc.Do("EXEC")
		if err != nil {
			return nil, err
		}

		// see if we have all the parts we need
		count, err := redis.Int(rc.Do("HLEN", mapKey))
		if err != nil {
			return nil, err
		}

		// we don't have all the parts yet, say we received the message
		if count != longCount {
			return nil, handlers.WriteAndLogRequestIgnored(ctx, h, c, w, r, "Message part received")
		}

		// we have all our parts, grab them and put them together
		// build up the list of keys we are looking up
		keys := make([]any, longCount+1)
		keys[0] = mapKey
		for i := 1; i < longCount+1; i++ {
			keys[i] = fmt.Sprintf("%d", i)
		}

		segments, err := redis.Strings(rc.Do("HMGET", keys...))
		if err != nil {
			return nil, err
		}

		// join our segments in our text
		text = strings.Join(segments, "")

		// finally delete our key, we are done with this message
		rc.Do("DEL", mapKey)
	}

	// create our URN
	urn, err := handlers.StrictTelForCountry(from, c.Country())
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// if this a stop command, shortcut stopping that contact
	if keyword == "Stop" {
		stop := h.Backend().NewChannelEvent(c, courier.EventTypeStopContact, urn, clog)
		err := h.Backend().WriteChannelEvent(ctx, stop, clog)
		if err != nil {
			return nil, err
		}
		return []courier.Event{stop}, courier.WriteChannelEventSuccess(w, stop)
	}

	// otherwise, create and write the message
	msg := h.Backend().NewIncomingMsg(c, urn, text, msgID, clog).WithReceivedOn(time.Now().UTC())
	return handlers.WriteMsgsAndResponse(ctx, h, []courier.MsgIn{msg}, w, r, clog)
}

// Send sends the given message, logging any HTTP calls or errors
func (h *handler) Send(ctx context.Context, msg courier.MsgOut, clog *courier.ChannelLog) (courier.StatusUpdate, error) {
	username := msg.Channel().StringConfigForKey(courier.ConfigUsername, "")
	if username == "" {
		return nil, fmt.Errorf("no username set for MT channel")
	}

	password := msg.Channel().StringConfigForKey(courier.ConfigPassword, "")
	if password == "" {
		return nil, fmt.Errorf("no password set for MT channel")
	}

	// send our message
	status := h.Backend().NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog)
	for _, part := range handlers.SplitMsgByChannel(msg.Channel(), handlers.GetTextAndAttachments(msg), maxMsgLength) {
		// build our request
		params := url.Values{
			"username":     []string{username},
			"password":     []string{password},
			"msisdn":       []string{msg.URN().Path()},
			"msg":          []string{part},
			"serviceid":    []string{msg.Channel().Address()},
			"allowunicode": []string{"true"},
		}

		msgURL, _ := url.Parse(sendURL)
		msgURL.RawQuery = params.Encode()
		req, err := http.NewRequest(http.MethodPost, msgURL.String(), nil)
		if err != nil {
			return nil, err
		}

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err != nil || resp.StatusCode/100 != 2 {
			return status, nil
		}

		// parse our response for our status code and ticket (external id)
		// {
		//	"results": [{
		//		"msisdn": "+447xxxxxxxx",
		//		"smscount": "1",
		//		"code": "0",
		//		"reason": "ACCEPTED",
		//		"ticket": "760eeaa0-5034-11e7-bb92-00000a0a643a"
		//  }]
		// }
		code, _ := jsonparser.GetString(respBody, "results", "[0]", "code")
		externalID, _ := jsonparser.GetString(respBody, "results", "[0]", "ticket")
		if code == "0" && externalID != "" {
			// all went well, set ourselves to wired
			status.SetStatus(courier.MsgStatusWired)
			status.SetExternalID(externalID)
		} else {
			status.SetStatus(courier.MsgStatusFailed)
			clog.RawError(fmt.Errorf("Error status code, failing permanently"))
			break
		}
	}

	return status, nil
}
