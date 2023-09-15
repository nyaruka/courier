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
		UserRef string `json:"user_ref,omitempty"`
		ID      string `json:"id,omitempty"`
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
		URL        string `json:"url"`
		IsReusable bool   `json:"is_reusable"`
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
