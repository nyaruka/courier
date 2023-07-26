package rapidpro

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gomodule/redigo/redis"
	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v2"
	"github.com/nyaruka/redisx/assertredis"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

type BackendTestSuite struct {
	suite.Suite
	b *backend
}

func testConfig() *courier.Config {
	config := courier.NewConfig()
	config.DB = "postgres://courier_test:temba@localhost:5432/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	config.MediaDomain = "nyaruka.s3.com"
	return config
}

func (ts *BackendTestSuite) SetupSuite() {
	storageDir = "_test_storage"

	// turn off logging
	logrus.SetOutput(io.Discard)

	b, err := courier.NewBackend(testConfig())
	if err != nil {
		log.Fatalf("unable to create rapidpro backend: %v", err)
	}
	ts.b = b.(*backend)

	err = ts.b.Start()
	if err != nil {
		log.Fatalf("unable to start backend for testing: %v", err)
	}

	// read our schema sql
	sqlSchema, err := os.ReadFile("schema.sql")
	if err != nil {
		panic(fmt.Errorf("Unable to read schema.sql: %s", err))
	}
	ts.b.db.MustExec(string(sqlSchema))

	// read our testdata sql
	sql, err := os.ReadFile("testdata.sql")
	if err != nil {
		panic(fmt.Errorf("Unable to read testdata.sql: %s", err))
	}
	ts.b.db.MustExec(string(sql))

	// clear redis
	r := ts.b.redisPool.Get()
	defer r.Close()
	_, err = r.Do("FLUSHDB")
	ts.Require().NoError(err)
}

func (ts *BackendTestSuite) TearDownSuite() {
	ts.b.Stop()
	ts.b.Cleanup()

	if err := os.RemoveAll(storageDir); err != nil {
		panic(err)
	}
}

func (ts *BackendTestSuite) getChannel(cType string, cUUID string) *DBChannel {
	channelUUID := courier.ChannelUUID(cUUID)

	channel, err := ts.b.GetChannel(context.Background(), courier.ChannelType(cType), channelUUID)
	ts.Require().NoError(err, "error getting channel")
	ts.Require().NotNil(channel)

	return channel.(*DBChannel)
}

func (ts *BackendTestSuite) TestMsgUnmarshal() {
	msgJSON := `{
		"attachments": ["https://foo.bar/image.jpg"],
		"quick_replies": ["Yes", "No"],
		"text": "Test message 21",
		"contact_id": 30,
		"contact_urn_id": 14,
		"flow": {"uuid": "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", "name": "Favorites"},
		"id": 204,
		"channel_uuid": "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba",
		"uuid": "54c893b9-b026-44fc-a490-50aed0361c3f",
		"next_attempt": "2017-07-21T19:22:23.254182Z",
		"urn": "telegram:3527065",
		"urn_auth": "5ApPVsFDcFt:RZdK9ne7LgfvBYdtCYg7tv99hC9P2",
		"org_id": 1,
		"created_on": "2017-07-21T19:22:23.242757Z",
		"high_priority": true,
		"response_to_external_id": "external-id",
		"is_resend": true,
		"metadata": {"topic": "event"}
	}`

	msg := DBMsg{}
	err := json.Unmarshal([]byte(msgJSON), &msg)
	ts.NoError(err)
	ts.Equal(courier.ChannelUUID("f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba"), msg.ChannelUUID_)
	ts.Equal([]string{"https://foo.bar/image.jpg"}, msg.Attachments())
	ts.Equal("5ApPVsFDcFt:RZdK9ne7LgfvBYdtCYg7tv99hC9P2", msg.URNAuth_)
	ts.Equal("", msg.ExternalID())
	ts.Equal([]string{"Yes", "No"}, msg.QuickReplies())
	ts.Equal("event", msg.Topic())
	ts.Equal("external-id", msg.ResponseToExternalID())
	ts.True(msg.HighPriority())
	ts.True(msg.IsResend())
	flow_ref := courier.FlowReference{UUID: "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", Name: "Favorites"}
	ts.Equal(&flow_ref, msg.Flow())
	ts.Equal("Favorites", msg.FlowName())
	ts.Equal("9de3663f-c5c5-4c92-9f45-ecbc09abcc85", msg.FlowUUID())

	msgJSONNoQR := `{
		"text": "Test message 21",
		"contact_id": 30,
		"contact_urn_id": 14,
		"id": 204,
		"channel_uuid": "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba",
		"uuid": "54c893b9-b026-44fc-a490-50aed0361c3f",
		"next_attempt": "2017-07-21T19:22:23.254182Z",
		"urn": "telegram:3527065",
		"org_id": 1,
		"created_on": "2017-07-21T19:22:23.242757Z",
		"high_priority": true,
		"response_to_external_id": null,
		"metadata": null
	}`

	msg = DBMsg{}
	err = json.Unmarshal([]byte(msgJSONNoQR), &msg)
	ts.NoError(err)
	ts.Nil(msg.Attachments())
	ts.Nil(msg.QuickReplies())
	ts.Equal("", msg.Topic())
	ts.Equal("", msg.ResponseToExternalID())
	ts.False(msg.IsResend())
	ts.Nil(msg.Flow())
	ts.Equal("", msg.FlowName())
	ts.Equal("", msg.FlowUUID())
}

func (ts *BackendTestSuite) TestCheckMsgExists() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)

	// check with invalid message id
	err := checkMsgExists(ts.b, ts.b.NewMsgStatusForID(knChannel, -1, courier.MsgStatusValue("S"), clog))
	ts.Equal(err, courier.ErrMsgNotFound)

	// check with valid message id
	err = checkMsgExists(ts.b, ts.b.NewMsgStatusForID(knChannel, 10000, courier.MsgStatusValue("S"), clog))
	ts.Nil(err)

	// only outgoing messages are matched
	err = checkMsgExists(ts.b, ts.b.NewMsgStatusForID(knChannel, 10002, courier.MsgStatusValue("S"), clog))
	ts.Equal(err, courier.ErrMsgNotFound)

	// check with invalid external id
	err = checkMsgExists(ts.b, ts.b.NewMsgStatusForExternalID(knChannel, "ext-invalid", courier.MsgStatusValue("S"), clog))
	ts.Equal(err, courier.ErrMsgNotFound)

	// only outgoing messages are matched
	err = checkMsgExists(ts.b, ts.b.NewMsgStatusForExternalID(knChannel, "ext2", courier.MsgStatusValue("S"), clog))
	ts.Equal(err, courier.ErrMsgNotFound)

	// check with valid external id
	status := ts.b.NewMsgStatusForExternalID(knChannel, "ext1", courier.MsgStatusValue("S"), clog)
	err = checkMsgExists(ts.b, status)
	ts.Nil(err)
}

func (ts *BackendTestSuite) TestDeleteMsgWithExternalID() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	ctx := context.Background()

	// no error for invalid external ID
	err := ts.b.DeleteMsgWithExternalID(ctx, knChannel, "ext-invalid")
	ts.Nil(err)

	// cannot change out going messages
	err = ts.b.DeleteMsgWithExternalID(ctx, knChannel, "ext1")
	ts.Nil(err)

	m := readMsgFromDB(ts.b, 10000)
	ts.Equal(m.Text_, "test message")
	ts.Equal(len(m.Attachments()), 0)
	ts.Equal(m.Visibility_, MsgVisibility("V"))

	// for incoming messages mark them deleted by sender and readact their text and clear their attachments
	err = ts.b.DeleteMsgWithExternalID(ctx, knChannel, "ext2")
	ts.Nil(err)

	m = readMsgFromDB(ts.b, 10002)
	ts.Equal(m.Text_, "")
	ts.Equal(len(m.Attachments()), 0)
	ts.Equal(m.Visibility_, MsgVisibility("X"))

}

