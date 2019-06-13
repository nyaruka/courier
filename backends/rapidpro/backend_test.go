package rapidpro

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"encoding/json"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/garyburd/redigo/redis"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/null"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

type BackendTestSuite struct {
	suite.Suite
	b *backend
}

type mockS3Client struct {
	s3iface.S3API
}

func (m *mockS3Client) PutObject(*s3.PutObjectInput) (*s3.PutObjectOutput, error) {
	return nil, nil
}

func (m *mockS3Client) HeadBucket(*s3.HeadBucketInput) (*s3.HeadBucketOutput, error) {
	return nil, nil
}

func testConfig() *courier.Config {
	config := courier.NewConfig()
	config.DB = "postgres://courier@localhost/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return config
}

func (ts *BackendTestSuite) SetupSuite() {
	// turn off logging
	logrus.SetOutput(ioutil.Discard)

	b, err := courier.NewBackend(testConfig())
	if err != nil {
		log.Fatalf("unable to create rapidpro backend: %v", err)
	}
	ts.b = b.(*backend)

	err = ts.b.Start()
	if err != nil {
		log.Fatalf("unable to start backend for testing: %v", err)
	}

	// read our testdata sql
	sql, err := ioutil.ReadFile("testdata.sql")
	if err != nil {
		panic(fmt.Errorf("Unable to read testdata.sql: %s", err))
	}
	ts.b.db.MustExec(string(sql))

	// clear redis
	r := ts.b.redisPool.Get()
	defer r.Close()
	r.Do("FLUSHDB")

	// plug in our mock s3 client
	ts.b.s3Client = &mockS3Client{}
}

func (ts *BackendTestSuite) TearDownSuite() {
	ts.b.Stop()
	ts.b.Cleanup()
}

func (ts *BackendTestSuite) getChannel(cType string, cUUID string) *DBChannel {
	channelUUID, err := courier.NewChannelUUID(cUUID)
	ts.NoError(err, "error building channel uuid")

	channel, err := ts.b.GetChannel(context.Background(), courier.ChannelType(cType), channelUUID)
	ts.NoError(err, "error getting channel")
	ts.NotNil(channel)

	return channel.(*DBChannel)
}

func (ts *BackendTestSuite) TestMsgUnmarshal() {
	msgJSON := `{
		"status": "P",
		"direction": "O",
		"attachments": ["https://foo.bar/image.jpg"],
		"queued_on": null,
		"text": "Test message 21",
		"contact_id": 30,
		"contact_urn_id": 14,
		"error_count": 0,
		"modified_on": "2017-07-21T19:22:23.254133Z",
		"id": 204,
		"channel_uuid": "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba",
		"uuid": "54c893b9-b026-44fc-a490-50aed0361c3f",
		"next_attempt": "2017-07-21T19:22:23.254182Z",
		"urn": "telegram:3527065",
		"urn_auth": "5ApPVsFDcFt:RZdK9ne7LgfvBYdtCYg7tv99hC9P2",
		"org_id": 1,
		"created_on": "2017-07-21T19:22:23.242757Z",
		"sent_on": null,
		"high_priority": true,
		"channel_id": 11,
		"response_to_id": 15,
		"response_to_external_id": "external-id",
		"external_id": null,
		"metadata": {"quick_replies": ["Yes", "No"]}
	}`

	msg := DBMsg{}
	err := json.Unmarshal([]byte(msgJSON), &msg)
	ts.NoError(err)
	ts.Equal(msg.ChannelUUID_.String(), "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba")
	ts.Equal(msg.ChannelID_, courier.NewChannelID(11))
	ts.Equal([]string{"https://foo.bar/image.jpg"}, msg.Attachments())
	ts.Equal(msg.URNAuth_, "5ApPVsFDcFt:RZdK9ne7LgfvBYdtCYg7tv99hC9P2")
	ts.Equal(msg.ExternalID(), "")
	ts.Equal([]string{"Yes", "No"}, msg.QuickReplies())
	ts.Equal(courier.NewMsgID(15), msg.ResponseToID())
	ts.Equal("external-id", msg.ResponseToExternalID())
	ts.True(msg.HighPriority())

	msgJSONNoQR := `{
		"status": "P",
		"direction": "O",
		"attachments": ["https://foo.bar/image.jpg"],
		"queued_on": null,
		"text": "Test message 21",
		"contact_id": 30,
		"contact_urn_id": 14,
		"error_count": 0,
		"modified_on": "2017-07-21T19:22:23.254133Z",
		"id": 204,
		"channel_uuid": "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba",
		"uuid": "54c893b9-b026-44fc-a490-50aed0361c3f",
		"next_attempt": "2017-07-21T19:22:23.254182Z",
		"urn": "telegram:3527065",
		"org_id": 1,
		"created_on": "2017-07-21T19:22:23.242757Z",
		"sent_on": null,
		"high_priority": true,
		"channel_id": 11,
		"response_to_id": null,
		"response_to_external_id": "",
		"external_id": null,
		"metadata": null
	}`

	msg = DBMsg{}
	err = json.Unmarshal([]byte(msgJSONNoQR), &msg)
	ts.NoError(err)
	ts.Equal([]string{}, msg.QuickReplies())
	ts.Equal(courier.NilMsgID, msg.ResponseToID())
	ts.Equal("", msg.ResponseToExternalID())
}

