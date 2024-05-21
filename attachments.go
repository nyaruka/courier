package courier

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/h2non/filetype"
	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
)

const (
	maxAttBodyReadBytes = 100 * 1024 * 1024
)

type Attachment struct {
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
	Size        int    `json:"size"`
}

type fetchAttachmentRequest struct {
	ChannelType ChannelType `json:"channel_type" validate:"required"`
	ChannelUUID ChannelUUID `json:"channel_uuid" validate:"required,uuid"`
	URL         string      `json:"url"          validate:"required"`
	MsgID       MsgID       `json:"msg_id"`
}

type fetchAttachmentResponse struct {
	Attachment *Attachment    `json:"attachment"`
	LogUUID    ChannelLogUUID `json:"log_uuid"`
}

func fetchAttachment(ctx context.Context, b Backend, r *http.Request) (*fetchAttachmentResponse, error) {
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

	ch, err := b.GetChannel(ctx, fa.ChannelType, fa.ChannelUUID)
	if err != nil {
		return nil, fmt.Errorf("error getting channel: %w", err)
	}

	clog := NewChannelLogForAttachmentFetch(ch, GetHandler(ch.ChannelType()).RedactValues(ch))

	attachment, err := FetchAndStoreAttachment(ctx, b, ch, fa.URL, clog)

	// try to write channel log even if we have an error
	clog.End()
	if err := b.WriteChannelLog(ctx, clog); err != nil {
		slog.Error("error writing log", "error", err)
	}

	if err != nil {
		return nil, err
	}

	return &fetchAttachmentResponse{Attachment: attachment, LogUUID: clog.UUID()}, nil
}

func FetchAndStoreAttachment(ctx context.Context, b Backend, channel Channel, attURL string, clog *ChannelLog) (*Attachment, error) {
	parsedURL, err := url.Parse(attURL)
	if err != nil {
		return nil, err
	}

	var attRequest *http.Request

	handler := GetHandler(channel.ChannelType())
	builder, isBuilder := handler.(AttachmentRequestBuilder)
	if isBuilder {
		attRequest, err = builder.BuildAttachmentRequest(ctx, b, channel, parsedURL.String(), clog)
	} else {
		attRequest, err = http.NewRequest(http.MethodGet, attURL, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("unable to create attachment request: %w", err)
	}

	trace, err := httpx.DoTrace(b.HttpClient(true), attRequest, nil, b.HttpAccess(), maxAttBodyReadBytes)
	if trace != nil {
		clog.HTTP(trace)

		// if we got a non-200 response, return the attachment with a pseudo content type which tells the caller
		// to continue without the attachment
		if trace.Response == nil || trace.Response.StatusCode/100 != 2 || err == httpx.ErrResponseSize || err == httpx.ErrAccessConfig {
			return &Attachment{ContentType: "unavailable", URL: attURL}, nil
		}
	}
	if err != nil {
		return nil, err
	}

	mimeType := ""
	extension := filepath.Ext(parsedURL.Path)
	if extension != "" {
		extension = extension[1:]
	}

	// prioritize to use the response content type header if provided
	contentTypeHeader := trace.Response.Header.Get("Content-Type")
	if contentTypeHeader != "" && contentTypeHeader != "application/octet-stream" {
		mimeType, _, _ = mime.ParseMediaType(contentTypeHeader)
		if extension == "" {
			extensions, err := mime.ExtensionsByType(mimeType)
			if extensions == nil || err != nil {
				extension = ""
			} else {
				extension = extensions[0][1:]
			}
		}
	} else {

		// first try getting our mime type from the first 300 bytes of our body
		fileType, _ := filetype.Match(trace.ResponseBody[:300])
		if fileType != filetype.Unknown {
			mimeType = fileType.MIME.Value
			extension = fileType.Extension
		} else {
			// if that didn't work, try from our extension
			fileType = filetype.GetType(extension)
			if fileType != filetype.Unknown {
				mimeType = fileType.MIME.Value
				extension = fileType.Extension
			}
		}
	}

	storageURL, err := b.SaveAttachment(ctx, channel, mimeType, trace.ResponseBody, extension)
	if err != nil {
		return nil, err
	}

	return &Attachment{ContentType: mimeType, URL: storageURL, Size: len(trace.ResponseBody)}, nil
}
