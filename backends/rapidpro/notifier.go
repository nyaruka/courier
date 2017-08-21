package rapidpro

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/config"
	"github.com/nyaruka/courier/utils"
	"github.com/sirupsen/logrus"
)

func notifyRapidPro(config *config.Courier, body url.Values) error {
	// build our request
	req, err := http.NewRequest("POST", fmt.Sprintf(config.RapidproHandleURL, body.Get("action")), strings.NewReader(body.Encode()))

	// this really should never happen, but if it does we only log it
	if err != nil {
		logrus.WithField("comp", "notifier").WithError(err).Error("error creating request")
		return nil
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", fmt.Sprintf("Token %s", config.RapidproToken))
	_, err = utils.MakeHTTPRequest(req)

	return err
}

func newNotifier(config *config.Courier) *notifier {
	return &notifier{
		config:        config,
		notifications: make(chan url.Values, 100000), // TODO: is 100k enough?
	}
}

func (n *notifier) addHandleMsgNotification(msgID courier.MsgID) {
	body := url.Values{}
	body.Add("action", "handle_message")
	body.Add("message_id", msgID.String())
	n.notifications <- body
}

func (n *notifier) addStopContactNotification(contactID ContactID) {
	body := url.Values{}
	body.Add("action", "stop_contact")
	body.Add("contact_id", fmt.Sprintf("%d", contactID.Int64))
	n.notifications <- body
}

func (n *notifier) addTriggerNewConversation(contactID ContactID, channelID courier.ChannelID) {
	body := url.Values{}
	body.Add("action", "trigger_new_conversation")
	body.Add("contact_id", fmt.Sprintf("%d", contactID.Int64))
	body.Add("channel_id", fmt.Sprintf("%d", channelID.Int64))
	n.notifications <- body
}

func (n *notifier) start(backend *backend) {
	go func() {
		backend.waitGroup.Add(1)
		defer backend.waitGroup.Done()

		log := logrus.WithField("comp", "notifier")
		log.WithField("state", "started").Info("notifier started")

		lastError := false

		for {
			select {
			case body := <-n.notifications:
				// try to notify rapidpro
				err := notifyRapidPro(n.config, body)

				// we failed, append it to our retries
				if err != nil {
					if !lastError {
						log.WithError(err).WithField("body", body).Error("error notifying rapidpro")
					}
					n.retries = append(n.retries, body)
					lastError = true
				} else {
					lastError = false
				}

				// otherwise, all is well, move onto the next

			case <-backend.stopChan:
				// we are being stopped, exit
				log.WithField("state", "stopped").Info("notifier stopped")
				return

			case <-time.After(500 * time.Millisecond):
				// if we are quiet for 500ms, try to send some retries
				retried := 0
				for retried < 10 && retried < len(n.retries) {
					body := n.retries[0]
					n.retries = n.retries[1:]

					err := notifyRapidPro(n.config, body)
					if err != nil {
						if !lastError {
							log.WithError(err).Error("error notifying rapidpro")
						}
						n.retries = append(n.retries, body)
						lastError = true
					} else {
						lastError = false
					}

					retried++
				}
			}
		}
	}()
}

type notifier struct {
	config        *config.Courier
	notifications chan url.Values
	retries       []url.Values
}