func (ts *BackendTestSuite) TestCheckMsgExists() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	// check with invalid message id
	err := checkMsgExists(ts.b, ts.b.NewMsgStatusForID(knChannel, courier.NewMsgID(-1), courier.MsgStatusValue("S")))
	ts.Equal(err, courier.ErrMsgNotFound)

	// check with valid message id
	err = checkMsgExists(ts.b, ts.b.NewMsgStatusForID(knChannel, courier.NewMsgID(10000), courier.MsgStatusValue("S")))
	ts.Nil(err)

	// check with invalid external id
	err = checkMsgExists(ts.b, ts.b.NewMsgStatusForExternalID(knChannel, "ext-invalid", courier.MsgStatusValue("S")))
	ts.Equal(err, courier.ErrMsgNotFound)

	// check with valid external id
	status := ts.b.NewMsgStatusForExternalID(knChannel, "ext1", courier.MsgStatusValue("S"))
	err = checkMsgExists(ts.b, status)
	ts.Nil(err)
}

func (ts *BackendTestSuite) TestContact() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	urn, _ := urns.NewTelURNForCountry("12065551518", "US")

	ctx := context.Background()
	now := time.Now()

	// create our new contact
	contact, err := contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", "Ryan Lewis")
	ts.NoError(err)

	now2 := time.Now()

	// load this contact again by URN, should be same contact, name unchanged
	contact2, err := contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", "Other Name")
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
	contact, err = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, cURN, "", "")
	ts.NoError(err)
	ts.NotNil(contact)

	ts.Equal(null.String(""), contact.Name_)
	ts.Equal("a984069d-0008-4d8c-a772-b14a8a6acccc", contact.UUID_.String())

	urn, _ = urns.NewTelURNForCountry("12065551519", "US")

	// long name are truncated

	longName := "LongRandomNameHPGBRDjZvkz7y58jI2UPkio56IKGaMvaeDTvF74Q5SUkIHozFn1MLELfjX7vRrFto8YG2KPVaWzekgmFbkuxujIotFAgfhHqoHKW5c177FUtKf5YK9KbY8hp0x7PxIFY3MS5lMyMA5ELlqIgikThpr"
	contact3, err := contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", longName)
	ts.NoError(err)

	ts.Equal(null.String(longName[0:127]), contact3.Name_)

}