func (ts *BackendTestSuite) TestContact() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn, _ := urns.NewTelURNForCountry("12065551518", "US")

	ctx := context.Background()
	now := time.Now()

	// create our new contact
	contact, err := contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", "Ryan Lewis", clog)
	ts.NoError(err)

	now2 := time.Now()

	// load this contact again by URN, should be same contact, name unchanged
	contact2, err := contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", "Other Name", clog)
	ts.NoError(err)

	ts.Equal(contact.UUID_, contact2.UUID_)
	ts.Equal(contact.ID_, contact2.ID_)
	ts.Equal(knChannel.OrgID(), contact2.OrgID_)
	ts.Equal(null.String("Ryan Lewis"), contact2.Name_)
	ts.True(contact2.ModifiedOn_.After(now))
	ts.True(contact2.CreatedOn_.After(now))
	ts.True(contact2.ModifiedOn_.Before(now2))
	ts.True(contact2.CreatedOn_.Before(now2))

	// load a contact by URN instead (this one is in our testdata)
	cURN, _ := urns.NewTelURNForCountry("+12067799192", "US")
	contact, err = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, cURN, "", "", clog)
	ts.NoError(err)
	ts.NotNil(contact)

	ts.Equal(null.String(""), contact.Name_)
	ts.Equal(courier.ContactUUID("a984069d-0008-4d8c-a772-b14a8a6acccc"), contact.UUID_)

	urn, _ = urns.NewTelURNForCountry("12065551519", "US")

	// long name are truncated

	longName := "LongRandomNameHPGBRDjZvkz7y58jI2UPkio56IKGaMvaeDTvF74Q5SUkIHozFn1MLELfjX7vRrFto8YG2KPVaWzekgmFbkuxujIotFAgfhHqoHKW5c177FUtKf5YK9KbY8hp0x7PxIFY3MS5lMyMA5ELlqIgikThpr"
	contact3, err := contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", longName, clog)
	ts.NoError(err)

	ts.Equal(null.String(longName[0:127]), contact3.Name_)

}

func (ts *BackendTestSuite) TestContactRace() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn, _ := urns.NewTelURNForCountry("12065551518", "US")

	urnSleep = true
	defer func() { urnSleep = false }()

	ctx := context.Background()

	// create our contact twice
	var contact1, contact2 *DBContact
	var err1, err2 error

	go func() {
		contact1, err1 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", "Ryan Lewis", clog)
	}()
	go func() {
		contact2, err2 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", "Ryan Lewis", clog)
	}()

	time.Sleep(time.Second)

	ts.NoError(err1)
	ts.NoError(err2)
	ts.Equal(contact1.ID_, contact2.ID_)
}

func (ts *BackendTestSuite) TestAddAndRemoveContactURN() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	ctx := context.Background()

	cURN, err := urns.NewTelURNForCountry("+12067799192", "US")
	ts.NoError(err)

	contact, err := contactForURN(ctx, ts.b, knChannel.OrgID_, knChannel, cURN, "", "", clog)
	ts.NoError(err)
	ts.NotNil(contact)

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	contactURNs, err := contactURNsForContact(tx, contact.ID_)
	ts.NoError(err)
	ts.Equal(len(contactURNs), 1)

	urn, _ := urns.NewTelURNForCountry("12065551518", "US")
	addedURN, err := ts.b.AddURNtoContact(ctx, knChannel, contact, urn)
	ts.NoError(err)
	ts.NotNil(addedURN)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	contactURNs, err = contactURNsForContact(tx, contact.ID_)
	ts.NoError(err)
	ts.Equal(len(contactURNs), 2)

	removedURN, err := ts.b.RemoveURNfromContact(ctx, knChannel, contact, urn)
	ts.NoError(err)
	ts.NotNil(removedURN)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)
	contactURNs, err = contactURNsForContact(tx, contact.ID_)
	ts.NoError(err)
	ts.Equal(len(contactURNs), 1)
}

func (ts *BackendTestSuite) TestContactURN() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	twChannel := ts.getChannel("TW", "dbc126ed-66bc-4e28-b67b-81dc3327c96a")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn, _ := urns.NewTelURNForCountry("12065551515", "US")

	ctx := context.Background()

	contact, err := contactForURN(ctx, ts.b, knChannel.OrgID_, knChannel, urn, "", "", clog)
	ts.NoError(err)
	ts.NotNil(contact)

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	contact, err = contactForURN(ctx, ts.b, twChannel.OrgID_, twChannel, urn, "chestnut", "", clog)
	ts.NoError(err)
	ts.NotNil(contact)

	contactURNs, err := contactURNsForContact(tx, contact.ID_)
	ts.NoError(err)
	ts.Equal(null.String("chestnut"), contactURNs[0].Auth)

	// now build a URN for our number with the kannel channel
	knURN, err := contactURNForURN(tx, knChannel, contact.ID_, urn, "sesame")
	ts.NoError(err)
	ts.NoError(tx.Commit())
	ts.Equal(knURN.OrgID, knChannel.OrgID_)
	ts.Equal(null.String("sesame"), knURN.Auth)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	// then with our twilio channel
	twURN, err := contactURNForURN(tx, twChannel, contact.ID_, urn, "")
	ts.NoError(err)
	ts.NoError(tx.Commit())

	// should be the same URN
	ts.Equal(knURN.ID, twURN.ID)

	// same contact
	ts.Equal(knURN.ContactID, twURN.ContactID)

	// and channel should be set to twitter
	ts.Equal(twURN.ChannelID, twChannel.ID())

	// auth should be unchanged
	ts.Equal(null.String("sesame"), twURN.Auth)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	// again with different auth
	twURN, err = contactURNForURN(tx, twChannel, contact.ID_, urn, "peanut")
	ts.NoError(err)
	ts.NoError(tx.Commit())
	ts.Equal(null.String("peanut"), twURN.Auth)

	// test that we don't use display when looking up URNs
	tgChannel := ts.getChannel("TG", "dbc126ed-66bc-4e28-b67b-81dc3327c98a")
	tgURN, _ := urns.NewTelegramURN(12345, "")

	tgContact, err := contactForURN(ctx, ts.b, tgChannel.OrgID_, tgChannel, tgURN, "", "", clog)
	ts.NoError(err)

	tgURNDisplay, _ := urns.NewTelegramURN(12345, "Jane")
	displayContact, err := contactForURN(ctx, ts.b, tgChannel.OrgID_, tgChannel, tgURNDisplay, "", "", clog)

	ts.NoError(err)
	ts.Equal(tgContact.URNID_, displayContact.URNID_)
	ts.Equal(tgContact.ID_, displayContact.ID_)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	tgContactURN, err := contactURNForURN(tx, tgChannel, tgContact.ID_, tgURNDisplay, "")
	ts.NoError(err)
	ts.NoError(tx.Commit())
	ts.Equal(tgContact.URNID_, tgContactURN.ID)
	ts.Equal(null.String("Jane"), tgContactURN.Display)

	// try to create two contacts at the same time in goroutines, this tests our transaction rollbacks
	urn2, _ := urns.NewTelURNForCountry("12065551616", "US")
	var wait sync.WaitGroup
	var contact2, contact3 *DBContact
	wait.Add(2)
	go func() {
		var err2 error
		contact2, err2 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn2, "", "", clog)
		ts.NoError(err2)
		wait.Done()
	}()
	go func() {
		var err3 error
		contact3, err3 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn2, "", "", clog)
		ts.NoError(err3)
		wait.Done()
	}()
	wait.Wait()
	ts.NotNil(contact2)
	ts.NotNil(contact3)
	ts.Equal(contact2.ID_, contact3.ID_)
	ts.Equal(contact2.URNID_, contact3.URNID_)
}

