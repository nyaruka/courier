package handlers

import (
	"context"
	"net/http"

	"github.com/nyaruka/courier"
)

// BaseHandler is the base class for most handlers, it just stored the server, name and channel type for the handler
type BaseHandler struct {
	channelType courier.ChannelType
	name        string
	server      courier.Server
	backend     courier.Backend
}

// NewBaseHandler returns a newly constructed BaseHandler with the passed in parameters
func NewBaseHandler(channelType courier.ChannelType, name string) BaseHandler {
	return BaseHandler{channelType: channelType, name: name}
}

// SetServer can be used to change the server on a BaseHandler
func (h *BaseHandler) SetServer(server courier.Server) {
	h.server = server
	h.backend = server.Backend()
}

// Server returns the server instance on the BaseHandler
func (h *BaseHandler) Server() courier.Server {
	return h.server
}

// Backend returns the backend instance on the BaseHandler
func (h *BaseHandler) Backend() courier.Backend {
	return h.backend
}

// ChannelType returns the channel type that this handler deals with
func (h *BaseHandler) ChannelType() courier.ChannelType {
	return h.channelType
}

// ChannelName returns the name of the channel this handler deals with
func (h *BaseHandler) ChannelName() string {
	return h.name
}

// WriteStatusSuccessResponse writes a success response for the statuses
func (h *BaseHandler) WriteStatusSuccessResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, statuses []courier.MsgStatus) error {
	return courier.WriteStatusSuccess(ctx, w, r, statuses)
}

// WriteMsgSuccessResponse writes a success response for the messages
func (h *BaseHandler) WriteMsgSuccessResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, msgs []courier.Msg) error {
	return courier.WriteMsgSuccess(ctx, w, r, msgs)
}

// WriteRequestError writes the passed in error to our response writer
func (h *BaseHandler) WriteRequestError(ctx context.Context, w http.ResponseWriter, r *http.Request, err error) error {
	return courier.WriteError(ctx, w, r, err)
}

// WriteRequestIgnored writes an ignored payload to our response writer
func (h *BaseHandler) WriteRequestIgnored(ctx context.Context, w http.ResponseWriter, r *http.Request, details string) error {
	return courier.WriteIgnored(ctx, w, r, details)
}