func (ts *BackendTestSuite) TestContactRace() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	urn, _ := urns.NewTelURNForCountry("12065551518", "US")

	urnSleep = true
	defer func() { urnSleep = false }()

	ctx := context.Background()

	// create our contact twice
	var contact1, contact2 *DBContact
	var err1, err2 error

	go func() {
		contact1, err1 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", "Ryan Lewis")
	}()
	go func() {
		contact2, err2 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, "", "Ryan Lewis")
	}()

	time.Sleep(time.Second)

	ts.NoError(err1)
	ts.NoError(err2)
	ts.Equal(contact1.ID_, contact2.ID_)
}

func (ts *BackendTestSuite) TestAddAndRemoveContactURN() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	ctx := context.Background()

	cURN, err := urns.NewTelURNForCountry("+12067799192", "US")
	ts.NoError(err)

	contact, err := contactForURN(ctx, ts.b, knChannel.OrgID_, knChannel, cURN, "", "")
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
	urn, _ := urns.NewTelURNForCountry("12065551515", "US")

	ctx := context.Background()

	contact, err := contactForURN(ctx, ts.b, knChannel.OrgID_, knChannel, urn, "", "")
	ts.NoError(err)

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	contact, err = contactForURN(ctx, ts.b, twChannel.OrgID_, twChannel, urn, "chestnut", "")
	ts.NoError(err)

	contactURNs, err := contactURNsForContact(tx, contact.ID_)
	ts.NoError(err)
	ts.Equal(null.String("chestnut"), contactURNs[0].Auth)

	// now build a URN for our number with the kannel channel
	knURN, err := contactURNForURN(tx, knChannel.OrgID_, knChannel.ID_, contact.ID_, urn, "sesame")
	ts.NoError(err)
	ts.NoError(tx.Commit())
	ts.Equal(knURN.OrgID, knChannel.OrgID_)
	ts.Equal(null.String("sesame"), knURN.Auth)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	// then with our twilio channel
	twURN, err := contactURNForURN(tx, twChannel.OrgID_, twChannel.ID_, contact.ID_, urn, "")
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
	twURN, err = contactURNForURN(tx, twChannel.OrgID_, twChannel.ID_, contact.ID_, urn, "peanut")
	ts.NoError(err)
	ts.NoError(tx.Commit())
	ts.Equal(null.String("peanut"), twURN.Auth)

	// test that we don't use display when looking up URNs
	tgChannel := ts.getChannel("TG", "dbc126ed-66bc-4e28-b67b-81dc3327c98a")
	tgURN, _ := urns.NewTelegramURN(12345, "")

	tgContact, err := contactForURN(ctx, ts.b, tgChannel.OrgID_, tgChannel, tgURN, "", "")
	ts.NoError(err)

	tgURNDisplay, _ := urns.NewTelegramURN(12345, "Jane")
	displayContact, err := contactForURN(ctx, ts.b, tgChannel.OrgID_, tgChannel, tgURNDisplay, "", "")

	ts.Equal(tgContact.URNID_, displayContact.URNID_)
	ts.Equal(tgContact.ID_, displayContact.ID_)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	tgContactURN, err := contactURNForURN(tx, tgChannel.OrgID_, tgChannel.ID_, tgContact.ID_, tgURNDisplay, "")
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
		contact2, err2 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn2, "", "")
		ts.NoError(err2)
		wait.Done()
	}()
	go func() {
		var err3 error
		contact3, err3 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn2, "", "")
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

	ctx := context.Background()

	knContact, err := contactForURN(ctx, ts.b, knChannel.OrgID_, knChannel, knURN, "", "")
	ts.NoError(err)

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	_, err = contactURNForURN(tx, knChannel.OrgID_, twChannel.ID_, knContact.ID_, twURN, "")
	ts.NoError(err)
	ts.NoError(tx.Commit())

	// ok, now looking up our contact should reset our URNs and their affinity..
	// TwitterURN should be first all all URNs should now use Twitter channel
	twContact, err := contactForURN(ctx, ts.b, twChannel.OrgID_, twChannel, twURN, "", "")
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
	time.Sleep(2 * time.Millisecond)

	// update by id with external id
	status := ts.b.NewMsgStatusForID(channel, courier.NewMsgID(10001), courier.MsgWired)
	status.SetExternalID("ext0")
	err := ts.b.WriteMsgStatus(ctx, status)
	ts.NoError(err)
	time.Sleep(time.Second)

	m, err := readMsgFromDB(ts.b, courier.NewMsgID(10001))
	ts.NoError(err)
	ts.Equal(m.Status_, courier.MsgWired)
	ts.Equal(m.ExternalID_, null.String("ext0"))
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.After(now))

	// update by id, no external id, shouldn't overwrite it
	status = ts.b.NewMsgStatusForID(channel, courier.NewMsgID(10001), courier.MsgSent)
	err = ts.b.WriteMsgStatus(ctx, status)
	ts.NoError(err)
	time.Sleep(time.Second)

	m, err = readMsgFromDB(ts.b, courier.NewMsgID(10001))
	ts.NoError(err)
	ts.Equal(m.Status_, courier.MsgSent)
	ts.Equal(m.ExternalID_, null.String("ext0"))
	ts.True(m.ModifiedOn_.After(now))

	// update by external id
	status = ts.b.NewMsgStatusForExternalID(channel, "ext1", courier.MsgFailed)
	err = ts.b.WriteMsgStatus(ctx, status)
	ts.NoError(err)
	time.Sleep(time.Second)
	m, err = readMsgFromDB(ts.b, courier.NewMsgID(10000))
	ts.NoError(err)
	ts.Equal(m.Status_, courier.MsgFailed)
	ts.True(m.ModifiedOn_.After(now))

	// no such external id
	status = ts.b.NewMsgStatusForExternalID(channel, "ext2", courier.MsgSent)
	err = ts.b.WriteMsgStatus(ctx, status)
	ts.Error(err)

	// reset our status to sent
	status = ts.b.NewMsgStatusForExternalID(channel, "ext1", courier.MsgSent)
	err = ts.b.WriteMsgStatus(ctx, status)
	time.Sleep(time.Second)

	// error our msg
	now = time.Now().In(time.UTC)
	time.Sleep(2 * time.Millisecond)
	status = ts.b.NewMsgStatusForExternalID(channel, "ext1", courier.MsgErrored)
	err = ts.b.WriteMsgStatus(ctx, status)
	ts.NoError(err)
	time.Sleep(time.Second)

	m, err = readMsgFromDB(ts.b, courier.NewMsgID(10000))
	ts.NoError(err)
	ts.Equal(m.Status_, courier.MsgErrored)
	ts.Equal(m.ErrorCount_, 1)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.NextAttempt_.After(now))

	// second go
	status = ts.b.NewMsgStatusForExternalID(channel, "ext1", courier.MsgErrored)
	err = ts.b.WriteMsgStatus(ctx, status)
	ts.NoError(err)
	time.Sleep(time.Second)

	m, err = readMsgFromDB(ts.b, courier.NewMsgID(10000))
	ts.NoError(err)
	ts.Equal(m.Status_, courier.MsgErrored)
	ts.Equal(m.ErrorCount_, 2)

	// third go
	status = ts.b.NewMsgStatusForExternalID(channel, "ext1", courier.MsgErrored)
	err = ts.b.WriteMsgStatus(ctx, status)
	time.Sleep(time.Second)

	ts.NoError(err)
	m, err = readMsgFromDB(ts.b, courier.NewMsgID(10000))
	ts.NoError(err)
	ts.Equal(m.Status_, courier.MsgFailed)
	ts.Equal(m.ErrorCount_, 3)
}