func (ts *BackendTestSuite) TestContactURNPriority() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	twChannel := ts.getChannel("TW", "dbc126ed-66bc-4e28-b67b-81dc3327c96a")
	knURN, _ := urns.NewTelURNForCountry("12065551111", "US")
	twURN, _ := urns.NewTelURNForCountry("12065552222", "US")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)

	ctx := context.Background()

	knContact, err := contactForURN(ctx, ts.b, knChannel.OrgID_, knChannel, knURN, "", "", clog)
	ts.NoError(err)

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	_, err = contactURNForURN(tx, twChannel, knContact.ID_, twURN, "")
	ts.NoError(err)
	ts.NoError(tx.Commit())

	// ok, now looking up our contact should reset our URNs and their affinity..
	// TwitterURN should be first all all URNs should now use Twitter channel
	twContact, err := contactForURN(ctx, ts.b, twChannel.OrgID_, twChannel, twURN, "", "", clog)
	ts.NoError(err)

	ts.Equal(twContact.ID_, knContact.ID_)

	// get all the URNs for this contact
	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	urns, err := contactURNsForContact(tx, twContact.ID_)
	ts.NoError(err)
	ts.NoError(tx.Commit())

	ts.Equal("tel:+12065552222", urns[0].Identity)
	ts.Equal(twChannel.ID(), urns[0].ChannelID)

	ts.Equal("tel:+12065551111", urns[1].Identity)
	ts.Equal(twChannel.ID(), urns[1].ChannelID)
}

func (ts *BackendTestSuite) TestMsgStatus() {
	ctx := context.Background()
	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	now := time.Now().In(time.UTC)

	updateStatusByID := func(id courier.MsgID, status courier.MsgStatusValue, newExtID string) *courier.ChannelLog {
		clog := courier.NewChannelLog(courier.ChannelLogTypeMsgStatus, channel, nil)
		statusObj := ts.b.NewMsgStatusForID(channel, id, status, clog)
		if newExtID != "" {
			statusObj.SetExternalID(newExtID)
		}
		err := ts.b.WriteMsgStatus(ctx, statusObj)
		ts.NoError(err)
		time.Sleep(500 * time.Millisecond) // give committer time to write this
		return clog
	}

	updateStatusByExtID := func(extID string, status courier.MsgStatusValue) *courier.ChannelLog {
		clog := courier.NewChannelLog(courier.ChannelLogTypeMsgStatus, channel, nil)
		statusObj := ts.b.NewMsgStatusForExternalID(channel, extID, status, clog)
		err := ts.b.WriteMsgStatus(ctx, statusObj)
		ts.NoError(err)
		time.Sleep(500 * time.Millisecond) // give committer time to write this
		return clog
	}

	// put test message back into queued state
	ts.b.db.MustExec(`UPDATE msgs_msg SET status = 'Q', sent_on = NULL WHERE id = $1`, 10001)

	// update to WIRED using id and provide new external ID
	clog1 := updateStatusByID(10001, courier.MsgWired, "ext0")

	m := readMsgFromDB(ts.b, 10001)
	ts.Equal(courier.MsgWired, m.Status_)
	ts.Equal(null.String("ext0"), m.ExternalID_)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.After(now))
	ts.Equal(null.NullString, m.FailedReason_)
	ts.Equal(pq.StringArray([]string{string(clog1.UUID())}), m.LogUUIDs)

	sentOn := *m.SentOn_

	// update to SENT using id
	clog2 := updateStatusByID(10001, courier.MsgSent, "")

	m = readMsgFromDB(ts.b, 10001)
	ts.Equal(courier.MsgSent, m.Status_)
	ts.Equal(null.String("ext0"), m.ExternalID_) // no change
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.Equal(sentOn)) // no change
	ts.Equal(pq.StringArray([]string{string(clog1.UUID()), string(clog2.UUID())}), m.LogUUIDs)

	// update to DELIVERED using id
	clog3 := updateStatusByID(10001, courier.MsgDelivered, "")

	m = readMsgFromDB(ts.b, 10001)
	ts.Equal(m.Status_, courier.MsgDelivered)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.Equal(sentOn)) // no change
	ts.Equal(pq.StringArray([]string{string(clog1.UUID()), string(clog2.UUID()), string(clog3.UUID())}), m.LogUUIDs)

	// put test message back into queued state
	ts.b.db.MustExec(`UPDATE msgs_msg SET status = 'Q', sent_on = NULL, external_id = NULL WHERE id = $1`, 10001)

	rc := ts.b.redisPool.Get()
	defer rc.Close()

	key := fmt.Sprintf("msg-external-id|%s|%s", string(m.Channel().UUID()), "ext0")
	rc.Do("SET", key, 10001, "EX", 300)

	clog4 := updateStatusByExtID("ext0", courier.MsgSent)
	m = readMsgFromDB(ts.b, 10001)
	ts.Equal(courier.MsgSent, m.Status_)
	ts.Equal(null.NullString, m.ExternalID_) // no change
	ts.True(m.ModifiedOn_.After(now))
	ts.Equal(pq.StringArray([]string{string(clog1.UUID()), string(clog2.UUID()), string(clog3.UUID()), string(clog4.UUID())}), m.LogUUIDs)

	// no change for incoming messages
	updateStatusByID(10002, courier.MsgSent, "")

	m = readMsgFromDB(ts.b, 10002)
	ts.Equal(courier.MsgPending, m.Status_)
	ts.Equal(m.ExternalID_, null.String("ext2"))
	ts.Equal(pq.StringArray(nil), m.LogUUIDs)

	// update to FAILED using external id
	clog5 := updateStatusByExtID("ext1", courier.MsgFailed)

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(courier.MsgFailed, m.Status_)
	ts.True(m.ModifiedOn_.After(now))
	ts.Nil(m.SentOn_)
	ts.Equal(pq.StringArray([]string{string(clog5.UUID())}), m.LogUUIDs)

	now = time.Now().In(time.UTC)
	time.Sleep(2 * time.Millisecond)

	// update to WIRED using external id
	clog6 := updateStatusByExtID("ext1", courier.MsgWired)

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(courier.MsgWired, m.Status_)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.After(now))

	sentOn = *m.SentOn_

	// update to SENT using external id
	updateStatusByExtID("ext1", courier.MsgSent)

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(courier.MsgSent, m.Status_)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.Equal(sentOn)) // no change

	// put test outgoing messages back into queued state
	ts.b.db.MustExec(`UPDATE msgs_msg SET status = 'Q', sent_on = NULL WHERE id IN ($1, $2)`, 10002, 10001)

	// can skip WIRED and go straight to SENT or DELIVERED
	updateStatusByExtID("ext1", courier.MsgSent)
	updateStatusByID(10001, courier.MsgDelivered, "")

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(courier.MsgSent, m.Status_)
	ts.NotNil(m.SentOn_)
	m = readMsgFromDB(ts.b, 10001)
	ts.Equal(courier.MsgDelivered, m.Status_)
	ts.NotNil(m.SentOn_)

	// no such external id for outgoing message
	status := ts.b.NewMsgStatusForExternalID(channel, "ext2", courier.MsgSent, clog6)
	err := ts.b.WriteMsgStatus(ctx, status)
	ts.Error(err)

	// no such external id
	status = ts.b.NewMsgStatusForExternalID(channel, "ext3", courier.MsgSent, clog6)
	err = ts.b.WriteMsgStatus(ctx, status)
	ts.Error(err)

	// reset our status to sent
	status = ts.b.NewMsgStatusForExternalID(channel, "ext1", courier.MsgSent, clog6)
	err = ts.b.WriteMsgStatus(ctx, status)
	ts.NoError(err)
	time.Sleep(time.Second)

	// error our msg
	now = time.Now().In(time.UTC)
	time.Sleep(2 * time.Millisecond)
	status = ts.b.NewMsgStatusForExternalID(channel, "ext1", courier.MsgErrored, clog6)
	err = ts.b.WriteMsgStatus(ctx, status)
	ts.NoError(err)

	time.Sleep(time.Second) // give committer time to write this

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(m.Status_, courier.MsgErrored)
	ts.Equal(m.ErrorCount_, 1)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.NextAttempt_.After(now))
	ts.Equal(null.NullString, m.FailedReason_)

	// second go
	status = ts.b.NewMsgStatusForExternalID(channel, "ext1", courier.MsgErrored, clog6)
	err = ts.b.WriteMsgStatus(ctx, status)
	ts.NoError(err)

	time.Sleep(time.Second) // give committer time to write this

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(m.Status_, courier.MsgErrored)
	ts.Equal(m.ErrorCount_, 2)
	ts.Equal(null.NullString, m.FailedReason_)

	// third go
	status = ts.b.NewMsgStatusForExternalID(channel, "ext1", courier.MsgErrored, clog6)
	err = ts.b.WriteMsgStatus(ctx, status)

	time.Sleep(time.Second) // give committer time to write this

	ts.NoError(err)
	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(m.Status_, courier.MsgFailed)
	ts.Equal(m.ErrorCount_, 3)
	ts.Equal(null.String("E"), m.FailedReason_)

	// update URN when the new doesn't exist
	tx, _ := ts.b.db.BeginTxx(ctx, nil)
	oldURN, _ := urns.NewWhatsAppURN("55988776655")
	_ = insertContactURN(tx, newDBContactURN(channel.OrgID_, channel.ID_, NilContactID, oldURN, ""))

	ts.NoError(tx.Commit())

	newURN, _ := urns.NewWhatsAppURN("5588776655")
	status = ts.b.NewMsgStatusForID(channel, courier.MsgID(10000), courier.MsgSent, clog6)
	status.SetUpdatedURN(oldURN, newURN)

	ts.NoError(ts.b.WriteMsgStatus(ctx, status))

	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	contactURN, err := selectContactURN(tx, channel.OrgID_, newURN)

	ts.NoError(err)
	ts.Equal(contactURN.Identity, newURN.Identity().String())
	ts.NoError(tx.Commit())

	// new URN already exits but don't have an associated contact
	oldURN, _ = urns.NewWhatsAppURN("55999887766")
	newURN, _ = urns.NewWhatsAppURN("5599887766")
	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	contact, _ := contactForURN(ctx, ts.b, channel.OrgID_, channel, oldURN, "", "", clog6)
	_ = insertContactURN(tx, newDBContactURN(channel.OrgID_, channel.ID_, NilContactID, newURN, ""))

	ts.NoError(tx.Commit())

	status = ts.b.NewMsgStatusForID(channel, courier.MsgID(10007), courier.MsgSent, clog6)
	status.SetUpdatedURN(oldURN, newURN)

	ts.NoError(ts.b.WriteMsgStatus(ctx, status))

	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	newContactURN, _ := selectContactURN(tx, channel.OrgID_, newURN)
	oldContactURN, _ := selectContactURN(tx, channel.OrgID_, oldURN)

	ts.Equal(newContactURN.ContactID, contact.ID_)
	ts.Equal(oldContactURN.ContactID, NilContactID)
	ts.NoError(tx.Commit())

	// new URN already exits and have an associated contact
	oldURN, _ = urns.NewWhatsAppURN("55988776655")
	newURN, _ = urns.NewWhatsAppURN("5588776655")
	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	_, _ = contactForURN(ctx, ts.b, channel.OrgID_, channel, oldURN, "", "", clog6)
	otherContact, _ := contactForURN(ctx, ts.b, channel.OrgID_, channel, newURN, "", "", clog6)

	ts.NoError(tx.Commit())

	status = ts.b.NewMsgStatusForID(channel, courier.MsgID(10007), courier.MsgSent, clog6)
	status.SetUpdatedURN(oldURN, newURN)

	ts.NoError(ts.b.WriteMsgStatus(ctx, status))

	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	oldContactURN, _ = selectContactURN(tx, channel.OrgID_, oldURN)
	newContactURN, _ = selectContactURN(tx, channel.OrgID_, newURN)

	ts.Equal(oldContactURN.ContactID, NilContactID)
	ts.Equal(newContactURN.ContactID, otherContact.ID_)
	ts.NoError(tx.Commit())
}

