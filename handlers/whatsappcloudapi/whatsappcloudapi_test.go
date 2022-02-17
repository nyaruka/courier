package whatsappcloudapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/handlers"
	. "github.com/nyaruka/courier/handlers"
	"github.com/stretchr/testify/assert"
)

var testChannelsCWA = []courier.Channel{
	courier.NewMockChannel("8eb23e93-5ecb-45ba-b726-3b064e0c568c", "CWA", "12345", "", map[string]interface{}{courier.ConfigAuthToken: "a123"}),
}

var cwaReceiveURL = "/c/cwa/receive"

var helloCWA = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
                        "messages": [
                            {
                                "from": "5678",
                                "id": "external_id",
                                "timestamp": "1454119029",
                                "text": {
                                    "body": "Hello World"
                                },
                                "type": "text"
                            }
                        ]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var duplicatedMsg = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
                        "messages": [
                            {
                                "from": "5678",
                                "id": "external_id",
                                "timestamp": "1454119029",
                                "text": {
                                    "body": "Hello World"
                                },
                                "type": "text"
                            },{
                                "from": "5678",
                                "id": "external_id",
                                "timestamp": "1454119029",
                                "text": {
                                    "body": "Hello World"
                                },
                                "type": "text"
                            }
                        ]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}
`

var voiceMsg = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
						"messages":[{
							"from": "5678",
							"id": "external_id",
							"timestamp": "1454119029",
							"type": "voice",
							"voice": {
								"file": "/usr/local/wamedia/shared/463e/b7ec/ff4e4d9bb1101879cbd411b2",
								"id": "id_voice",
								"mime_type": "audio/ogg; codecs=opus",
								"sha256": "fa9e1807d936b7cebe63654ea3a7912b1fa9479220258d823590521ef53b0710"}
					  }]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var buttonMsg = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
						"messages": [
        {
            "button": {
                "payload": "No-Button-Payload",
                "text": "No"
            },
            "context": {
                "from": "5678",
                "id": "gBGGFmkiWVVPAgkgQkwi7IORac0"
            },
            "from": "5678",
            "id": "external_id",
            "timestamp": "1454119029",
            "type": "button"
        }
    ]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var documentMsg = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
						"messages": [{
							"from": "5678",
							"id": "external_id",
							"timestamp": "1454119029",
							"type": "document",
							"document": {
							  "caption": "80skaraokesonglistartist",
									"file": "/usr/local/wamedia/shared/fc233119-733f-49c-bcbd-b2f68f798e33",
									"id": "id_document",
									"mime_type": "application/pdf",
									"sha256": "3b11fa6ef2bde1dd14726e09d3edaf782120919d06f6484f32d5d5caa4b8e"
								}
							}]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var imageMsg = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
						"messages": [{
							"from": "5678",
							"id": "external_id",
							"image": {
								"file": "/usr/local/wamedia/shared/b1cf38-8734-4ad3-b4a1-ef0c10d0d683",
								"id": "id_image",
								"mime_type": "image/jpeg",
								"sha256": "29ed500fa64eb55fc19dc4124acb300e5dcc54a0f822a301ae99944db",
								"caption": "Check out my new phone!"
							},
							"timestamp": "1454119029",
							"type": "image"
						}]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var videoMsg = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
						"messages": [{
							"from": "5678",
							"id": "external_id",
							"video": {
								"file": "/usr/local/wamedia/shared/b1cf38-8734-4ad3-b4a1-ef0c10d0d683",
								"id": "id_video",
								"mime_type": "image/jpeg",
								"sha256": "29ed500fa64eb55fc19dc4124acb300e5dcc54a0f822a301ae99944db",
								"caption": "Check out my new phone!"
							},
							"timestamp": "1454119029",
							"type": "video"
						}]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var audioMsg = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
						"messages": [{
							"from": "5678",
							"id": "external_id",
							"audio": {
								"file": "/usr/local/wamedia/shared/b1cf38-8734-4ad3-b4a1-ef0c10d0d683",
								"id": "id_audio",
								"mime_type": "image/jpeg",
								"sha256": "29ed500fa64eb55fc19dc4124acb300e5dcc54a0f822a301ae99944db",
								"caption": "Check out my new phone!"
							},
							"timestamp": "1454119029",
							"type": "audio"
						}]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var locationMsg = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
						"messages":[{
							"from":"5678",
							"id":"external_id",
							"location":{
							   "address":"Main Street Beach, Santa Cruz, CA",
							   "latitude":0.000000,
							   "longitude":1.000000,
							   "name":"Main Street Beach",
							   "url":"https://foursquare.com/v/4d7031d35b5df7744"},
							"timestamp":"1454119029",
							"type":"location"
						  }]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var invalidMsg = `not json`

var invalidFrom = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "bla"
                            }
                        ],
                        "messages": [
                            {
                                "from": "bla",
                                "id": "external_id",
                                "timestamp": "1454119029",
                                "text": {
                                    "body": "Hello World"
                                },
                                "type": "text"
                            }
                        ]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var invalidTimestamp = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "bla"
                            }
                        ],
                        "messages": [
                            {
                                "from": "bla",
                                "id": "external_id",
                                "timestamp": "asdf",
                                "text": {
                                    "body": "Hello World"
                                },
                                "type": "text"
                            }
                        ]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var validStatusCWA = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
                        "statuses": [{
							"id": "external_id",
							"recipient_id": "5678",
							"status": "sent",
							"timestamp": "1454119029",
							"type": "message",
							"conversation": {
							  "id": "CONVERSATION_ID",
							  "expiration_timestamp": 1454119029,
							  "origin": {
								 "type": "referral_conversion"
							  }
							},
							"pricing": {
							  "pricing_model": "CBP",
							  "billable": false,
							  "category": "referral_conversion"
							}
						   }]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var invalidStatusCWA = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
                        "statuses": [{
							"id": "external_id",
							"recipient_id": "5678",
							"status": "in_orbit",
							"timestamp": "1454119029",
							"type": "message",
							"conversation": {
							  "id": "CONVERSATION_ID",
							  "expiration_timestamp": 1454119029,
							  "origin": {
								 "type": "referral_conversion"
							  }
							},
							"pricing": {
							  "pricing_model": "CBP",
							  "billable": false,
							  "category": "referral_conversion"
							}
						   }]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var ignoreStatusCWA = `{
    "object": "whatsapp_business_account",
    "entry": [
        {
            "id": "8856996819413533",
            "changes": [
                {
                    "value": {
                        "messaging_product": "whatsapp",
                        "metadata": {
                            "display_phone_number": "12345",
                            "phone_number_id": "27681414235104944"
                        },
                        "contacts": [
                            {
                                "profile": {
                                    "name": "Kerry Fisher"
                                },
                                "wa_id": "5678"
                            }
                        ],
                        "statuses": [{
							"id": "external_id",
							"recipient_id": "5678",
							"status": "deleted",
							"timestamp": "1454119029",
							"type": "message",
							"conversation": {
							  "id": "CONVERSATION_ID",
							  "expiration_timestamp": 1454119029,
							  "origin": {
								 "type": "referral_conversion"
							  }
							},
							"pricing": {
							  "pricing_model": "CBP",
							  "billable": false,
							  "category": "referral_conversion"
							}
						   }]
                    },
                    "field": "messages"
                }
            ]
        }
    ]
}`

var testCasesCWA = []ChannelHandleTestCase{
	{Label: "Receive Message CWA", URL: cwaReceiveURL, Data: helloCWA, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Duplicate Valid Message", URL: cwaReceiveURL, Data: duplicatedMsg, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Hello World"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Valid Voice Message", URL: cwaReceiveURL, Data: voiceMsg, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp(""), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Voice"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Valid Button Message", URL: cwaReceiveURL, Data: buttonMsg, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("No"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Valid Document Message", URL: cwaReceiveURL, Data: documentMsg, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("80skaraokesonglistartist"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Document"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Image Message", URL: cwaReceiveURL, Data: imageMsg, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Check out my new phone!"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Image"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Video Message", URL: cwaReceiveURL, Data: videoMsg, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Check out my new phone!"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Video"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Audio Message", URL: cwaReceiveURL, Data: audioMsg, Status: 200, Response: "Handled", NoQueueErrorCheck: true, NoInvalidChannelCheck: true,
		Text: Sp("Check out my new phone!"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Attachment: Sp("https://foo.bar/attachmentURL_Audio"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},
	{Label: "Receive Valid Location Message", URL: cwaReceiveURL, Data: locationMsg, Status: 200, Response: `"type":"msg"`,
		Text: Sp(""), Attachment: Sp("geo:0.000000,1.000000"), URN: Sp("whatsapp:5678"), ExternalID: Sp("external_id"), Date: Tp(time.Date(2016, 1, 30, 1, 57, 9, 0, time.UTC)),
		PrepRequest: addValidSignature},

	{Label: "Receive Invalid JSON", URL: cwaReceiveURL, Data: invalidMsg, Status: 400, Response: "unable to parse", PrepRequest: addValidSignature},
	{Label: "Receive Invalid JSON", URL: cwaReceiveURL, Data: invalidFrom, Status: 400, Response: "invalid whatsapp id", PrepRequest: addValidSignature},
	{Label: "Receive Invalid JSON", URL: cwaReceiveURL, Data: invalidTimestamp, Status: 400, Response: "invalid timestamp", PrepRequest: addValidSignature},

	{Label: "Receive Valid Status", URL: cwaReceiveURL, Data: validStatusCWA, Status: 200, Response: `"type":"status"`,
		MsgStatus: Sp("S"), ExternalID: Sp("external_id"), PrepRequest: addValidSignature},
	{Label: "Receive Invalid Status", URL: cwaReceiveURL, Data: invalidStatusCWA, Status: 400, Response: `"unknown status: in_orbit"`, PrepRequest: addValidSignature},
	{Label: "Receive Ignore Status", URL: cwaReceiveURL, Data: ignoreStatusCWA, Status: 200, Response: `"ignoring status: deleted"`, PrepRequest: addValidSignature},
}

func addValidSignature(r *http.Request) {
	body, _ := handlers.ReadBody(r, 100000)
	sig, _ := fbCalculateSignature("fb_app_secret", body)
	r.Header.Set(signatureHeader, fmt.Sprintf("sha1=%s", string(sig)))
}

func addInvalidSignature(r *http.Request) {
	r.Header.Set(signatureHeader, "invalidsig")
}

func TestHandler(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accessToken := r.Header.Get("Authorization")
		defer r.Body.Close()

		// invalid auth token
		if accessToken != "Bearer a123" {
			fmt.Printf("Access token: %s\n", accessToken)
			http.Error(w, "invalid auth token", 403)
			return
		}

		if strings.HasSuffix(r.URL.Path, "image") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Image"}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "audio") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Audio"}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "voice") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Voice"}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "video") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Video"}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "document") {
			w.Write([]byte(`{"url": "https://foo.bar/attachmentURL_Document"}`))
			return
		}

		// valid token
		w.Write([]byte(`{"url": "https://foo.bar/attachmentURL"}`))

	}))
	graphURL = server.URL

	RunChannelTestCases(t, testChannelsCWA, newHandler("CWA", "Cloud API WhatsApp", false), testCasesCWA)
}

func TestSigning(t *testing.T) {
	tcs := []struct {
		Body      string
		Signature string
	}{
		{
			"hello world",
			"308de7627fe19e92294c4572a7f831bc1002809d",
		},
		{
			"hello world2",
			"ab6f902b58b9944032d4a960f470d7a8ebfd12b7",
		},
	}

	for i, tc := range tcs {
		sig, err := fbCalculateSignature("sesame", []byte(tc.Body))
		assert.NoError(t, err)
		assert.Equal(t, tc.Signature, sig, "%d: mismatched signature", i)
	}
}
