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
	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/handlers"
	"github.com/nyaruka/gocommon/urns"
)

var (
	sendURL      = "https://api-public.mtarget.fr/api-sms.json"
	maxMsgLength = 765
)

func init() {
	channels.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() channels.Handler {
	return &handler{handlers.NewBaseHandler(models.ChannelType("MT"), "Mtarget")}
}

var statusMapping = map[string]models.MsgStatus{
	"0": models.MsgStatusWired,
	"1": models.MsgStatusWired,
	"2": models.MsgStatusSent,
	"3": models.MsgStatusDelivered,
	"4": models.MsgStatusFailed,
	"6": models.MsgStatusFailed,
}

// Initialize is called by the engine once everything is loaded
func (h *handler) Initialize(r *channels.Routes) error {
	r.Add(h, http.MethodPost, "receive", models.ChannelLogTypeMsgReceive, h.receiveMsg)

	statusHandler := handlers.NewExternalIDStatusHandler(h, statusMapping, "MsgId", "Status")
	r.Add(h, http.MethodPost, "status", models.ChannelLogTypeMsgStatus, statusHandler)
	return nil
}

// ReceiveMsg handles both MO messages and Stop commands
func (h *handler) receiveMsg(ctx context.Context, c *models.Channel, w http.ResponseWriter, r *http.Request, clog *models.ChannelLog) ([]channels.Event, error) {
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

		rc := h.Runtime().VK.Get()
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
	urn, err := urns.ParsePhone(from, c.Country(), true, false)
	if err != nil {
		return nil, handlers.WriteAndLogRequestError(ctx, h, c, w, r, err)
	}

	// if this a stop command, shortcut stopping that contact
	if keyword == "Stop" {
		stop := models.NewChannelEvent(c, models.EventTypeStopContact, urn, clog)
		err := models.WriteChannelEvent(ctx, h.Runtime(), stop, clog)
		if err != nil {
			return nil, err
		}
		return []channels.Event{stop}, channels.WriteChannelEventSuccess(w, stop)
	}

	// otherwise, create and write the message
	msg := models.NewIncomingMsg(c, urn, text, msgID, clog).WithReceivedOn(time.Now().UTC())
	return handlers.WriteMsgsAndResponse(ctx, h, []*models.MsgIn{msg}, w, r, clog)
}

func (h *handler) Send(ctx context.Context, msg *models.MsgOut, res *channels.SendResult, clog *models.ChannelLog) error {
	username := msg.Channel().StringConfigForKey(models.ConfigUsername, "")
	password := msg.Channel().StringConfigForKey(models.ConfigPassword, "")
	if username == "" || password == "" {
		return channels.ErrChannelConfig
	}

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
			return err
		}

		resp, respBody, err := h.RequestHTTP(req, clog)
		if err := handlers.ErrorFromResponse(resp, err); err != nil {
			return err
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
			res.AddExternalID(externalID)
		} else {
			reason, _ := jsonparser.GetString(respBody, "results", "[0]", "reason")
			return channels.ErrFailedWithReason(code, reason)
		}
	}

	return nil
}
