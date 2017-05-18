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

func notifyRapidPro(config *config.Courier, msgID courier.MsgID) error {
	// our form is just the id of the message to handle
	body := url.Values{}
	body.Add("message_id", msgID.String())

	// build our request
	req, err := http.NewRequest("POST", config.RapidproHandleURL, strings.NewReader(body.Encode()))

	// this really should never happen, but if it does we ignore it
	if err != nil {
		logrus.WithField("comp", "notifier").WithError(err).Error("error creating request")
		return nil
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("AUTHORIZATION", fmt.Sprintf("Token %s", config.RapidproToken))
	_, _, err = utils.MakeHTTPRequest(req)

	return err
}

func newNotifier(config *config.Courier) *notifier {
	return &notifier{
		config:    config,
		msgIDChan: make(chan courier.MsgID, 100000), // TODO: is 100k enough?
	}
}

func (n *notifier) addMsg(msgID courier.MsgID) {
	n.msgIDChan <- msgID
}

func (n *notifier) start(backend *backend) {
	go func() {
		backend.waitGroup.Add(1)
		defer backend.waitGroup.Done()

		log := logrus.WithField("comp", "notifier")
		log.WithField("state", "started").Info("notifier started")

		for {
			select {
			case msgID := <-n.msgIDChan:
				// if this failed, rapidpro is likely down, push it onto our retry list
				err := notifyRapidPro(n.config, msgID)

				// we failed, append it to our retries
				if err != nil {
					log.WithError(err).Error("error notifying rapidpro")
					n.retries = append(n.retries, msgID)
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
					msgID := n.retries[0]
					n.retries = n.retries[1:]

					err := notifyRapidPro(n.config, msgID)
					if err != nil {
						log.WithError(err).Error("error notifying rapidpro")
						n.retries = append(n.retries, msgID)
					}
					retried++
				}
			}
		}
	}()
}

type notifier struct {
	config    *config.Courier
	msgIDChan chan courier.MsgID
	retries   []courier.MsgID
}