func (ts *BackendTestSuite) TestHealth() {
	// all should be well in test land
	ts.Equal(ts.b.Health(), "")
}

func (ts *BackendTestSuite) TestHeartbeat() {
	// TODO make analytics abstraction layer so we can test what we report
	ts.NoError(ts.b.Heartbeat())
}

func (ts *BackendTestSuite) TestDupes() {
	r := ts.b.redisPool.Get()
	defer r.Close()

	ctx := context.Background()
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn, _ := urns.NewTelURNForCountry("12065551215", knChannel.Country())

	msg := ts.b.NewIncomingMsg(knChannel, urn, "ping", clog).(*DBMsg)
	err := ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)

	// grab our UUID
	uuid1 := msg.UUID()

	// trying again should lead to same UUID
	msg = ts.b.NewIncomingMsg(knChannel, urn, "ping", clog).(*DBMsg)
	err = ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)

	ts.Equal(uuid1, msg.UUID())

	// different message should change that
	msg = ts.b.NewIncomingMsg(knChannel, urn, "test", clog).(*DBMsg)
	err = ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)

	ts.NotEqual(uuid1, msg.UUID())
	uuid2 := msg.UUID()

	// an outgoing message should clear things
	dbMsg := readMsgFromDB(ts.b, 10000)
	dbMsg.URN_ = urn
	dbMsg.channel = knChannel
	dbMsg.ChannelUUID_ = knChannel.UUID()
	dbMsg.Text_ = "test"

	msgJSON, err := json.Marshal([]interface{}{dbMsg})
	ts.NoError(err)

	err = queue.PushOntoQueue(r, msgQueueName, "dbc126ed-66bc-4e28-b67b-81dc3327c95d", 10, string(msgJSON), queue.HighPriority)
	ts.NoError(err)

	_, err = ts.b.PopNextOutgoingMsg(ctx)
	ts.NoError(err)

	msg = ts.b.NewIncomingMsg(knChannel, urn, "test", clog).(*DBMsg)
	err = ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)

	ts.NotEqual(uuid2, msg.UUID())
}

