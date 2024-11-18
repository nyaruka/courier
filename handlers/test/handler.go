package test

import (
	"context"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	"github.com/nyaruka/gocommon/random"
)

func init() {
	courier.RegisterHandler(newHandler())
}

type handler struct {
	handlers.BaseHandler
}

func newHandler() courier.ChannelHandler {
	return &handler{handlers.NewBaseHandler(courier.ChannelType("TST"), "Test")}
}

func (h *handler) Initialize(s courier.Server) error {
	h.SetServer(s)
	return nil
}

func (h *handler) Send(ctx context.Context, msg courier.MsgOut, res *courier.SendResult, clog *courier.ChannelLog) error {
	sendDelayMs := msg.Channel().IntConfigForKey("send_delay_ms", 10)
	errorPercent := msg.Channel().IntConfigForKey("error_percent", 5)

	time.Sleep(time.Duration(sendDelayMs) * time.Millisecond)

	if random.IntN(100) < errorPercent || strings.Contains(msg.Text(), "\\error") {
		return courier.ErrConnectionFailed
	}

	return nil
}
