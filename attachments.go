package courier

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/nyaruka/courier/utils"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/pkg/errors"
	"gopkg.in/h2non/filetype.v1"
)

const (
	maxAttBodyReadBytes = 100 * 1024 * 1024
)

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