func (ts *BackendTestSuite) TestExternalIDDupes() {
	r := ts.b.redisPool.Get()
	defer r.Close()

	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn, _ := urns.NewTelURNForCountry("12065551215", knChannel.Country())

	msg := newMsg(MsgIncoming, knChannel, urn, "ping", clog)

	var checkedMsg = ts.b.CheckExternalIDSeen(msg)
	m := checkedMsg.(*DBMsg)
	ts.False(m.alreadyWritten)

	ts.b.WriteExternalIDSeen(msg)

	checkedMsg = ts.b.CheckExternalIDSeen(msg)
	m2 := checkedMsg.(*DBMsg)
	ts.True(m2.alreadyWritten)
}

func (ts *BackendTestSuite) TestStatus() {
	// our health should just contain the header
	ts.True(strings.Contains(ts.b.Status(), "Channel"), ts.b.Status())

	// add a message to our queue
	r := ts.b.redisPool.Get()
	defer r.Close()

	dbMsg := readMsgFromDB(ts.b, 10000)
	dbMsg.ChannelUUID_ = courier.ChannelUUID("dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	ts.NotNil(dbMsg)

	// serialize our message
	msgJSON, err := json.Marshal([]interface{}{dbMsg})
	ts.NoError(err)

	err = queue.PushOntoQueue(r, msgQueueName, "dbc126ed-66bc-4e28-b67b-81dc3327c95d", 10, string(msgJSON), queue.HighPriority)
	ts.NoError(err)

	// status should now contain that channel
	ts.True(strings.Contains(ts.b.Status(), "1           0         0    10     KN   dbc126ed-66bc-4e28-b67b-81dc3327c95d"), ts.b.Status())
}

func (ts *BackendTestSuite) TestOutgoingQueue() {
	// add one of our outgoing messages to the queue
	ctx := context.Background()
	r := ts.b.redisPool.Get()
	defer r.Close()

	dbMsg := readMsgFromDB(ts.b, 10000)
	dbMsg.ChannelUUID_ = courier.ChannelUUID("dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	ts.NotNil(dbMsg)

	// serialize our message
	msgJSON, err := json.Marshal([]interface{}{dbMsg})
	ts.NoError(err)

	err = queue.PushOntoQueue(r, msgQueueName, "dbc126ed-66bc-4e28-b67b-81dc3327c95d", 10, string(msgJSON), queue.HighPriority)
	ts.NoError(err)

	// pop a message off our queue
	msg, err := ts.b.PopNextOutgoingMsg(ctx)
	ts.NoError(err)
	ts.NotNil(msg)

	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, msg.Channel(), nil)

	// make sure it is the message we just added
	ts.Equal(dbMsg.ID(), msg.ID())

	// and that it has the appropriate text
	ts.Equal(msg.Text(), "test message")

	// mark this message as dealt with
	ts.b.MarkOutgoingMsgComplete(ctx, msg, ts.b.NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgWired, clog))

	// this message should now be marked as sent
	sent, err := ts.b.WasMsgSent(ctx, msg.ID())
	ts.NoError(err)
	ts.True(sent)

	// pop another message off, shouldn't get anything
	msg2, err := ts.b.PopNextOutgoingMsg(ctx)
	ts.Nil(msg2)
	ts.Nil(err)

	// checking another message should show unsent
	msg3 := readMsgFromDB(ts.b, 10001)
	sent, err = ts.b.WasMsgSent(ctx, msg3.ID())
	ts.NoError(err)
	ts.False(sent)

	// write an error for our original message
	err = ts.b.WriteMsgStatus(ctx, ts.b.NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored, clog))
	ts.NoError(err)

	// message should no longer be considered sent
	sent, err = ts.b.WasMsgSent(ctx, msg.ID())
	ts.NoError(err)
	ts.False(sent)
}

func (ts *BackendTestSuite) TestChannel() {
	noAddress := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c99a")
	ts.Equal("US", noAddress.Country())
	ts.Equal(courier.NilChannelAddress, noAddress.ChannelAddress())

	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	ts.Equal("2500", knChannel.Address())
	ts.Equal(courier.ChannelAddress("2500"), knChannel.ChannelAddress())
	ts.Equal("RW", knChannel.Country())
	ts.Equal([]courier.ChannelRole{courier.ChannelRoleSend, courier.ChannelRoleReceive}, knChannel.Roles())
	ts.True(knChannel.HasRole(courier.ChannelRoleSend))
	ts.True(knChannel.HasRole(courier.ChannelRoleReceive))
	ts.False(knChannel.HasRole(courier.ChannelRoleCall))
	ts.False(knChannel.HasRole(courier.ChannelRoleAnswer))

	// assert our config values
	val := knChannel.ConfigForKey("use_national", false)
	boolVal, isBool := val.(bool)
	ts.True(isBool)
	ts.True(boolVal)

	val = knChannel.ConfigForKey("encoding", "default")
	ts.Equal("smart", val)

	val = knChannel.StringConfigForKey("encoding", "default")
	ts.Equal("smart", val)

	val = knChannel.StringConfigForKey("encoding_missing", "default")
	ts.Equal("default", val)

	val = knChannel.IntConfigForKey("max_length_int", -1)
	ts.Equal(320, val)

	val = knChannel.IntConfigForKey("max_length_str", -1)
	ts.Equal(320, val)

	val = knChannel.IntConfigForKey("max_length_missing", -1)
	ts.Equal(-1, val)

	// missing value
	val = knChannel.ConfigForKey("missing", "missingValue")
	ts.Equal("missingValue", val)

	// try an org config
	val = knChannel.OrgConfigForKey("CHATBASE_API_KEY", nil)
	ts.Equal("cak", val)

	// and a missing value
	val = knChannel.OrgConfigForKey("missing", "missingValue")
	ts.Equal("missingValue", val)

	exChannel := ts.getChannel("EX", "dbc126ed-66bc-4e28-b67b-81dc3327100a")
	ts.Equal([]courier.ChannelRole{courier.ChannelRoleReceive}, exChannel.Roles())
	ts.False(exChannel.HasRole(courier.ChannelRoleSend))
	ts.True(exChannel.HasRole(courier.ChannelRoleReceive))
	ts.False(exChannel.HasRole(courier.ChannelRoleCall))
	ts.False(exChannel.HasRole(courier.ChannelRoleAnswer))

	exChannel2 := ts.getChannel("EX", "dbc126ed-66bc-4e28-b67b-81dc3327222a")
	ts.False(exChannel2.HasRole(courier.ChannelRoleSend))
	ts.False(exChannel2.HasRole(courier.ChannelRoleReceive))
	ts.False(exChannel2.HasRole(courier.ChannelRoleCall))
	ts.False(exChannel2.HasRole(courier.ChannelRoleAnswer))
}