func (ts *BackendTestSuite) TestHealth() {
	// all should be well in test land
	ts.Equal(ts.b.Health(), "")
}

func (ts *BackendTestSuite) TestDupes() {
	r := ts.b.redisPool.Get()
	defer r.Close()

	ctx := context.Background()
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	urn, _ := urns.NewTelURNForCountry("12065551215", knChannel.Country())

	msg := ts.b.NewIncomingMsg(knChannel, urn, "ping").(*DBMsg)
	err := ts.b.WriteMsg(ctx, msg)
	ts.NoError(err)

	// grab our UUID
	uuid1 := msg.UUID().String()

	// trying again should lead to same UUID
	msg = ts.b.NewIncomingMsg(knChannel, urn, "ping").(*DBMsg)
	err = ts.b.WriteMsg(ctx, msg)
	ts.NoError(err)

	ts.Equal(uuid1, msg.UUID().String())

	// different message should change that
	msg = ts.b.NewIncomingMsg(knChannel, urn, "test").(*DBMsg)
	err = ts.b.WriteMsg(ctx, msg)
	ts.NoError(err)

	ts.NotEqual(uuid1, msg.UUID().String())
	uuid2 := msg.UUID().String()

	// an outgoing message should clear things
	dbMsg, err := readMsgFromDB(ts.b, courier.NewMsgID(10000))
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

	msg = ts.b.NewIncomingMsg(knChannel, urn, "test").(*DBMsg)
	err = ts.b.WriteMsg(ctx, msg)
	ts.NoError(err)

	ts.NotEqual(uuid2, msg.UUID().String())
}

