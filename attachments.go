package courier

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/h2non/filetype.v1"
)

const (
	maxAttBodyReadBytes = 100 * 1024 * 1024
)

type fetchAttachmentRequest struct {
	ChannelType ChannelType `json:"channel_type" validate:"required"`
	ChannelUUID ChannelUUID `json:"channel_uuid" validate:"required,uuid"`
	URL         string      `json:"url"          validate:"required"`
}

func fetchAttachment(ctx context.Context, b Backend, r *http.Request) (string, int, *ChannelLog, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", 0, nil, errors.Wrap(err, "error reading request body")
	}

	fa := &fetchAttachmentRequest{}
	if err := json.Unmarshal(body, fa); err != nil {
		return "", 0, nil, errors.Wrap(err, "error unmarshalling request")
	}
	if err := utils.Validate(fa); err != nil {
		return "", 0, nil, err
	}

	ch, err := b.GetChannel(ctx, fa.ChannelType, fa.ChannelUUID)
	if err != nil {
		return "", 0, nil, errors.Wrap(err, "error getting channel")
	}

	clog := NewChannelLogForAttachmentFetch(ch, GetHandler(ch.ChannelType()).RedactValues(ch))

	newURL, size, err := FetchAndStoreAttachment(ctx, b, ch, fa.URL, clog)

	if err := b.WriteChannelLog(ctx, clog); err != nil {
		logrus.WithError(err).Error()
	}

	return newURL, size, clog, err
}

func FetchAndStoreAttachment(ctx context.Context, b Backend, channel Channel, attURL string, clog *ChannelLog) (string, int, error) {
	parsedURL, err := url.Parse(attURL)
	if err != nil {
		return "", 0, err
	}

	var httpClient *http.Client
	var attRequest *http.Request

	handler := GetHandler(channel.ChannelType())
	builder, isBuilder := handler.(AttachmentRequestBuilder)
	if isBuilder {
		httpClient = builder.AttachmentRequestClient(channel)
		attRequest, err = builder.BuildAttachmentRequest(ctx, b, channel, parsedURL.String())
	} else {
		httpClient = utils.GetHTTPClient()
		attRequest, err = http.NewRequest(http.MethodGet, attURL, nil)
	}

	if err != nil {
		return "", 0, errors.Wrap(err, "unable to create attachment request")
	}

	trace, err := httpx.DoTrace(httpClient, attRequest, nil, nil, maxAttBodyReadBytes)
	if trace != nil {
		clog.HTTP(trace)
	}
	if err != nil {
		return "", 0, err
	}

	mimeType := ""
	extension := filepath.Ext(parsedURL.Path)
	if extension != "" {
		extension = extension[1:]
	}

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

	// we still don't know our mime type, use our content header instead
	if mimeType == "" {
		mimeType, _, _ = mime.ParseMediaType(trace.Response.Header.Get("Content-Type"))
		if extension == "" {
			extensions, err := mime.ExtensionsByType(mimeType)
			if extensions == nil || err != nil {
				extension = ""
			} else {
				extension = extensions[0][1:]
			}
		}
	}

	newURL, err := b.SaveAttachment(ctx, channel, mimeType, trace.ResponseBody, extension)
	return newURL, len(trace.ResponseBody), err
}