func (ts *BackendTestSuite) TestGetChannel() {
	ctx := context.Background()

	knUUID := courier.ChannelUUID("dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	xxUUID := courier.ChannelUUID("0a1256fe-c6e4-494d-99d3-576286f31d3b") // doesn't exist

	ch, err := ts.b.GetChannel(ctx, courier.ChannelType("KN"), knUUID)
	ts.Assert().NoError(err)
	ts.Assert().NotNil(ch)
	ts.Assert().Equal(knUUID, ch.UUID())

	ch, err = ts.b.GetChannel(ctx, courier.ChannelType("KN"), knUUID) // from cache
	ts.Assert().NoError(err)
	ts.Assert().NotNil(ch)
	ts.Assert().Equal(knUUID, ch.UUID())

	ch, err = ts.b.GetChannel(ctx, courier.ChannelType("KN"), xxUUID)
	ts.Assert().Error(err)
	ts.Assert().Nil(ch)
	ts.Assert().True(ch == nil) // https://github.com/stretchr/testify/issues/503

	ch, err = ts.b.GetChannel(ctx, courier.ChannelType("KN"), xxUUID) // from cache
	ts.Assert().Error(err)
	ts.Assert().Nil(ch)
	ts.Assert().True(ch == nil) // https://github.com/stretchr/testify/issues/503
}

func (ts *BackendTestSuite) TestWriteChanneLog() {
	ctx := context.Background()
	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	defer func() {
		ts.b.db.MustExecContext(ctx, "DELETE FROM channels_channellog")
	}()

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]*httpx.MockResponse{
		"https://api.messages.com/send.json": {
			httpx.NewMockResponse(200, nil, []byte(`{"status":"success"}`)),
		},
	}))
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	// make a request that will have a response
	req, _ := http.NewRequest("POST", "https://api.messages.com/send.json", nil)
	trace, err := httpx.DoTrace(http.DefaultClient, req, nil, nil, 0)
	ts.NoError(err)

	clog1 := courier.NewChannelLog(courier.ChannelLogTypeTokenRefresh, channel, nil)
	clog1.HTTP(trace)
	clog1.Error(courier.ErrorResponseStatusCode())

	// log isn't attached to a message so will be written to the database
	err = ts.b.WriteChannelLog(ctx, clog1)
	ts.NoError(err)

	time.Sleep(time.Second) // give writer time to write this

	assertdb.Query(ts.T(), ts.b.db, `SELECT count(*) FROM channels_channellog`).Returns(1)
	assertdb.Query(ts.T(), ts.b.db, `SELECT channel_id, http_logs->0->>'url' AS url, errors->0->>'message' AS err FROM channels_channellog`).
		Columns(map[string]interface{}{"channel_id": int64(channel.ID()), "url": "https://api.messages.com/send.json", "err": "Unexpected response status code."})

	clog2 := courier.NewChannelLog(courier.ChannelLogTypeMsgSend, channel, nil)
	clog2.HTTP(trace)
	clog2.SetMsgID(1234)

	// log is attached to a message so will be written to storage
	err = ts.b.WriteChannelLog(ctx, clog2)
	ts.NoError(err)

	time.Sleep(time.Second) // give writer time to write this

	_, body, err := ts.b.logStorage.Get(context.Background(), fmt.Sprintf("channels/%s/%s/%s.json", channel.UUID(), clog2.UUID()[0:4], clog2.UUID()))
	ts.NoError(err)
	ts.Contains(string(body), "msg_send")
	ts.Contains(string(body), "https://api.messages.com/send.json")

	ts.b.db.MustExec(`DELETE FROM channels_channellog`)

	// channel channel log policy to only write errors
	channel.LogPolicy = LogPolicyErrors

	clog3 := courier.NewChannelLog(courier.ChannelLogTypeMsgSend, channel, nil)
	clog3.HTTP(trace)
	ts.NoError(ts.b.WriteChannelLog(ctx, clog3))

	time.Sleep(time.Second) // give writer time to.. not write this

	assertdb.Query(ts.T(), ts.b.db, `SELECT count(*) FROM channels_channellog`).Returns(0)

	clog4 := courier.NewChannelLog(courier.ChannelLogTypeMsgSend, channel, nil)
	clog4.HTTP(trace)
	clog4.Error(courier.ErrorResponseStatusCode())
	ts.NoError(ts.b.WriteChannelLog(ctx, clog4))

	time.Sleep(time.Second) // give writer time to write this because it's an error

	assertdb.Query(ts.T(), ts.b.db, `SELECT count(*) FROM channels_channellog`).Returns(1)

	// channel channel log policy to discard all
	channel.LogPolicy = LogPolicyNone

	clog5 := courier.NewChannelLog(courier.ChannelLogTypeMsgSend, channel, nil)
	clog5.HTTP(trace)
	clog5.Error(courier.ErrorResponseStatusCode())
	ts.NoError(ts.b.WriteChannelLog(ctx, clog5))

	time.Sleep(time.Second) // give writer time to.. not write this

	assertdb.Query(ts.T(), ts.b.db, `SELECT count(*) FROM channels_channellog`).Returns(1)
}

func (ts *BackendTestSuite) TestSaveAttachment() {
	testJPG := test.ReadFile("../../test/testdata/test.jpg")
	ctx := context.Background()

	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234))

	newURL, err := ts.b.SaveAttachment(ctx, knChannel, "image/jpeg", testJPG, "jpg")
	ts.NoError(err)
	ts.Equal("_test_storage/attachments/media/1/c00e/5d67/c00e5d67-c275-4389-aded-7d8b151cbd5b.jpg", newURL)
}

func (ts *BackendTestSuite) TestWriteMsg() {
	ctx := context.Background()
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)

	// have to round to microseconds because postgres can't store nanos
	now := time.Now().Round(time.Microsecond).In(time.UTC)

	// create a new courier msg
	urn, _ := urns.NewTelURNForCountry("12065551212", knChannel.Country())
	msg := ts.b.NewIncomingMsg(knChannel, urn, "test123", clog).WithExternalID("ext123").WithReceivedOn(now).WithContactName("test contact").(*DBMsg)

	// try to write it to our db
	err := ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)

	// creating the incoming msg again should give us the same UUID and have the msg set as not to write
	time.Sleep(1 * time.Second)
	msg2 := ts.b.NewIncomingMsg(knChannel, urn, "test123", clog).(*DBMsg)
	ts.Equal(msg2.UUID(), msg.UUID())
	ts.True(msg2.alreadyWritten)

	// check we had an id set
	ts.NotZero(msg.ID)

	// load it back from the id
	m := readMsgFromDB(ts.b, msg.ID())

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	// load our URN
	contactURN, err := contactURNForURN(tx, m.channel, m.ContactID_, urn, "")
	if !ts.NoError(err) || !ts.NoError(tx.Commit()) {
		ts.FailNow("failed writing contact urn")
	}

	// make sure our values are set appropriately
	ts.Equal(msg.ID(), m.ID())
	ts.Equal(knChannel.ID_, m.ChannelID_)
	ts.Equal(knChannel.OrgID_, m.OrgID_)
	ts.Equal(contactURN.ContactID, m.ContactID_)
	ts.Equal(contactURN.ID, m.ContactURNID_)
	ts.Equal(MsgIncoming, m.Direction_)
	ts.Equal(courier.MsgPending, m.Status_)
	ts.False(m.HighPriority_)
	ts.Equal("ext123", m.ExternalID())
	ts.Equal("test123", m.Text_)
	ts.Equal(0, len(m.Attachments()))
	ts.Equal(1, m.MessageCount_)
	ts.Equal(0, m.ErrorCount_)
	ts.Equal(now, m.SentOn_.In(time.UTC))
	ts.NotNil(m.NextAttempt_)
	ts.NotNil(m.CreatedOn_)
	ts.NotNil(m.ModifiedOn_)
	ts.NotNil(m.QueuedOn_)

	contact, err := contactForURN(ctx, ts.b, m.OrgID_, knChannel, urn, "", "", clog)
	ts.NoError(err)
	ts.Equal(null.String("test contact"), contact.Name_)
	ts.Equal(m.OrgID_, contact.OrgID_)
	ts.Equal(m.ContactID_, contact.ID_)
	ts.NotNil(contact.UUID_)
	ts.NotNil(contact.ID_)

	// waiting 5 seconds should let us write it successfully
	time.Sleep(5 * time.Second)
	msg3 := ts.b.NewIncomingMsg(knChannel, urn, "test123", clog).(*DBMsg)
	ts.NotEqual(msg3.UUID(), msg.UUID())

	// msg with null bytes in it, that's fine for a request body
	msg = ts.b.NewIncomingMsg(knChannel, urn, "test456\x00456", clog).WithExternalID("ext456").(*DBMsg)
	err = writeMsgToDB(ctx, ts.b, msg, clog)
	ts.NoError(err)

	// more null bytes
	text, _ := url.PathUnescape("%1C%00%00%00%00%00%07%E0%00")
	msg = ts.b.NewIncomingMsg(knChannel, urn, text, clog).(*DBMsg)
	err = writeMsgToDB(ctx, ts.b, msg, clog)
	ts.NoError(err)

	// check that our mailroom queue has an item
	rc := ts.b.redisPool.Get()
	defer rc.Close()
	rc.Do("DEL", "handler:1", "handler:active", fmt.Sprintf("c:1:%d", msg.ContactID_))

	msg = ts.b.NewIncomingMsg(knChannel, urn, "hello 1 2 3", clog).(*DBMsg)
	err = writeMsgToDB(ctx, ts.b, msg, clog)
	ts.NoError(err)

	count, err := redis.Int(rc.Do("ZCARD", "handler:1"))
	ts.NoError(err)
	ts.Equal(1, count)

	count, err = redis.Int(rc.Do("ZCARD", "handler:active"))
	ts.NoError(err)
	ts.Equal(1, count)

	count, err = redis.Int(rc.Do("LLEN", fmt.Sprintf("c:1:%d", msg.ContactID_)))
	ts.NoError(err)
	ts.Equal(1, count)

	data, err := redis.Bytes(rc.Do("LPOP", fmt.Sprintf("c:1:%d", contact.ID_)))
	ts.NoError(err)

	var body map[string]interface{}
	err = json.Unmarshal(data, &body)
	ts.NoError(err)
	ts.Equal("msg_event", body["type"])
	ts.Equal(map[string]interface{}{
		"contact_id":      float64(contact.ID_),
		"org_id":          float64(1),
		"channel_id":      float64(10),
		"msg_id":          float64(msg.ID_),
		"msg_uuid":        string(msg.UUID()),
		"msg_external_id": msg.ExternalID(),
		"urn":             msg.URN().String(),
		"urn_id":          float64(msg.ContactURNID_),
		"text":            msg.Text(),
		"attachments":     nil,
		"new_contact":     contact.IsNew_,
	}, body["task"])
}

