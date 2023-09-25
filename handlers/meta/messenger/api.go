package messenger

//	{
//	  "messaging_type": "<MESSAGING_TYPE>"
//	  "recipient": {
//	    "id":"<PSID>"
//	  },
//	  "message": {
//	    "text":"hello, world!"
//	    "attachment":{
//	      "type":"image",
//	      "payload":{
//	        "url":"http://www.messenger-rocks.com/image.jpg",
//	        "is_reusable":true
//	      }
//	    }
//	  }
//	}
type SendRequest struct {
	MessagingType string `json:"messaging_type"`
	Tag           string `json:"tag,omitempty"`
	Recipient     struct {
		UserRef                   string `json:"user_ref,omitempty"`
		ID                        string `json:"id,omitempty"`
		NotificationMessagesToken string `json:"notification_messages_token,omitempty"`
	} `json:"recipient"`
	Message struct {
		Text         string       `json:"text,omitempty"`
		QuickReplies []QuickReply `json:"quick_replies,omitempty"`
		Attachment   *Attachment  `json:"attachment,omitempty"`
	} `json:"message"`
}

type Attachment struct {
	Type    string `json:"type"`
	Payload struct {
		URL        string `json:"url,omitempty"`
		IsReusable bool   `json:"is_reusable,omitempty"`

		TemplateType string `json:"template_type,omitempty"`
		Title        string `json:"title,omitempty"`
		Payload      string `json:"payload,omitempty"`
	} `json:"payload"`
}

type QuickReply struct {
	Title       string `json:"title"`
	Payload     string `json:"payload"`
	ContentType string `json:"content_type"`
}

type SendResponse struct {
	ExternalID  string `json:"message_id"`
	RecipientID string `json:"recipient_id"`
	Error       struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error"`
}

// see https://developers.facebook.com/docs/messenger-platform/webhooks/#event-notifications
type Messaging struct {
	Sender *struct {
		ID      string `json:"id"`
		UserRef string `json:"user_ref,omitempty"`
	} `json:"sender"`
	Recipient *struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Timestamp int64 `json:"timestamp"`

	OptIn *struct {
		Type                          string `json:"type"`
		Payload                       string `json:"payload"`
		NotificationMessagesToken     string `json:"notification_messages_token"`
		NotificationMessagesTimezone  string `json:"notification_messages_timezone"`
		NotificationMessagesFrequency string `json:"notification_messages_frequency"`
		NotificationMessagesStatus    string `json:"notification_messages_status"`
		TokenExpiryTimestamp          int64  `json:"token_expiry_timestamp"`
		UserTokenStatus               string `json:"user_token_status"`
		Title                         string `json:"title"`

		Ref     string `json:"ref"`
		UserRef string `json:"user_ref"`
	} `json:"optin"`

	Referral *struct {
		Ref    string `json:"ref"`
		Source string `json:"source"`
		Type   string `json:"type"`
		AdID   string `json:"ad_id"`
	} `json:"referral"`

	Postback *struct {
		MID      string `json:"mid"`
		Title    string `json:"title"`
		Payload  string `json:"payload"`
		Referral struct {
			Ref    string `json:"ref"`
			Source string `json:"source"`
			Type   string `json:"type"`
			AdID   string `json:"ad_id"`
		} `json:"referral"`
	} `json:"postback"`

	Message *struct {
		IsEcho      bool   `json:"is_echo"`
		MID         string `json:"mid"`
		Text        string `json:"text"`
		IsDeleted   bool   `json:"is_deleted"`
		Attachments []struct {
			Type    string `json:"type"`
			Payload *struct {
				URL         string `json:"url"`
				StickerID   int64  `json:"sticker_id"`
				Coordinates *struct {
					Lat  float64 `json:"lat"`
					Long float64 `json:"long"`
				} `json:"coordinates"`
			}
		} `json:"attachments"`
	} `json:"message"`

	Delivery *struct {
		MIDs      []string `json:"mids"`
		Watermark int64    `json:"watermark"`
	} `json:"delivery"`
}
