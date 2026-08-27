package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nyaruka/courier/v26/core/channels"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"

	"github.com/h2non/filetype"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/courier/v26/runtime"
	"github.com/nyaruka/courier/v26/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/svclogs"
)

type Attachment struct {
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
	Size        int    `json:"size"`
}

type fetchAttachmentRequest struct {
	ChannelType models.ChannelType `json:"channel_type" validate:"required"`
	ChannelUUID models.ChannelUUID `json:"channel_uuid" validate:"required,uuid"`
	URL         string             `json:"url"          validate:"required"`
	MsgUUID     models.MsgUUID     `json:"msg_uuid"`
}

type fetchAttachmentResponse struct {
	Attachment *Attachment  `json:"attachment"`
	LogUUID    svclogs.UUID `json:"log_uuid"`
}

func fetchAttachment(ctx context.Context, rt *runtime.Runtime, r *http.Request) (*fetchAttachmentResponse, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading request body: %w", err)
	}

	fa := &fetchAttachmentRequest{}
	if err := json.Unmarshal(body, fa); err != nil {
		return nil, fmt.Errorf("error unmarshalling request: %w", err)
	}
	if err := utils.Validate(fa); err != nil {
		return nil, err
	}

	ch, err := models.GetChannel(ctx, fa.ChannelType, fa.ChannelUUID)
	if err != nil {
		return nil, fmt.Errorf("error getting channel: %w", err)
	}

	handler := channels.GetHandler(ch.ChannelType())
	clog := models.NewChannelLogForAttachmentFetch(ch, handler.RedactValues(ch))

	attachment, err := FetchAndStoreAttachment(ctx, rt, ch, fa.URL, clog)

	// try to write channel log even if we have an error
	clog.End()
	if handler.StoreChannelLogs() {
		models.WriteChannelLog(rt, clog)
	}

	if err != nil {
		return nil, fmt.Errorf("error fetching attachment for msg %s: %w", fa.MsgUUID, err)
	}

	return &fetchAttachmentResponse{Attachment: attachment, LogUUID: clog.UUID}, nil
}

func FetchAndStoreAttachment(ctx context.Context, rt *runtime.Runtime, channel *models.Channel, attURL string, clog *models.ChannelLog) (*Attachment, error) {
	parsedURL, err := url.Parse(attURL)
	if err != nil {
		return nil, fmt.Errorf("unable to parse attachment url '%s': %w", attURL, err)
	}

	var attRequest *http.Request

	handler := channels.GetHandler(channel.ChannelType())
	builder, isBuilder := handler.(channels.AttachmentRequestBuilder)
	if isBuilder {
		attRequest, err = builder.BuildAttachmentRequest(ctx, channel, parsedURL.String(), clog)
	} else {
		attRequest, err = http.NewRequest(http.MethodGet, attURL, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to create attachment request: %w", err)
	}

	// attachment URLs are untrusted, so this goes through rt.HTTP.Attachments, whose transport bounds the body read
	// at runtime.MaxAttachmentBodyBytes and enforces access control (the SSRF blocklist) — a denied request comes
	// back as an error with a nil response. DoTraced drains the body, which is what turns an oversized attachment
	// into the ErrResponseSize checked below rather than a silently truncated file.
	trace, resp, err := utils.DoTraced(rt.HTTP.Attachments, attRequest)
	if trace != nil {
		clog.HTTP(trace)
	}

	// if we didn't get a usable 2xx response — connection failure, access denied, a non-2xx status, or
	// a body exceeding the size limit — return a pseudo content type which tells the caller to continue
	// without the attachment
	if resp == nil || resp.StatusCode/100 != 2 || errors.Is(err, httpx.ErrResponseSize) || errors.Is(err, httpx.ErrAccessConfig) {
		return &Attachment{ContentType: "unavailable", URL: attURL}, nil
	}
	if err != nil {
		return nil, err
	}

	mimeType, extension := getAttachmentType(trace)

	storageURL, err := models.SaveAttachment(ctx, rt, channel, mimeType, trace.ResponseBody, extension)
	if err != nil {
		return nil, err
	}

	return &Attachment{ContentType: mimeType, URL: storageURL, Size: len(trace.ResponseBody)}, nil
}

func getAttachmentType(t *httpx.Trace) (string, string) {
	var typ string

	// use extension from url path if it exists
	ext := filepath.Ext(t.Request.URL.Path)

	// prioritize to use the response content type header if provided
	contentTypeHeader := t.Response.Header.Get("Content-Type")
	if contentTypeHeader != "" {
		typ, _, _ = mime.ParseMediaType(contentTypeHeader)
	}

	// if we didn't get a meaningful content type from the header, try to guess it from the body
	if typ == "" || typ == "*/*" || typ == "application/octet-stream" {
		fileType, _ := filetype.Match(t.ResponseBody[:300])
		if fileType != filetype.Unknown {
			typ = fileType.MIME.Value
			if ext == "" {
				ext = fileType.Extension
			}
		}
	}

	// if we still don't have a type but the path has an extension, try to use that
	if typ == "" && ext != "" {
		fileType := filetype.GetType(ext)
		if fileType != filetype.Unknown {
			typ = fileType.MIME.Value
		}
	}

	// if we have a type but no extension, try to get one from the type
	if ext == "" {
		extensions, err := mime.ExtensionsByType(typ)
		if len(extensions) > 0 && err == nil {
			ext = extensions[0][1:]
			if slices.Contains([]string{"jpe", "jfif"}, ext) {
				ext = "jpg"
			}

		}
	}

	// got to default to something...
	if typ == "" {
		typ = "application/octet-stream"
	}

	return typ, strings.TrimPrefix(ext, ".")
}