func (ts *BackendTestSuite) TestWriteMsgWithAttachments() {
	ctx := context.Background()

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234))

	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn, _ := urns.NewTelURNForCountry("12065551218", knChannel.Country())

	msg := ts.b.NewIncomingMsg(knChannel, urn, "two regular attachments", clog).(*DBMsg)
	msg.WithAttachment("http://example.com/test.jpg")
	msg.WithAttachment("http://example.com/test.m4a")

	// should just write attachments as they are
	err := ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)
	ts.Equal([]string{"http://example.com/test.jpg", "http://example.com/test.m4a"}, msg.Attachments())

	// try an embedded attachment
	msg = ts.b.NewIncomingMsg(knChannel, urn, "embedded attachment data", clog).(*DBMsg)
	msg.WithAttachment(fmt.Sprintf("data:%s", base64.StdEncoding.EncodeToString(test.ReadFile("../../test/testdata/test.jpg"))))

	// should have actually fetched and saved it to storage, with the correct content type
	err = ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)
	ts.Equal([]string{"image/jpeg:_test_storage/attachments/media/1/9b95/5e36/9b955e36-ac16-4c6b-8ab6-9b9af5cd042a.jpg"}, msg.Attachments())

	// try an invalid embedded attachment
	msg = ts.b.NewIncomingMsg(knChannel, urn, "invalid embedded attachment data", clog).(*DBMsg)
	msg.WithAttachment("data:34564363576573573")

	err = ts.b.WriteMsg(ctx, msg, clog)
	ts.EqualError(err, "unable to decode attachment data: illegal base64 data at input byte 16")

	// try a geo attachment
	msg = ts.b.NewIncomingMsg(knChannel, urn, "geo attachment", clog).(*DBMsg)
	msg.WithAttachment("geo:123.234,-45.676")

	// should be saved as is
	err = ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)
	ts.Equal([]string{"geo:123.234,-45.676"}, msg.Attachments())
}

func (ts *BackendTestSuite) TestPreferredChannelCheckRole() {
	exChannel := ts.getChannel("EX", "dbc126ed-66bc-4e28-b67b-81dc3327100a")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, exChannel, nil)
	ctx := context.Background()

	// have to round to microseconds because postgres can't store nanos
	now := time.Now().Round(time.Microsecond).In(time.UTC)

	urn, _ := urns.NewTelURNForCountry("12065552020", exChannel.Country())
	msg := ts.b.NewIncomingMsg(exChannel, urn, "test123", clog).WithExternalID("ext123").WithReceivedOn(now).WithContactName("test contact").(*DBMsg)

	// try to write it to our db
	err := ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)

	time.Sleep(1 * time.Second)

	// load it back from the id
	m := readMsgFromDB(ts.b, msg.ID())

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	// load our URN
	exContactURN, err := contactURNForURN(tx, m.channel, m.ContactID_, urn, "")
	if !ts.NoError(err) || !ts.NoError(tx.Commit()) {
		ts.FailNow("failed writing contact urn")
	}

	ts.Equal(exContactURN.ChannelID, courier.NilChannelID)
}

func (ts *BackendTestSuite) TestChannelEvent() {
	ctx := context.Background()
	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, channel, nil)
	urn, _ := urns.NewTelURNForCountry("12065551616", channel.Country())

	event := ts.b.NewChannelEvent(channel, courier.Referral, urn, clog).WithExtra(map[string]interface{}{"ref_id": "12345"}).WithContactName("kermit frog")
	err := ts.b.WriteChannelEvent(ctx, event, clog)
	ts.NoError(err)

	contact, err := contactForURN(ctx, ts.b, channel.OrgID_, channel, urn, "", "", clog)
	ts.NoError(err)
	ts.Equal(null.String("kermit frog"), contact.Name_)

	dbE := event.(*DBChannelEvent)
	dbE, err = readChannelEventFromDB(ts.b, dbE.ID_)
	ts.NoError(err)
	ts.Equal(dbE.EventType_, courier.Referral)
	ts.Equal(map[string]interface{}{"ref_id": "12345"}, dbE.Extra())
	ts.Equal(contact.ID_, dbE.ContactID_)
	ts.Equal(contact.URNID_, dbE.ContactURNID_)
}

func (ts *BackendTestSuite) TestSessionTimeout() {
	ctx := context.Background()

	// parse from an iso date
	t, err := time.Parse("2006-01-02 15:04:05.000000-07", "2018-12-04 11:52:20.958955-08")
	ts.NoError(err)

	err = updateSessionTimeout(ctx, ts.b, SessionID(1), t, 300)
	ts.NoError(err)

	// make sure that took
	assertdb.Query(ts.T(), ts.b.db, `SELECT count(*) from flows_flowsession WHERE timeout_on > NOW()`).Returns(1)
}