func (ts *BackendTestSuite) TestExternalIDDupes() {
	r := ts.b.redisPool.Get()
	defer r.Close()

	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	urn, _ := urns.NewTelURNForCountry("12065551215", knChannel.Country())

	msg := newMsg(MsgIncoming, knChannel, urn, "ping")

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

	dbMsg, err := readMsgFromDB(ts.b, courier.NewMsgID(10000))
	dbMsg.ChannelUUID_, _ = courier.NewChannelUUID("dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	ts.NoError(err)
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

	dbMsg, err := readMsgFromDB(ts.b, courier.NewMsgID(10000))
	dbMsg.ChannelUUID_, _ = courier.NewChannelUUID("dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	ts.NoError(err)
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

	// make sure it is the message we just added
	ts.Equal(dbMsg.ID(), msg.ID())

	// and that it has the appropriate text
	ts.Equal(msg.Text(), "test message")

	// mark this message as dealt with
	ts.b.MarkOutgoingMsgComplete(ctx, msg, ts.b.NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgWired))

	// this message should now be marked as sent
	sent, err := ts.b.WasMsgSent(ctx, msg)
	ts.NoError(err)
	ts.True(sent)

	// pop another message off, shouldn't get anything
	msg2, err := ts.b.PopNextOutgoingMsg(ctx)
	ts.Nil(msg2)
	ts.Nil(err)

	// checking another message should show unsent
	msg3, err := readMsgFromDB(ts.b, courier.NewMsgID(10001))
	ts.NoError(err)
	sent, err = ts.b.WasMsgSent(ctx, msg3)
	ts.NoError(err)
	ts.False(sent)

	// write an error for our original message
	err = ts.b.WriteMsgStatus(ctx, ts.b.NewMsgStatusForID(msg.Channel(), msg.ID(), courier.MsgErrored))
	ts.NoError(err)

	// message should no longer be considered sent
	sent, err = ts.b.WasMsgSent(ctx, msg)
	ts.NoError(err)
	ts.False(sent)
}

func (ts *BackendTestSuite) TestChannel() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	ts.Equal("2500", knChannel.Address())
	ts.Equal("RW", knChannel.Country())

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
}

func (ts *BackendTestSuite) TestChanneLog() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	ctx := context.Background()

	log := courier.NewChannelLog("Message Send Error", knChannel, courier.NilMsgID, "POST", "/null/value", 400,
		"request with null \x00 content", "response with null \x00 content", time.Millisecond, nil)

	err := writeChannelLog(ctx, ts.b, log)
	ts.NoError(err)
}

