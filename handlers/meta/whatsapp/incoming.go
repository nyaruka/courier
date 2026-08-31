package whatsapp

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/nyaruka/courier/v26/core/channels"
	"github.com/nyaruka/courier/v26/core/models"
	"github.com/nyaruka/gocommon/urns"
)

// ResolveMediaFunc asks the provider for the URL of a message's media, given its ID. It's the one thing that
// differs between the channel types that deliver WhatsApp payloads - who they ask, and with whose credentials.
type ResolveMediaFunc func(mediaID string) (string, error)

// ParseChanges turns the changes carried by a WhatsApp notification into the messages and statuses they
// contained, adding them to the given batch without writing any of them - which is what lets the whole batch be
// written together, and keeps a failure part way through it from leaving a response that describes more than we
// actually did. It still does I/O, since resolving a message's media means asking the provider for its URL, but
// it makes no changes.
//
// A returned error means the payload itself is malformed - a message we can't read a timestamp or a sender from
// - so asking for it again wouldn't get any further. Callers whose provider retries on an error response answer
// it as ignored instead.
func ParseChanges(channel *models.Channel, changes []Change, resolveMedia ResolveMediaFunc, r *http.Request, in *channels.Received, clog *models.ChannelLog) error {
	seenMsgIDs := make(map[string]bool, 2)
	contactNames := make(map[string]string)

	for _, change := range changes {
		// contacts are keyed by both identifiers they can carry, as a message from a user with a username may
		// only reference them by their user_id
		for _, contact := range change.Value.Contacts {
			if contact.WaID != "" {
				contactNames[contact.WaID] = contact.Profile.Name
			}
			if contact.UserID != "" {
				contactNames[contact.UserID] = contact.Profile.Name
			}
		}

		for _, waMsg := range change.Value.Messages {
			if seenMsgIDs[waMsg.ID] {
				continue
			}

			if waMsg.GroupID != "" {
				in.Ignored("ignoring group message")
				continue
			}

			date, urn, text, mediaURL, mediaID, err, finalErr := waMsg.ExtractData(clog)
			if finalErr != nil {
				return finalErr
			}

			if err != nil {
				channels.LogRequestError(r, channel, err)
				continue
			}

			if mediaID != "" && mediaURL == "" {
				mediaURL, err = resolveMedia(mediaID)
				// we had an error downloading media
				if err != nil {
					channels.LogRequestError(r, channel, err)
				}
			}

			// create our message
			event := models.NewIncomingMsg(channel, urn, text, waMsg.ID, clog).WithReceivedOn(date).WithContactName(contactNames[waMsg.Identifier()])

			if mediaURL != "" {
				event.WithAttachment(mediaURL)
			}

			if payload := waMsg.ExtractPayload(); payload != nil {
				event.WithPayload(payload)
			} else if waMsg.Interactive.Type == "nfm_reply" && waMsg.Interactive.NFMReply.ResponseJSON != "" {
				channels.LogRequestError(r, channel, errors.New("nfm_reply response_json is not a valid JSON object"))
			}

			// if we have a user_id, add it as a secondary whatsapp URN (unless it's already the primary URN)
			if waMsg.FromUserID != "" {
				userIDURN, urnErr := urns.New(urns.WhatsApp, waMsg.FromUserID)
				if urnErr == nil {
					if userIDURN != urn {
						event.WithNewURN(userIDURN, models.NewURNAppend)
					}
				} else {
					channels.LogRequestError(r, channel, fmt.Errorf("invalid user_id for whatsapp URN: %w", urnErr))
				}
			}

			in.Msg(event)
			seenMsgIDs[waMsg.ID] = true
		}

		for _, status := range change.Value.Statuses {
			msgStatus, found := StatusMapping[status.Status]
			if !found {
				if IgnoreStatuses[status.Status] {
					in.Ignored(fmt.Sprintf("ignoring status: %s", status.Status))
				} else {
					// a status in neither map is one WhatsApp has added since we last looked, so it's logged
					// where we'll see it rather than only at debug
					channels.LogRequestError(r, channel, fmt.Errorf("unknown status: %s", status.Status))
					in.Ignored(fmt.Sprintf("unknown status: %s", status.Status))
				}
				continue
			}

			for _, statusError := range status.Errors {
				statusError.ErrorChannelLog(clog)
			}

			in.Status(models.NewStatusUpdateByExternalID(channel, status.ID, msgStatus, clog))
		}

		for _, chError := range change.Value.Errors {
			chError.ErrorChannelLog(clog)
		}
	}

	return nil
}
