package external

import (
	"net/http"
	"testing"

	"github.com/nyaruka/courier/v26/core/models"
	. "github.com/nyaruka/courier/v26/handlers/handlertest"
	"github.com/nyaruka/courier/v26/test"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/urns"
)

func newChannel(country i18n.Country, schemes []string, config map[string]any) *models.Channel {
	return test.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c56ab", "EX", "2020", country, schemes, config)
}

func TestIncoming(t *testing.T) {
	phone := []string{urns.Phone.Prefix}

	RunIncomingTests(t, []*models.Channel{newChannel("US", phone, nil)}, newHandler, "testdata/incoming.json", nil)

	soapChannel := newChannel("US", phone, map[string]any{
		configTextXPath:             "//content",
		configFromXPath:             "//source",
		configMOResponse:            "<?xml version=“1.0”?><return>0</return>",
		configMOResponseContentType: "text/xml",
	})
	RunIncomingTests(t, []*models.Channel{soapChannel}, newHandler, "testdata/incoming_soap.json", nil)

	RunIncomingTests(t, []*models.Channel{newChannel("GM", phone, nil)}, newHandler, "testdata/incoming_gm.json", nil)

	customFieldsChannel := newChannel("US", phone, map[string]any{
		configMOFromField: "from_number",
		configMODateField: "timestamp",
		configMOTextField: "messageText",
	})
	RunIncomingTests(t, []*models.Channel{customFieldsChannel}, newHandler, "testdata/incoming_custom_fields.json", nil)

	extChannel := newChannel("GM", []string{urns.External.Prefix}, nil)
	RunIncomingTests(t, []*models.Channel{extChannel}, newHandler, "testdata/incoming_ext_urns.json", nil)
}