func (ts *BackendTestSuite) TestWriteAttachment() {
	ctx := context.Background()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		content := ""
		switch r.URL.Path {
		case "/test.jpg":
			content = "malformedjpegbody"

		case "/giffy":
			content = "GIF87aandstuff"

		case "/header":
			w.Header().Add("Content-Type", "image/png")
			content = "nothingbody"

		default:
			content = "unknown"
		}

		w.Write([]byte(content))
	}))

	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	urn, _ := urns.NewTelURNForCountry("12065551215", knChannel.Country())
	msg := ts.b.NewIncomingMsg(knChannel, urn, "invalid attachment").(*DBMsg)
	msg.WithAttachment(testServer.URL)

	// should just end up being text/plain
	err := ts.b.WriteMsg(ctx, msg)
	ts.NoError(err)
	ts.True(strings.HasPrefix(msg.Attachments()[0], "text/plain"))

	// use an extension for our attachment instead
	msg = ts.b.NewIncomingMsg(knChannel, urn, "jpg attachment").(*DBMsg)
	msg.WithAttachment(testServer.URL + "/test.jpg")

	err = ts.b.WriteMsg(ctx, msg)
	ts.NoError(err)
	ts.True(strings.HasPrefix(msg.Attachments()[0], "image/jpeg:"))
	ts.True(strings.HasSuffix(msg.Attachments()[0], ".jpg"))

	// ok, now derive it from magic bytes
	msg = ts.b.NewIncomingMsg(knChannel, urn, "gif attachment").(*DBMsg)
	msg.WithAttachment(testServer.URL + "/giffy")

	err = ts.b.WriteMsg(ctx, msg)
	ts.NoError(err)
	if ts.Equal(1, len(msg.Attachments())) {
		ts.True(strings.HasPrefix(msg.Attachments()[0], "image/gif:"))
		ts.True(strings.HasSuffix(msg.Attachments()[0], ".gif"))
	}

	// finally from our header
	msg = ts.b.NewIncomingMsg(knChannel, urn, "png attachment").(*DBMsg)
	msg.WithAttachment(testServer.URL + "/header")

	err = ts.b.WriteMsg(ctx, msg)
	ts.NoError(err)
	if ts.Equal(1, len(msg.Attachments())) {
		ts.True(strings.HasPrefix(msg.Attachments()[0], "image/png:"))
		ts.True(strings.HasSuffix(msg.Attachments()[0], ".png"))
	}

	// load it back from the id
	m, err := readMsgFromDB(ts.b, msg.ID())
	ts.NoError(err)
	if ts.Equal(1, len(m.Attachments())) {
		ts.True(strings.HasPrefix(m.Attachments()[0], "image/png:"))
		ts.True(strings.HasSuffix(m.Attachments()[0], ".png"))
	}
}