func (ts *BackendTestSuite) TestMailroomEvents() {
	ctx := context.Background()

	rc := ts.b.redisPool.Get()
	defer rc.Close()
	rc.Do("FLUSHDB")

	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, channel, nil)
	urn, _ := urns.NewTelURNForCountry("12065551616", channel.Country())

	event := ts.b.NewChannelEvent(channel, courier.Referral, urn, clog).WithExtra(map[string]interface{}{"ref_id": "12345"}).
		WithContactName("kermit frog").
		WithOccurredOn(time.Date(2020, 8, 5, 13, 30, 0, 123456789, time.UTC))
	err := ts.b.WriteChannelEvent(ctx, event, clog)
	ts.NoError(err)

	contact, err := contactForURN(ctx, ts.b, channel.OrgID_, channel, urn, "", "", clog)
	ts.NoError(err)
	ts.Equal(null.String("kermit frog"), contact.Name_)

	dbE := event.(*DBChannelEvent)
	dbE, err = readChannelEventFromDB(ts.b, dbE.ID_)
	ts.NoError(err)
	ts.Equal(dbE.EventType_, courier.Referral)
	ts.Equal(map[string]interface{}{"ref_id": "12345"}, dbE.Extra())
	ts.Equal(contact.ID_, dbE.ContactID_)
	ts.Equal(contact.URNID_, dbE.ContactURNID_)

	count, err := redis.Int(rc.Do("ZCARD", "handler:1"))
	ts.NoError(err)
	ts.Equal(1, count)

	count, err = redis.Int(rc.Do("ZCARD", "handler:active"))
	ts.NoError(err)
	ts.Equal(1, count)

	count, err = redis.Int(rc.Do("LLEN", fmt.Sprintf("c:1:%d", contact.ID_)))
	ts.NoError(err)
	ts.Equal(1, count)

	data, err := redis.Bytes(rc.Do("LPOP", fmt.Sprintf("c:1:%d", contact.ID_)))
	ts.NoError(err)

	var body map[string]interface{}
	err = json.Unmarshal(data, &body)
	ts.NoError(err)
	ts.Equal("referral", body["type"])
	ts.Equal(map[string]interface{}{
		"channel_id":  float64(10),
		"contact_id":  float64(contact.ID_),
		"extra":       map[string]interface{}{"ref_id": "12345"},
		"new_contact": contact.IsNew_,
		"occurred_on": "2020-08-05T13:30:00.123456789Z",
		"org_id":      float64(1),
		"urn_id":      float64(contact.URNID_),
	}, body["task"])
}

func (ts *BackendTestSuite) TestResolveMedia() {
	ctx := context.Background()

	tcs := []struct {
		url   string
		media courier.Media
		err   string
	}{
		{ // image upload that can be resolved
			url: "http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
			media: &DBMedia{
				UUID_:        "ec6972be-809c-4c8d-be59-ba9dbd74c977",
				Path_:        "/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				ContentType_: "image/jpeg",
				URL_:         "http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				Size_:        123,
				Width_:       1024,
				Height_:      768,
				Alternates_:  []*DBMedia{},
			},
		},
		{ // same image upload, this time from cache
			url: "http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
			media: &DBMedia{
				UUID_:        "ec6972be-809c-4c8d-be59-ba9dbd74c977",
				Path_:        "/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				ContentType_: "image/jpeg",
				URL_:         "http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				Size_:        123,
				Width_:       1024,
				Height_:      768,
				Alternates_:  []*DBMedia{},
			},
		},
		{ // image upload that can't be resolved
			url:   "http://nyaruka.s3.com/orgs/1/media/9790/97904d00-1e64-4f92-b4a0-156e21239d24/test.jpg",
			media: nil,
		},
		{ // image upload that can't be resolved, this time from cache
			url:   "http://nyaruka.s3.com/orgs/1/media/9790/97904d00-1e64-4f92-b4a0-156e21239d24/test.jpg",
			media: nil,
		},
		{ // image upload but with wrong domain
			url:   "http://temba.s2.com/orgs/1/media/f328/f32801ec-433a-4862-978d-56c1823b92b2/test.jpg",
			media: nil,
		},
		{ // image upload but no UUID in URL
			url:   "http://nyaruka.s3.com/orgs/1/media/test.jpg",
			media: nil,
		},
		{ // audio upload
			url: "http://nyaruka.s3.com/orgs/1/media/5310/5310f50f-9c8e-4035-9150-be5a1f78f21a/test.mp3",
			media: &DBMedia{
				UUID_:        "5310f50f-9c8e-4035-9150-be5a1f78f21a",
				Path_:        "/orgs/1/media/5310/5310f50f-9c8e-4035-9150-be5a1f78f21a/test.mp3",
				ContentType_: "audio/mp3",
				URL_:         "http://nyaruka.s3.com/orgs/1/media/5310/5310f50f-9c8e-4035-9150-be5a1f78f21a/test.mp3",
				Size_:        123,
				Duration_:    500,
				Alternates_: []*DBMedia{
					{
						UUID_:        "514c552c-e585-40e2-938a-fe9450172da8",
						Path_:        "/orgs/1/media/514c/514c552c-e585-40e2-938a-fe9450172da8/test.m4a",
						ContentType_: "audio/mp4",
						URL_:         "http://nyaruka.s3.com/orgs/1/media/514c/514c552c-e585-40e2-938a-fe9450172da8/test.m4a",
						Size_:        114,
						Duration_:    500,
					},
				},
			},
		},
		{ // user entered unparseable URL
			url: ":xx",
			err: "error parsing media URL: parse \":xx\": missing protocol scheme",
		},
	}

	for _, tc := range tcs {
		media, err := ts.b.ResolveMedia(ctx, tc.url)
		if tc.err != "" {
			ts.EqualError(err, tc.err)
		} else {
			ts.NoError(err, "unexpected error for url '%s'", tc.url)
			ts.Equal(tc.media, media, "media mismatch for url '%s'", tc.url)
		}
	}

	// check we've cached 3 media lookups
	assertredis.HLen(ts.T(), ts.b.redisPool, fmt.Sprintf("media-lookups:%s", time.Now().In(time.UTC).Format("2006-01-02")), 3)
}

func TestMsgSuite(t *testing.T) {
	suite.Run(t, new(BackendTestSuite))
}

var invalidConfigTestCases = []struct {
	config        courier.Config
	expectedError string
}{
	{config: courier.Config{DB: ":foo"}, expectedError: "unable to parse DB URL"},
	{config: courier.Config{DB: "mysql:test"}, expectedError: "only postgres is supported"},
	{config: courier.Config{DB: "postgres://courier:courier@localhost:5432/courier", Redis: ":foo"}, expectedError: "unable to parse Redis URL"},
}

func (ts *ServerTestSuite) TestInvalidConfigs() {
	for _, testCase := range invalidConfigTestCases {
		config := &testCase.config
		config.Backend = "rapidpro"
		backend := newBackend(config)
		err := backend.Start()
		if ts.Error(err) {
			ts.Contains(err.Error(), testCase.expectedError)
		}
	}
}

func TestBackendSuite(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}

type ServerTestSuite struct {
	suite.Suite
}

// for testing only, returned DBMsg object is not fully populated
func readMsgFromDB(b *backend, id courier.MsgID) *DBMsg {
	m := &DBMsg{
		ID_: id,
	}
	err := b.db.Get(m, sqlSelectMsg, id)
	if err != nil {
		panic(err)
	}

	ch := &DBChannel{
		ID_: m.ChannelID_,
	}
	err = b.db.Get(ch, selectChannelSQL, m.ChannelID_)
	if err != nil {
		panic(err)
	}

	m.channel = ch
	return m
}
