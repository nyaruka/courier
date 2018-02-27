package chatbase

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"github.com/nyaruka/courier/utils"
)

// ChatbaseAPIURL is the URL chatbase API messages will be sent to
var chatbaseAPIURL = "https://chatbase.com/api/message"

// chatbaseLog is the payload for a chatbase request
type chatbaseLog struct {
	Type      string `json:"type"`
	UserID    string `json:"user_id"`
	Platform  string `json:"platform"`
	Message   string `json:"message"`
	TimeStamp int64  `json:"time_stamp"`

	APIKey     string `json:"api_key"`
	APIVersion string `json:"version,omitempty"`
}

// SendChatbaseMessage sends a chatbase message with the passed in api key and message details
func SendChatbaseMessage(apiKey string, apiVersion string, messageType string, userID string, platform string, message string, timestamp time.Time) error {
	body := chatbaseLog{
		Type:      messageType,
		UserID:    userID,
		Platform:  platform,
		Message:   message,
		TimeStamp: timestamp.UnixNano() / int64(time.Millisecond),

		APIKey:     apiKey,
		APIVersion: apiVersion,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, _ := http.NewRequest(http.MethodPost, chatbaseAPIURL, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")

	_, err = utils.MakeHTTPRequest(req)
	return err
}