func (ts *BackendTestSuite) TestWriteMsg() {
	ctx := context.Background()
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	// have to round to microseconds because postgres can't store nanos
	now := time.Now().Round(time.Microsecond).In(time.UTC)

	// create a new courier msg
	urn, _ := urns.NewTelURNForCountry("12065551212", knChannel.Country())
	msg := ts.b.NewIncomingMsg(knChannel, urn, "test123").WithExternalID("ext123").WithReceivedOn(now).WithContactName("test contact").(*DBMsg)

	// try to write it to our db
	err := ts.b.WriteMsg(ctx, msg)
	ts.NoError(err)

	// creating the incoming msg again should give us the same UUID and have the msg set as not to write
	time.Sleep(1 * time.Second)
	msg2 := ts.b.NewIncomingMsg(knChannel, urn, "test123").(*DBMsg)
	ts.Equal(msg2.UUID(), msg.UUID())
	ts.True(msg2.alreadyWritten)

	// check we had an id set
	ts.NotZero(msg.ID)

	// load it back from the id
	m, err := readMsgFromDB(ts.b, msg.ID())
	ts.NoError(err)

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	// load our URN
	contactURN, err := contactURNForURN(tx, m.OrgID_, m.ChannelID_, m.ContactID_, urn, "")
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

	contact, err := contactForURN(ctx, ts.b, m.OrgID_, knChannel, urn, "", "")
	ts.Equal(null.String("test contact"), contact.Name_)
	ts.Equal(m.OrgID_, contact.OrgID_)
	ts.Equal(m.ContactID_, contact.ID_)
	ts.NotNil(contact.UUID_)
	ts.NotNil(contact.ID_)

	// waiting 5 seconds should let us write it successfully
	time.Sleep(5 * time.Second)
	msg3 := ts.b.NewIncomingMsg(knChannel, urn, "test123").(*DBMsg)
	ts.NotEqual(msg3.UUID(), msg.UUID())

	// msg with null bytes in it, that's fine for a request body
	msg = ts.b.NewIncomingMsg(knChannel, urn, "test456\x00456").WithExternalID("ext456").(*DBMsg)
	err = writeMsgToDB(ctx, ts.b, msg)
	ts.NoError(err)

	// more null bytes
	text, _ := url.PathUnescape("%1C%00%00%00%00%00%07%E0%00")
	msg = ts.b.NewIncomingMsg(knChannel, urn, text).(*DBMsg)
	err = writeMsgToDB(ctx, ts.b, msg)
	ts.NoError(err)

	// check that our mailroom queue has an item
	rc := ts.b.redisPool.Get()
	defer rc.Close()
	rc.Do("DEL", "handler:1", "handler:active", fmt.Sprintf("c:1:%d", msg.ContactID_))

	msg = ts.b.NewIncomingMsg(knChannel, urn, "hello 1 2 3").(*DBMsg)
	err = writeMsgToDB(ctx, ts.b, msg)
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
}

func (ts *BackendTestSuite) TestChannelEvent() {
	ctx := context.Background()

	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	urn, _ := urns.NewTelURNForCountry("12065551616", channel.Country())
	event := ts.b.NewChannelEvent(channel, courier.Referral, urn).WithExtra(map[string]interface{}{"ref_id": "12345"}).WithContactName("kermit frog")
	err := ts.b.WriteChannelEvent(ctx, event)
	ts.NoError(err)

	contact, err := contactForURN(ctx, ts.b, channel.OrgID_, channel, urn, "", "")
	ts.NoError(err)
	ts.Equal(null.String("kermit frog"), contact.Name_)

	dbE := event.(*DBChannelEvent)
	dbE, err = readChannelEventFromDB(ts.b, dbE.ID_)
	ts.NoError(err)
	ts.Equal(dbE.EventType_, courier.Referral)
	ts.Equal(map[string]interface{}{"ref_id": "12345"}, dbE.Extra_.Map)
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
	count := 0
	ts.b.db.Get(&count, "SELECT count(*) from flows_flowsession WHERE timeout_on > NOW()")
	ts.Equal(1, count)
}

func (ts *BackendTestSuite) TestMailroomEvents() {
	ctx := context.Background()

	rc := ts.b.redisPool.Get()
	defer rc.Close()
	rc.Do("FLUSHDB")

	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	urn, _ := urns.NewTelURNForCountry("12065551616", channel.Country())
	event := ts.b.NewChannelEvent(channel, courier.Referral, urn).WithExtra(map[string]interface{}{"ref_id": "12345"}).WithContactName("kermit frog")
	err := ts.b.WriteChannelEvent(ctx, event)
	ts.NoError(err)

	contact, err := contactForURN(ctx, ts.b, channel.OrgID_, channel, urn, "", "")
	ts.NoError(err)
	ts.Equal(null.String("kermit frog"), contact.Name_)

	dbE := event.(*DBChannelEvent)
	dbE, err = readChannelEventFromDB(ts.b, dbE.ID_)
	ts.NoError(err)
	ts.Equal(dbE.EventType_, courier.Referral)
	ts.Equal(map[string]interface{}{"ref_id": "12345"}, dbE.Extra_.Map)
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
	{config: courier.Config{DB: "postgres://courier@localhost/courier", Redis: ":foo"}, expectedError: "unable to parse Redis URL"},
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