func TestOutgoing(t *testing.T) {
	phone := []string{urns.Phone.Prefix}

	getChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:    "http://example.com/send?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
		models.ConfigSendMethod: http.MethodGet,
	})
	RunOutgoingTests(t, getChannel, newHandler, "testdata/outgoing_get.json", nil)

	getSmartChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:    "http://example.com/send?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
		configEncoding:          encodingSmart,
		models.ConfigSendMethod: http.MethodGet,
	})
	RunOutgoingTests(t, getSmartChannel, newHandler, "testdata/outgoing_get_smart.json", nil)

	postChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:    "http://example.com/send",
		models.ConfigSendBody:   "to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
		models.ConfigSendMethod: http.MethodPost,
	})
	RunOutgoingTests(t, postChannel, newHandler, "testdata/outgoing_post.json", nil)

	postContentTypeChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:     "http://example.com/send",
		models.ConfigSendBody:    "to={{to_no_plus}}&text={{text}}&from={{from_no_plus}}{{quick_replies}}",
		models.ConfigContentType: "application/x-www-form-urlencoded; charset=utf-8",
		models.ConfigSendMethod:  http.MethodPost,
	})
	RunOutgoingTests(t, postContentTypeChannel, newHandler, "testdata/outgoing_post_content_type.json", nil)

	postSmartChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:    "http://example.com/send",
		models.ConfigSendBody:   "to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
		configEncoding:          encodingSmart,
		models.ConfigSendMethod: http.MethodPost,
	})
	RunOutgoingTests(t, postSmartChannel, newHandler, "testdata/outgoing_post_smart.json", nil)

	jsonChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:     "http://example.com/send",
		models.ConfigSendBody:    `{ "to":{{to}}, "text":{{text}}, "from":{{from}}, "quick_replies":{{quick_replies}} }`,
		models.ConfigContentType: contentJSON,
		models.ConfigSendMethod:  http.MethodPost,
		models.ConfigSendHeaders: map[string]any{"Authorization": "Token ABCDEF", "foo": "bar"},
	})
	RunOutgoingTests(t, jsonChannel, newHandler, "testdata/outgoing_json.json", nil)

	xmlChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:     "http://example.com/send",
		models.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from><quick_replies>{{quick_replies}}</quick_replies></msg>`,
		models.ConfigContentType: contentXML,
		models.ConfigSendMethod:  http.MethodPut,
	})
	RunOutgoingTests(t, xmlChannel, newHandler, "testdata/outgoing_xml.json", nil)

	xmlResponseCheckChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:     "http://example.com/send",
		models.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from><quick_replies>{{quick_replies}}</quick_replies></msg>`,
		configMTResponseCheck:    "<return>0</return>",
		models.ConfigContentType: contentXML,
		models.ConfigSendMethod:  http.MethodPut,
	})
	RunOutgoingTests(t, xmlResponseCheckChannel, newHandler, "testdata/outgoing_xml_response_check.json", nil)

	// max length can be configured as an int or a string
	getMaxLengthChannel := newChannel("US", phone, map[string]any{
		models.ConfigMaxLength:  30,
		models.ConfigSendURL:    "http://example.com/send?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
		models.ConfigSendMethod: http.MethodGet,
	})
	RunOutgoingTests(t, getMaxLengthChannel, newHandler, "testdata/outgoing_get_max_length.json", nil)

	getMaxLengthStrChannel := newChannel("US", phone, map[string]any{
		models.ConfigMaxLength:  "30",
		models.ConfigSendURL:    "http://example.com/send?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
		models.ConfigSendMethod: http.MethodGet,
	})
	RunOutgoingTests(t, getMaxLengthStrChannel, newHandler, "testdata/outgoing_get_max_length_str.json", nil)

	jsonMaxLengthChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:     "http://example.com/send",
		models.ConfigMaxLength:   30,
		models.ConfigSendBody:    `{ "to":{{to}}, "text":{{text}}, "from":{{from}}, "quick_replies":{{quick_replies}} }`,
		models.ConfigContentType: contentJSON,
		models.ConfigSendMethod:  http.MethodPost,
		models.ConfigSendHeaders: map[string]any{"Authorization": "Token ABCDEF", "foo": "bar"},
	})
	RunOutgoingTests(t, jsonMaxLengthChannel, newHandler, "testdata/outgoing_json_max_length.json", nil)

	xmlMaxLengthChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:     "http://example.com/send",
		models.ConfigMaxLength:   30,
		models.ConfigSendBody:    `<msg><to>{{to}}</to><text>{{text}}</text><from>{{from}}</from><quick_replies>{{quick_replies}}</quick_replies></msg>`,
		models.ConfigContentType: contentXML,
		models.ConfigSendMethod:  http.MethodPost,
		models.ConfigSendHeaders: map[string]any{"Authorization": "Token ABCDEF", "foo": "bar"},
	})
	RunOutgoingTests(t, xmlMaxLengthChannel, newHandler, "testdata/outgoing_xml_max_length.json", nil)

	nationalChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:    "http://example.com/send?to={{to}}&text={{text}}&from={{from}}{{quick_replies}}",
		"use_national":          true,
		models.ConfigSendMethod: http.MethodGet,
	})
	RunOutgoingTests(t, nationalChannel, newHandler, "testdata/outgoing_get_national.json", nil)

	jsonAuthorizationChannel := newChannel("US", phone, map[string]any{
		models.ConfigSendURL:           "http://example.com/send",
		models.ConfigSendBody:          `{ "to":{{to}}, "text":{{text}}, "from":{{from}}, "quick_replies":{{quick_replies}} }`,
		models.ConfigContentType:       contentJSON,
		models.ConfigSendMethod:        http.MethodPost,
		models.ConfigSendAuthorization: "Token ABCDEF",
	})
	RunOutgoingTests(t, jsonAuthorizationChannel, newHandler, "testdata/outgoing_json_authorization.json", &OutgoingOptions{CheckRedacted: []string{"Token ABCDEF"}})
}
