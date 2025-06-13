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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/queue"
	"github.com/nyaruka/courier/test"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/gocommon/dates"
	"github.com/nyaruka/gocommon/dbutil/assertdb"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/i18n"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/null/v3"
	"github.com/nyaruka/vkutil/assertvk"
	"github.com/stretchr/testify/suite"
)

type BackendTestSuite struct {
	suite.Suite
	b *backend
}

func testConfig() *courier.Config {
	config := courier.NewDefaultConfig()
	config.DB = "postgres://courier_test:temba@localhost:5432/courier_test?sslmode=disable"
	config.Valkey = "valkey://localhost:6379/0"
	config.MediaDomain = "nyaruka.s3.com"

	// configure S3 to use a local minio instance
	config.AWSAccessKeyID = "root"
	config.AWSSecretAccessKey = "tembatemba"
	config.S3Endpoint = "http://localhost:9000"
	config.S3AttachmentsBucket = "test-attachments"
	config.S3Minio = true
	config.DynamoEndpoint = "http://localhost:6000"
	config.DynamoTablePrefix = "Test"

	return config
}

func (ts *BackendTestSuite) loadSQL(path string) {
	db, err := sqlx.Open("postgres", ts.b.config.DB)
	noError(err)

	sql, err := os.ReadFile(path)
	noError(err)
	db.MustExec(string(sql))
}

func (ts *BackendTestSuite) SetupSuite() {
	ctx := context.Background()
	cfg := testConfig()

	// turn off logging
	log.SetOutput(io.Discard)

	// create dynamo tables prior to starting backend, as it will check they exist
	dyn, err := dynamo.NewClient(cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, cfg.AWSRegion, cfg.DynamoEndpoint)
	noError(err)

	tablesFile, err := os.Open("dynamo.json")
	noError(err)
	defer tablesFile.Close()

	tablesJSON, err := io.ReadAll(tablesFile)
	noError(err)

	inputs := []*dynamodb.CreateTableInput{}
	jsonx.MustUnmarshal(tablesJSON, &inputs)

	for _, input := range inputs {
		input.TableName = aws.String(cfg.DynamoTablePrefix + *input.TableName) // add table prefix

		// delete table if it exists
		if _, err := dyn.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: input.TableName}); err == nil {
			_, err := dyn.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: input.TableName})
			must(err)
		}

		_, err := dyn.CreateTable(ctx, input)
		noError(err)
	}

	b, err := courier.NewBackend(cfg)
	noError(err)
	ts.b = b.(*backend)

	// load our test schema and data
	ts.loadSQL("schema.sql")
	ts.loadSQL("testdata.sql")

	must(ts.b.Start())

	ts.b.s3.Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("test-attachments")})
	ts.b.s3.Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("test-logs")})

	ts.clearValkey()
}

func (ts *BackendTestSuite) TearDownSuite() {
	ts.b.Stop()
	ts.b.Cleanup()

	ts.b.dynamo.Purge(context.Background())
	ts.b.s3.EmptyBucket(context.Background(), "test-attachments")
}

func (ts *BackendTestSuite) clearValkey() {
	// clear valkey
	r := ts.b.rp.Get()
	defer r.Close()
	_, err := r.Do("FLUSHDB")
	ts.Require().NoError(err)
}

func (ts *BackendTestSuite) getChannel(cType string, cUUID string) *Channel {
	channelUUID := courier.ChannelUUID(cUUID)

	channel, err := ts.b.GetChannel(context.Background(), courier.ChannelType(cType), channelUUID)
	ts.Require().NoError(err, "error getting channel")
	ts.Require().NotNil(channel)

	return channel.(*Channel)
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
		"is_resend": true
	}`

	msg := Msg{}
	err := json.Unmarshal([]byte(msgJSON), &msg)
	ts.NoError(err)
	ts.Equal(courier.ChannelUUID("f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba"), msg.ChannelUUID_)
	ts.Equal([]string{"https://foo.bar/image.jpg"}, msg.Attachments())
	ts.Equal("5ApPVsFDcFt:RZdK9ne7LgfvBYdtCYg7tv99hC9P2", msg.URNAuth_)
	ts.Equal("", msg.ExternalID())
	ts.Equal([]courier.QuickReply{{Text: "Yes"}, {Text: "No"}}, msg.QuickReplies())
	ts.Equal("external-id", msg.ResponseToExternalID())
	ts.True(msg.HighPriority())
	ts.True(msg.IsResend())
	flow_ref := courier.FlowReference{UUID: "9de3663f-c5c5-4c92-9f45-ecbc09abcc85", Name: "Favorites"}
	ts.Equal(&flow_ref, msg.Flow())

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

	msg = Msg{}
	err = json.Unmarshal([]byte(msgJSONNoQR), &msg)
	ts.NoError(err)
	ts.Nil(msg.Attachments())
	ts.Nil(msg.QuickReplies())
	ts.Equal("", msg.ResponseToExternalID())
	ts.False(msg.IsResend())
	ts.Nil(msg.Flow())
}

func (ts *BackendTestSuite) TestDeleteMsgByExternalID() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	ctx := context.Background()

	ts.clearValkey()

	// noop for invalid external ID
	err := ts.b.DeleteMsgByExternalID(ctx, knChannel, "ext-invalid")
	ts.Nil(err)

	// noop for external ID of outgoing message
	err = ts.b.DeleteMsgByExternalID(ctx, knChannel, "ext1")
	ts.Nil(err)

	ts.assertNoQueuedContactTask(ContactID(100))

	// a valid external id becomes a queued task
	err = ts.b.DeleteMsgByExternalID(ctx, knChannel, "ext2")
	ts.Nil(err)

	ts.assertQueuedContactTask(ContactID(100), "msg_deleted", map[string]any{"msg_id": float64(10002)})
}

func (ts *BackendTestSuite) TestContact() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn := urns.URN("tel:+12065551518")

	ctx := context.Background()
	now := time.Now()

	contact, err := contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, nil, "Ryan Lewis", false, clog)
	ts.NoError(err)
	ts.Nil(contact)

	// create our new contact
	contact, err = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, nil, "Ryan Lewis", true, clog)
	ts.NoError(err)

	now2 := time.Now()

	// load this contact again by URN, should be same contact, name unchanged
	contact2, err := contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, nil, "Other Name", true, clog)
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
	cURN := urns.URN("tel:+12067799192")
	contact, err = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, cURN, nil, "", true, clog)
	ts.NoError(err)
	ts.NotNil(contact)

	ts.Equal(null.String(""), contact.Name_)
	ts.Equal(courier.ContactUUID("a984069d-0008-4d8c-a772-b14a8a6acccc"), contact.UUID_)

	urn = urns.URN("tel:+12065551519")

	// long name are truncated

	longName := "LongRandomNameHPGBRDjZvkz7y58jI2UPkio56IKGaMvaeDTvF74Q5SUkIHozFn1MLELfjX7vRrFto8YG2KPVaWzekgmFbkuxujIotFAgfhHqoHKW5c177FUtKf5YK9KbY8hp0x7PxIFY3MS5lMyMA5ELlqIgikThpr"
	contact3, err := contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, nil, longName, true, clog)
	ts.NoError(err)

	ts.Equal(null.String(longName[0:127]), contact3.Name_)

}

func (ts *BackendTestSuite) TestContactRace() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn := urns.URN("tel:+12065551518")

	urnSleep = true
	defer func() { urnSleep = false }()

	ctx := context.Background()

	// create our contact twice
	var contact1, contact2 *Contact
	var err1, err2 error

	go func() {
		contact1, err1 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, nil, "Ryan Lewis", true, clog)
	}()
	go func() {
		contact2, err2 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn, nil, "Ryan Lewis", true, clog)
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

	cURN := urns.URN("tel:+12067799192")

	contact, err := contactForURN(ctx, ts.b, knChannel.OrgID_, knChannel, cURN, nil, "", true, clog)
	ts.NoError(err)
	ts.NotNil(contact)

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	contactURNs, err := getURNsForContact(tx, contact.ID_)
	ts.NoError(err)
	ts.Equal(len(contactURNs), 1)

	urn := urns.URN("tel:+12065551518")
	addedURN, err := ts.b.AddURNtoContact(ctx, knChannel, contact, urn, nil)
	ts.NoError(err)
	ts.NotNil(addedURN)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	contactURNs, err = getURNsForContact(tx, contact.ID_)
	ts.NoError(err)
	ts.Equal(len(contactURNs), 2)

	removedURN, err := ts.b.RemoveURNfromContact(ctx, knChannel, contact, urn)
	ts.NoError(err)
	ts.NotNil(removedURN)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)
	contactURNs, err = getURNsForContact(tx, contact.ID_)
	ts.NoError(err)
	ts.Equal(len(contactURNs), 1)
}

func (ts *BackendTestSuite) TestContactURN() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	fbChannel := ts.getChannel("FBA", "dbc126ed-66bc-4e28-b67b-81dc3327c96a")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn := urns.URN("tel:+12065551515")

	ctx := context.Background()

	contact, err := contactForURN(ctx, ts.b, knChannel.OrgID_, knChannel, urn, nil, "", true, clog)
	ts.NoError(err)
	ts.NotNil(contact)

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	contact, err = contactForURN(ctx, ts.b, fbChannel.OrgID_, fbChannel, urn, map[string]string{"token1": "chestnut"}, "", true, clog)
	ts.NoError(err)
	ts.NotNil(contact)

	contactURNs, err := getURNsForContact(tx, contact.ID_)
	ts.NoError(err)
	ts.Equal(null.Map[string]{"token1": "chestnut"}, contactURNs[0].AuthTokens)

	// now build a URN for our number with the kannel channel
	knURN, err := getOrCreateContactURN(tx, knChannel, contact.ID_, urn, map[string]string{"token2": "sesame"})
	ts.NoError(err)
	ts.NoError(tx.Commit())
	ts.Equal(knURN.OrgID, knChannel.OrgID_)
	ts.Equal(null.Map[string]{"token1": "chestnut", "token2": "sesame"}, knURN.AuthTokens)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	// then with our twilio channel
	fbURN, err := getOrCreateContactURN(tx, fbChannel, contact.ID_, urn, nil)
	ts.NoError(err)
	ts.NoError(tx.Commit())

	// should be the same URN
	ts.Equal(knURN.ID, fbURN.ID)

	// same contact
	ts.Equal(knURN.ContactID, fbURN.ContactID)

	// and channel should be set to facebook
	ts.Equal(fbURN.ChannelID, fbChannel.ID())

	// auth should be unchanged
	ts.Equal(null.Map[string]{"token1": "chestnut", "token2": "sesame"}, fbURN.AuthTokens)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	// again with different auth
	fbURN, err = getOrCreateContactURN(tx, fbChannel, contact.ID_, urn, map[string]string{"token3": "peanut"})
	ts.NoError(err)
	ts.NoError(tx.Commit())
	ts.Equal(null.Map[string]{"token1": "chestnut", "token2": "sesame", "token3": "peanut"}, fbURN.AuthTokens)

	// test that we don't use display when looking up URNs
	tgChannel := ts.getChannel("TG", "dbc126ed-66bc-4e28-b67b-81dc3327c98a")
	tgURN := urns.URN("telegram:12345")

	tgContact, err := contactForURN(ctx, ts.b, tgChannel.OrgID_, tgChannel, tgURN, nil, "", true, clog)
	ts.NoError(err)

	tgURNDisplay := urns.URN("telegram:12345#Jane")
	displayContact, err := contactForURN(ctx, ts.b, tgChannel.OrgID_, tgChannel, tgURNDisplay, nil, "", true, clog)

	ts.NoError(err)
	ts.Equal(tgContact.URNID_, displayContact.URNID_)
	ts.Equal(tgContact.ID_, displayContact.ID_)

	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	tgContactURN, err := getOrCreateContactURN(tx, tgChannel, tgContact.ID_, tgURNDisplay, nil)
	ts.NoError(err)
	ts.NoError(tx.Commit())
	ts.Equal(tgContact.URNID_, tgContactURN.ID)
	ts.Equal(null.String("Jane"), tgContactURN.Display)

	// try to create two contacts at the same time in goroutines, this tests our transaction rollbacks
	urn2 := urns.URN("tel:+12065551616")
	var wait sync.WaitGroup
	var contact2, contact3 *Contact
	wait.Add(2)
	go func() {
		var err2 error
		contact2, err2 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn2, nil, "", true, clog)
		ts.NoError(err2)
		wait.Done()
	}()
	go func() {
		var err3 error
		contact3, err3 = contactForURN(ctx, ts.b, knChannel.OrgID(), knChannel, urn2, nil, "", true, clog)
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
	fbChannel := ts.getChannel("FBA", "dbc126ed-66bc-4e28-b67b-81dc3327c96a")
	knURN := urns.URN("tel:+12065551111")
	fbURN := urns.URN("tel:+12065552222")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)

	ctx := context.Background()

	knContact, err := contactForURN(ctx, ts.b, knChannel.OrgID_, knChannel, knURN, nil, "", true, clog)
	ts.NoError(err)

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	_, err = getOrCreateContactURN(tx, fbChannel, knContact.ID_, fbURN, nil)
	ts.NoError(err)
	ts.NoError(tx.Commit())

	// ok, now looking up our contact should reset our URNs and their affinity..
	// FacebookURN should be first all all URNs should now use Facebook channel
	fbContact, err := contactForURN(ctx, ts.b, fbChannel.OrgID_, fbChannel, fbURN, nil, "", true, clog)
	ts.NoError(err)

	ts.Equal(fbContact.ID_, knContact.ID_)

	// get all the URNs for this contact
	tx, err = ts.b.db.Beginx()
	ts.NoError(err)

	urns, err := getURNsForContact(tx, fbContact.ID_)
	ts.NoError(err)
	ts.NoError(tx.Commit())

	ts.Equal("tel:+12065552222", urns[0].Identity)
	ts.Equal(fbChannel.ID(), urns[0].ChannelID)

	ts.Equal("tel:+12065551111", urns[1].Identity)
	ts.Equal(fbChannel.ID(), urns[1].ChannelID)
}

func (ts *BackendTestSuite) TestMsgStatus() {
	ctx := context.Background()
	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	now := time.Now().In(time.UTC)

	updateStatusByID := func(id courier.MsgID, status courier.MsgStatus, newExtID string) *courier.ChannelLog {
		clog := courier.NewChannelLog(courier.ChannelLogTypeMsgStatus, channel, nil)
		statusObj := ts.b.NewStatusUpdate(channel, id, status, clog)
		if newExtID != "" {
			statusObj.SetExternalID(newExtID)
		}
		err := ts.b.WriteStatusUpdate(ctx, statusObj)
		ts.NoError(err)
		time.Sleep(600 * time.Millisecond) // give committer time to write this
		return clog
	}

	updateStatusByExtID := func(extID string, status courier.MsgStatus) *courier.ChannelLog {
		clog := courier.NewChannelLog(courier.ChannelLogTypeMsgStatus, channel, nil)
		statusObj := ts.b.NewStatusUpdateByExternalID(channel, extID, status, clog)
		err := ts.b.WriteStatusUpdate(ctx, statusObj)
		ts.NoError(err)
		time.Sleep(600 * time.Millisecond) // give committer time to write this
		return clog
	}

	// put test message back into queued state
	ts.b.db.MustExec(`UPDATE msgs_msg SET status = 'Q', sent_on = NULL WHERE id = $1`, 10001)

	// update to WIRED using id and provide new external ID
	clog1 := updateStatusByID(10001, courier.MsgStatusWired, "ext0")

	m := readMsgFromDB(ts.b, 10001)
	ts.Equal(courier.MsgStatusWired, m.Status_)
	ts.Equal(null.String("ext0"), m.ExternalID_)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.After(now))
	ts.Equal(null.NullString, m.FailedReason_)
	ts.Equal(pq.StringArray([]string{string(clog1.UUID)}), m.LogUUIDs)

	sentOn := *m.SentOn_

	// update to SENT using id
	clog2 := updateStatusByID(10001, courier.MsgStatusSent, "")

	m = readMsgFromDB(ts.b, 10001)
	ts.Equal(courier.MsgStatusSent, m.Status_)
	ts.Equal(null.String("ext0"), m.ExternalID_) // no change
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.Equal(sentOn)) // no change
	ts.Equal(pq.StringArray([]string{string(clog1.UUID), string(clog2.UUID)}), m.LogUUIDs)

	// update to DELIVERED using id
	clog3 := updateStatusByID(10001, courier.MsgStatusDelivered, "")

	m = readMsgFromDB(ts.b, 10001)
	ts.Equal(m.Status_, courier.MsgStatusDelivered)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.Equal(sentOn)) // no change
	ts.Equal(pq.StringArray([]string{string(clog1.UUID), string(clog2.UUID), string(clog3.UUID)}), m.LogUUIDs)

	// update to READ using id
	clog4 := updateStatusByID(10001, courier.MsgStatusRead, "")

	m = readMsgFromDB(ts.b, 10001)
	ts.Equal(m.Status_, courier.MsgStatusRead)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.Equal(sentOn)) // no change
	ts.Equal(pq.StringArray([]string{string(clog1.UUID), string(clog2.UUID), string(clog3.UUID), string(clog4.UUID)}), m.LogUUIDs)

	// no change for incoming messages
	updateStatusByID(10002, courier.MsgStatusSent, "")

	m = readMsgFromDB(ts.b, 10002)
	ts.Equal(courier.MsgStatusPending, m.Status_)
	ts.Equal(m.ExternalID_, null.String("ext2"))
	ts.Equal(pq.StringArray(nil), m.LogUUIDs)

	// update to FAILED using external id
	clog5 := updateStatusByExtID("ext1", courier.MsgStatusFailed)

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(courier.MsgStatusFailed, m.Status_)
	ts.True(m.ModifiedOn_.After(now))
	ts.Nil(m.SentOn_)
	ts.Equal(pq.StringArray([]string{string(clog5.UUID)}), m.LogUUIDs)

	now = time.Now().In(time.UTC)
	time.Sleep(2 * time.Millisecond)

	// update to WIRED using external id
	clog6 := updateStatusByExtID("ext1", courier.MsgStatusWired)

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(courier.MsgStatusWired, m.Status_)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.After(now))

	sentOn = *m.SentOn_

	// update to SENT using external id
	updateStatusByExtID("ext1", courier.MsgStatusSent)

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(courier.MsgStatusSent, m.Status_)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.SentOn_.Equal(sentOn)) // no change

	// put test outgoing messages back into queued state
	ts.b.db.MustExec(`UPDATE msgs_msg SET status = 'Q', sent_on = NULL WHERE id IN ($1, $2)`, 10002, 10001)

	// can skip WIRED and go straight to SENT or DELIVERED
	updateStatusByExtID("ext1", courier.MsgStatusSent)
	updateStatusByID(10001, courier.MsgStatusDelivered, "")

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(courier.MsgStatusSent, m.Status_)
	ts.NotNil(m.SentOn_)
	m = readMsgFromDB(ts.b, 10001)
	ts.Equal(courier.MsgStatusDelivered, m.Status_)
	ts.NotNil(m.SentOn_)

	// reset our status to sent
	status := ts.b.NewStatusUpdateByExternalID(channel, "ext1", courier.MsgStatusSent, clog6)
	err := ts.b.WriteStatusUpdate(ctx, status)
	ts.NoError(err)
	time.Sleep(time.Second)

	// error our msg
	now = time.Now().In(time.UTC)
	time.Sleep(2 * time.Millisecond)
	status = ts.b.NewStatusUpdateByExternalID(channel, "ext1", courier.MsgStatusErrored, clog6)
	err = ts.b.WriteStatusUpdate(ctx, status)
	ts.NoError(err)

	time.Sleep(time.Second) // give committer time to write this

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(m.Status_, courier.MsgStatusErrored)
	ts.Equal(m.ErrorCount_, 1)
	ts.True(m.ModifiedOn_.After(now))
	ts.True(m.NextAttempt_.After(now))
	ts.Equal(null.NullString, m.FailedReason_)

	// second go
	status = ts.b.NewStatusUpdateByExternalID(channel, "ext1", courier.MsgStatusErrored, clog6)
	err = ts.b.WriteStatusUpdate(ctx, status)
	ts.NoError(err)

	time.Sleep(time.Second) // give committer time to write this

	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(m.Status_, courier.MsgStatusErrored)
	ts.Equal(m.ErrorCount_, 2)
	ts.Equal(null.NullString, m.FailedReason_)

	// third go
	status = ts.b.NewStatusUpdateByExternalID(channel, "ext1", courier.MsgStatusErrored, clog6)
	err = ts.b.WriteStatusUpdate(ctx, status)

	time.Sleep(time.Second) // give committer time to write this

	ts.NoError(err)
	m = readMsgFromDB(ts.b, 10000)
	ts.Equal(m.Status_, courier.MsgStatusFailed)
	ts.Equal(m.ErrorCount_, 3)
	ts.Equal(null.String("E"), m.FailedReason_)

	// update URN when the new doesn't exist
	tx, _ := ts.b.db.BeginTxx(ctx, nil)
	oldURN := urns.URN("whatsapp:55988776655")
	_ = insertContactURN(tx, newContactURN(channel.OrgID_, channel.ID_, NilContactID, oldURN, nil))

	ts.NoError(tx.Commit())

	newURN := urns.URN("whatsapp:5588776655")
	status = ts.b.NewStatusUpdate(channel, courier.MsgID(10000), courier.MsgStatusSent, clog6)
	status.SetURNUpdate(oldURN, newURN)

	ts.NoError(ts.b.WriteStatusUpdate(ctx, status))

	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	contactURN, err := getContactURNByIdentity(tx, channel.OrgID_, newURN)

	ts.NoError(err)
	ts.Equal(contactURN.Identity, newURN.Identity().String())
	ts.NoError(tx.Commit())

	// new URN already exits but don't have an associated contact
	oldURN = urns.URN("whatsapp:55999887766")
	newURN = urns.URN("whatsapp:5599887766")
	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	contact, _ := contactForURN(ctx, ts.b, channel.OrgID_, channel, oldURN, nil, "", true, clog6)
	_ = insertContactURN(tx, newContactURN(channel.OrgID_, channel.ID_, NilContactID, newURN, nil))

	ts.NoError(tx.Commit())

	status = ts.b.NewStatusUpdate(channel, courier.MsgID(10007), courier.MsgStatusSent, clog6)
	status.SetURNUpdate(oldURN, newURN)

	ts.NoError(ts.b.WriteStatusUpdate(ctx, status))

	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	newContactURN, _ := getContactURNByIdentity(tx, channel.OrgID_, newURN)
	oldContactURN, _ := getContactURNByIdentity(tx, channel.OrgID_, oldURN)

	ts.Equal(newContactURN.ContactID, contact.ID_)
	ts.Equal(oldContactURN.ContactID, NilContactID)
	ts.NoError(tx.Commit())

	// new URN already exits and have an associated contact
	oldURN = urns.URN("whatsapp:55988776655")
	newURN = urns.URN("whatsapp:5588776655")
	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	_, _ = contactForURN(ctx, ts.b, channel.OrgID_, channel, oldURN, nil, "", true, clog6)
	otherContact, _ := contactForURN(ctx, ts.b, channel.OrgID_, channel, newURN, nil, "", true, clog6)

	ts.NoError(tx.Commit())

	status = ts.b.NewStatusUpdate(channel, courier.MsgID(10007), courier.MsgStatusSent, clog6)
	status.SetURNUpdate(oldURN, newURN)

	ts.NoError(ts.b.WriteStatusUpdate(ctx, status))

	tx, _ = ts.b.db.BeginTxx(ctx, nil)
	oldContactURN, _ = getContactURNByIdentity(tx, channel.OrgID_, oldURN)
	newContactURN, _ = getContactURNByIdentity(tx, channel.OrgID_, newURN)

	ts.Equal(oldContactURN.ContactID, NilContactID)
	ts.Equal(newContactURN.ContactID, otherContact.ID_)
	ts.NoError(tx.Commit())
}

func (ts *BackendTestSuite) TestSentExternalIDCaching() {
	rc := ts.b.rp.Get()
	defer rc.Close()

	ctx := context.Background()
	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeMsgSend, channel, nil)

	ts.clearValkey()

	// create a status update from a send which will have id and external id
	status1 := ts.b.NewStatusUpdate(channel, 10000, courier.MsgStatusSent, clog)
	status1.SetExternalID("ex457")
	err := ts.b.WriteStatusUpdate(ctx, status1)
	ts.NoError(err)

	// give batcher time to write it
	time.Sleep(time.Millisecond * 600)

	keys, err := redis.Strings(rc.Do("KEYS", "sent-external-ids:*"))
	ts.NoError(err)
	ts.Len(keys, 1)
	assertvk.HGetAll(ts.T(), rc, keys[0], map[string]string{"10|ex457": "10000"})

	// mimic a delay in that status being written by reverting the db changes
	ts.b.db.MustExec(`UPDATE msgs_msg SET status = 'W', external_id = NULL WHERE id = 10000`)

	// create a callback status update which only has external id
	status2 := ts.b.NewStatusUpdateByExternalID(channel, "ex457", courier.MsgStatusDelivered, clog)

	err = ts.b.WriteStatusUpdate(ctx, status2)
	ts.NoError(err)

	// give batcher time to write it
	time.Sleep(time.Millisecond * 700)

	// msg status successfully updated in the database
	assertdb.Query(ts.T(), ts.b.db, `SELECT status FROM msgs_msg WHERE id = 10000`).Returns("D")
}

func (ts *BackendTestSuite) TestHealth() {
	// all should be well in test land
	ts.Equal(ts.b.Health(), "")
}

func (ts *BackendTestSuite) TestCheckForDuplicate() {
	rc := ts.b.rp.Get()
	defer rc.Close()

	ctx := context.Background()
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	twChannel := ts.getChannel("FBA", "dbc126ed-66bc-4e28-b67b-81dc3327c96a")
	urn := urns.URN("tel:+12065551215")
	urn2 := urns.URN("tel:+12065551277")

	createAndWriteMsg := func(ch courier.Channel, u urns.URN, text, extID string) *Msg {
		clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
		m := ts.b.NewIncomingMsg(ctx, ch, u, text, extID, clog).(*Msg)
		err := ts.b.WriteMsg(ctx, m, clog)
		ts.NoError(err)
		return m
	}

	msg1 := createAndWriteMsg(knChannel, urn, "ping", "")
	ts.False(msg1.alreadyWritten)

	keys, err := redis.Strings(rc.Do("KEYS", "seen-msgs:*"))
	ts.NoError(err)
	ts.Len(keys, 1)
	assertvk.HGetAll(ts.T(), rc, keys[0], map[string]string{
		"dbc126ed-66bc-4e28-b67b-81dc3327c95d|tel:+12065551215": string(msg1.UUID()) + "|fb826459f96c6e3ee563238d158a24702afbdd78",
	})

	// trying again should lead to same UUID
	msg2 := createAndWriteMsg(knChannel, urn, "ping", "")
	ts.Equal(msg1.UUID(), msg2.UUID())
	ts.True(msg2.alreadyWritten)

	// different text should change that
	msg3 := createAndWriteMsg(knChannel, urn, "test", "")
	ts.NotEqual(msg2.UUID(), msg3.UUID())
	ts.False(msg3.alreadyWritten)

	// an outgoing message should clear things
	dbMsg := readMsgFromDB(ts.b, 10000)
	dbMsg.URN_ = urn
	dbMsg.channel = knChannel
	dbMsg.ChannelUUID_ = knChannel.UUID()
	dbMsg.Text_ = "test"

	msgJSON, err := json.Marshal([]any{dbMsg})
	ts.NoError(err)
	err = queue.PushOntoQueue(rc, msgQueueName, "dbc126ed-66bc-4e28-b67b-81dc3327c95d", 10, string(msgJSON), queue.HighPriority)
	ts.NoError(err)
	_, err = ts.b.PopNextOutgoingMsg(ctx)
	ts.NoError(err)

	msg4 := createAndWriteMsg(knChannel, urn, "test", "")
	ts.NotEqual(msg3.UUID(), msg4.UUID())

	// message on a different channel but same text won't be considered a dupe
	msg5 := createAndWriteMsg(twChannel, urn, "test", "")
	ts.NotEqual(msg4.UUID(), msg5.UUID())

	// message on a different URN but same text won't be considered a dupe
	msg6 := createAndWriteMsg(twChannel, urn2, "test", "")
	ts.NotEqual(msg5.UUID(), msg6.UUID())

	// when messages have external IDs those are used to de-dupe and text is ignored
	msg7 := createAndWriteMsg(twChannel, urn, "test", "EX123")
	msg8 := createAndWriteMsg(twChannel, urn, "testtest", "EX123")
	msg9 := createAndWriteMsg(twChannel, urn, "test", "EX234")

	ts.Equal(msg7.UUID(), msg8.UUID())
	ts.NotEqual(msg7.UUID(), msg9.UUID())

	ts.False(msg7.alreadyWritten)
	ts.True(msg8.alreadyWritten)
	ts.False(msg9.alreadyWritten)
}

func (ts *BackendTestSuite) TestStatus() {
	// our health should just contain the header
	ts.True(strings.Contains(ts.b.Status(), "Channel"), ts.b.Status())

	// add a message to our queue
	r := ts.b.rp.Get()
	defer r.Close()

	dbMsg := readMsgFromDB(ts.b, 10000)
	dbMsg.ChannelUUID_ = courier.ChannelUUID("dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	ts.NotNil(dbMsg)

	// serialize our message
	msgJSON, err := json.Marshal([]any{dbMsg})
	ts.NoError(err)

	err = queue.PushOntoQueue(r, msgQueueName, "dbc126ed-66bc-4e28-b67b-81dc3327c95d", 10, string(msgJSON), queue.HighPriority)
	ts.NoError(err)

	// status should now contain that channel
	ts.True(strings.Contains(ts.b.Status(), "1           0         0    10     KN   dbc126ed-66bc-4e28-b67b-81dc3327c95d"), ts.b.Status())
}

func (ts *BackendTestSuite) TestOutgoingQueue() {
	// add one of our outgoing messages to the queue
	ctx := context.Background()
	r := ts.b.rp.Get()
	defer r.Close()

	dbMsg := readMsgFromDB(ts.b, 10000)
	dbMsg.ChannelUUID_ = courier.ChannelUUID("dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	ts.NotNil(dbMsg)

	// serialize our message
	msgJSON, err := json.Marshal([]any{dbMsg})
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
	ts.b.OnSendComplete(ctx, msg, ts.b.NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusWired, clog), clog)

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
	err = ts.b.WriteStatusUpdate(ctx, ts.b.NewStatusUpdate(msg.Channel(), msg.ID(), courier.MsgStatusErrored, clog))
	ts.NoError(err)

	// message should no longer be considered sent
	sent, err = ts.b.WasMsgSent(ctx, msg.ID())
	ts.NoError(err)
	ts.False(sent)
}

func (ts *BackendTestSuite) TestChannel() {
	noAddress := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c99a")
	ts.Equal(i18n.Country("US"), noAddress.Country())
	ts.Equal(courier.NilChannelAddress, noAddress.ChannelAddress())

	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	ts.Equal("2500", knChannel.Address())
	ts.Equal(courier.ChannelAddress("2500"), knChannel.ChannelAddress())
	ts.Equal(i18n.Country("RW"), knChannel.Country())
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

	err = ts.b.WriteChannelLog(ctx, clog1)
	ts.NoError(err)

	time.Sleep(time.Second) // give writer time to write this

	// check that we can read the log back from DynamoDB
	item1, err := ts.b.dynamo.GetItem(ctx, GetChannelLogKey(clog1))
	ts.NoError(err)
	ts.Equal(1, item1.OrgID)
	ts.Equal("token_refresh", item1.Data["type"])
	ts.NotNil(item1.DataGZ)

	var dataGZ map[string]any
	err = dynamo.UnmarshalJSONGZ(item1.DataGZ, &dataGZ)
	ts.NoError(err)
	ts.NotNil(dataGZ["http_logs"])
	ts.Equal("https://api.messages.com/send.json", dataGZ["http_logs"].([]any)[0].(map[string]any)["url"])

	clog2 := courier.NewChannelLog(courier.ChannelLogTypeMsgSend, channel, nil)
	clog2.HTTP(trace)

	err = ts.b.WriteChannelLog(ctx, clog2)
	ts.NoError(err)

	time.Sleep(time.Second) // give writer time to write this

	// check that we can read the log back from DynamoDB
	item2, err := ts.b.dynamo.GetItem(ctx, GetChannelLogKey(clog2))
	ts.NoError(err)
	ts.Equal("msg_send", item2.Data["type"])

	// channel channel log policy to only write errors
	channel.LogPolicy = LogPolicyErrors

	clog3 := courier.NewChannelLog(courier.ChannelLogTypeMsgSend, channel, nil)
	clog3.HTTP(trace)
	ts.NoError(ts.b.WriteChannelLog(ctx, clog3))

	time.Sleep(time.Second) // give writer time to.. not write this

	item3, err := ts.b.dynamo.GetItem(ctx, GetChannelLogKey(clog3))
	ts.NoError(err)
	ts.Nil(item3)

	clog4 := courier.NewChannelLog(courier.ChannelLogTypeMsgSend, channel, nil)
	clog4.HTTP(trace)
	clog4.Error(courier.ErrorResponseStatusCode())
	ts.NoError(ts.b.WriteChannelLog(ctx, clog4))

	time.Sleep(time.Second) // give writer time to write this because it's an error

	item4, err := ts.b.dynamo.GetItem(ctx, GetChannelLogKey(clog4))
	ts.NoError(err)
	ts.NotNil(item4)

	// channel channel log policy to discard all
	channel.LogPolicy = LogPolicyNone

	clog5 := courier.NewChannelLog(courier.ChannelLogTypeMsgSend, channel, nil)
	clog5.HTTP(trace)
	clog5.Error(courier.ErrorResponseStatusCode())
	ts.NoError(ts.b.WriteChannelLog(ctx, clog5))

	time.Sleep(time.Second) // give writer time to.. not write this

	item5, err := ts.b.dynamo.GetItem(ctx, GetChannelLogKey(clog5))
	ts.NoError(err)
	ts.Nil(item5)

	count, err := ts.b.dynamo.Count(ctx)
	ts.NoError(err)
	ts.Equal(3, count)
}

func (ts *BackendTestSuite) TestSaveAttachment() {
	testJPG := test.ReadFile("../../test/testdata/test.jpg")
	ctx := context.Background()

	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, time.Now))

	newURL, err := ts.b.SaveAttachment(ctx, knChannel, "image/jpeg", testJPG, "jpg")
	ts.NoError(err)
	ts.Equal("http://localhost:9000/test-attachments/attachments/1/c00e/5d67/c00e5d67-c275-4389-aded-7d8b151cbd5b.jpg", newURL)
}

func (ts *BackendTestSuite) TestWriteMsg() {
	ctx := context.Background()
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)

	// have to round to microseconds because postgres can't store nanos
	now := time.Now().Round(time.Microsecond).In(time.UTC)

	// create a new courier msg
	urn := urns.URN("tel:+12065551212")
	msg1 := ts.b.NewIncomingMsg(ctx, knChannel, urn, "test123", "ext123", clog).WithReceivedOn(now).WithContactName("test contact").(*Msg)

	// try to write it to our db
	err := ts.b.WriteMsg(ctx, msg1, clog)
	ts.NoError(err)

	// creating the incoming msg again should give us the same UUID and have the msg set as not to write
	time.Sleep(1 * time.Second)
	msg2 := ts.b.NewIncomingMsg(ctx, knChannel, urn, "test123", "ext123", clog).(*Msg)
	ts.Equal(msg2.UUID(), msg1.UUID())
	ts.True(msg2.alreadyWritten)

	// check we had an id set
	ts.NotZero(msg1.ID)

	// load it back from the id
	m := readMsgFromDB(ts.b, msg1.ID())

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	// load our URN
	contactURN, err := getOrCreateContactURN(tx, m.channel, m.ContactID_, urn, nil)
	if !ts.NoError(err) || !ts.NoError(tx.Commit()) {
		ts.FailNow("failed writing contact urn")
	}

	// make sure our values are set appropriately
	ts.Equal(msg1.ID(), m.ID())
	ts.Equal(knChannel.ID_, m.ChannelID_)
	ts.Equal(knChannel.OrgID_, m.OrgID_)
	ts.Equal(contactURN.ContactID, m.ContactID_)
	ts.Equal(contactURN.ID, m.ContactURNID_)
	ts.Equal(MsgIncoming, m.Direction_)
	ts.Equal(courier.MsgStatusPending, m.Status_)
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

	contact, err := contactForURN(ctx, ts.b, m.OrgID_, knChannel, urn, nil, "", true, clog)
	ts.NoError(err)
	ts.Equal(null.String("test contact"), contact.Name_)
	ts.Equal(m.OrgID_, contact.OrgID_)
	ts.Equal(m.ContactID_, contact.ID_)
	ts.NotNil(contact.UUID_)
	ts.NotNil(contact.ID_)

	// waiting 5 seconds should let us write it successfully
	time.Sleep(5 * time.Second)
	msg3 := ts.b.NewIncomingMsg(ctx, knChannel, urn, "test123", "", clog).(*Msg)
	ts.Greater(msg3.UUID(), msg1.UUID())

	// msg with null bytes in it, that's fine for a request body
	msg4 := ts.b.NewIncomingMsg(ctx, knChannel, urn, "test456\x00456", "ext456", clog).(*Msg)
	_, err = writeMsgToDB(ctx, ts.b, msg4, clog)
	ts.NoError(err)

	// more null bytes
	text, _ := url.PathUnescape("%1C%00%00%00%00%00%07%E0%00")
	msg5 := ts.b.NewIncomingMsg(ctx, knChannel, urn, text, "", clog).(*Msg)
	_, err = writeMsgToDB(ctx, ts.b, msg5, clog)
	ts.NoError(err)

	ts.clearValkey()

	// check that msg is queued to mailroom for handling
	msg6 := ts.b.NewIncomingMsg(ctx, knChannel, urn, "hello 1 2 3", "", clog).(*Msg)
	err = ts.b.WriteMsg(ctx, msg6, clog)
	ts.NoError(err)

	ts.assertQueuedContactTask(msg6.ContactID_, "msg_received", map[string]any{
		"channel_id":      float64(10),
		"msg_id":          float64(msg6.ID_),
		"msg_uuid":        string(msg6.UUID()),
		"msg_external_id": msg6.ExternalID(),
		"urn":             msg6.URN().String(),
		"urn_id":          float64(msg6.ContactURNID_),
		"text":            msg6.Text(),
		"attachments":     nil,
		"new_contact":     contact.IsNew_,
	})
}

func (ts *BackendTestSuite) TestWriteMsgWithAttachments() {
	ctx := context.Background()

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	uuids.SetGenerator(uuids.NewSeededGenerator(1234, time.Now))

	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, knChannel, nil)
	urn := urns.URN("tel:+12065551218")

	msg1 := ts.b.NewIncomingMsg(ctx, knChannel, urn, "two regular attachments", "", clog).(*Msg)
	msg1.WithAttachment("http://example.com/test.jpg")
	msg1.WithAttachment("http://example.com/test.m4a")

	// should just write attachments as they are
	err := ts.b.WriteMsg(ctx, msg1, clog)
	ts.NoError(err)
	ts.Equal([]string{"http://example.com/test.jpg", "http://example.com/test.m4a"}, msg1.Attachments())

	// try an embedded attachment
	msg2 := ts.b.NewIncomingMsg(ctx, knChannel, urn, "embedded attachment data", "", clog).(*Msg)
	msg2.WithAttachment(fmt.Sprintf("data:%s", base64.StdEncoding.EncodeToString(test.ReadFile("../../test/testdata/test.jpg"))))

	// should have actually fetched and saved it to storage, with the correct content type
	err = ts.b.WriteMsg(ctx, msg2, clog)
	ts.NoError(err)
	ts.Equal([]string{"image/jpeg:http://localhost:9000/test-attachments/attachments/1/9b95/5e36/9b955e36-ac16-4c6b-8ab6-9b9af5cd042a.jpg"}, msg2.Attachments())

	// try an invalid embedded attachment
	msg3 := ts.b.NewIncomingMsg(ctx, knChannel, urn, "invalid embedded attachment data", "", clog).(*Msg)
	msg3.WithAttachment("data:34564363576573573")

	err = ts.b.WriteMsg(ctx, msg3, clog)
	ts.EqualError(err, "unable to decode attachment data: illegal base64 data at input byte 16")

	// try a geo attachment
	msg4 := ts.b.NewIncomingMsg(ctx, knChannel, urn, "geo attachment", "", clog).(*Msg)
	msg4.WithAttachment("geo:123.234,-45.676")

	// should be saved as is
	err = ts.b.WriteMsg(ctx, msg4, clog)
	ts.NoError(err)
	ts.Equal([]string{"geo:123.234,-45.676"}, msg4.Attachments())
}

func (ts *BackendTestSuite) TestPreferredChannelCheckRole() {
	exChannel := ts.getChannel("EX", "dbc126ed-66bc-4e28-b67b-81dc3327100a")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, exChannel, nil)
	ctx := context.Background()

	// have to round to microseconds because postgres can't store nanos
	now := time.Now().Round(time.Microsecond).In(time.UTC)

	urn := urns.URN("tel:+12065552020")
	msg := ts.b.NewIncomingMsg(ctx, exChannel, urn, "test123", "ext123", clog).WithReceivedOn(now).WithContactName("test contact").(*Msg)

	// try to write it to our db
	err := ts.b.WriteMsg(ctx, msg, clog)
	ts.NoError(err)

	time.Sleep(1 * time.Second)

	// load it back from the id
	m := readMsgFromDB(ts.b, msg.ID())

	tx, err := ts.b.db.Beginx()
	ts.NoError(err)

	// load our URN
	exContactURN, err := getOrCreateContactURN(tx, m.channel, m.ContactID_, urn, nil)
	if !ts.NoError(err) || !ts.NoError(tx.Commit()) {
		ts.FailNow("failed writing contact urn")
	}

	ts.Equal(exContactURN.ChannelID, courier.NilChannelID)
}

func (ts *BackendTestSuite) TestChannelEvent() {
	ctx := context.Background()
	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, channel, nil)
	urn := urns.URN("tel:+12065551616")

	event := ts.b.NewChannelEvent(channel, courier.EventTypeReferral, urn, clog).WithExtra(map[string]string{"ref_id": "12345"}).WithContactName("kermit frog")
	err := ts.b.WriteChannelEvent(ctx, event, clog)
	ts.NoError(err)

	contact, err := contactForURN(ctx, ts.b, channel.OrgID_, channel, urn, nil, "", true, clog)
	ts.NoError(err)
	ts.Equal(null.String("kermit frog"), contact.Name_)

	dbE := event.(*ChannelEvent)
	dbE = readChannelEventFromDB(ts.b, dbE.ID_)
	ts.Equal(dbE.EventType_, courier.EventTypeReferral)
	ts.Equal(map[string]string{"ref_id": "12345"}, dbE.Extra())
	ts.Equal(contact.ID_, dbE.ContactID_)
	ts.Equal(contact.URNID_, dbE.ContactURNID_)

	event = ts.b.NewChannelEvent(channel, courier.EventTypeOptIn, urn, clog).WithExtra(map[string]string{"title": "Polls", "payload": "1"})
	err = ts.b.WriteChannelEvent(ctx, event, clog)
	ts.NoError(err)

	dbE = event.(*ChannelEvent)
	dbE = readChannelEventFromDB(ts.b, dbE.ID_)
	ts.Equal(dbE.EventType_, courier.EventTypeOptIn)
	ts.Equal(map[string]string{"title": "Polls", "payload": "1"}, dbE.Extra())
	ts.Equal(null.Int(1), dbE.OptInID_)
}

func (ts *BackendTestSuite) TestSessionTimeout() {
	ctx := context.Background()

	dates.SetNowFunc(dates.NewSequentialNow(time.Date(2025, 1, 28, 20, 43, 34, 157379218, time.UTC), time.Second))
	defer dates.SetNowFunc(time.Now)

	msgJSON := `{
		"uuid": "54c893b9-b026-44fc-a490-50aed0361c3f",
		"id": 204,
		"org_id": 1,
		"text": "Test message 21",
		"contact_id": 100,
		"contact_urn_id": 14,
		"channel_uuid": "f3ad3eb6-d00d-4dc3-92e9-9f34f32940ba",
		"urn": "telegram:3527065",
		"created_on": "2017-07-21T19:22:23.242757Z",
		"high_priority": true,
		"session": {
			"uuid": "79c1dbc6-4200-4333-b17a-1f996273a4cb",
			"status": "W",
			"sprint_uuid": "0897c392-8b08-43c4-b9d9-e75d332a2c58",
			"timeout": 3600
		},
		"session_id": 12345,
		"session_timeout": 3600,
		"session_modified_on": "2025-01-28T20:43:34.157379218Z"
	}`

	msg := &Msg{}
	jsonx.MustUnmarshal([]byte(msgJSON), msg)

	err := ts.b.insertTimeoutFire(ctx, msg)
	ts.NoError(err)

	assertdb.Query(ts.T(), ts.b.db, `SELECT org_id, contact_id, fire_type, scope, session_uuid::text, sprint_uuid::text FROM contacts_contactfire`).
		Columns(map[string]any{
			"org_id":       int64(1),
			"contact_id":   int64(100),
			"fire_type":    "T",
			"scope":        "",
			"session_uuid": "79c1dbc6-4200-4333-b17a-1f996273a4cb",
			"sprint_uuid":  "0897c392-8b08-43c4-b9d9-e75d332a2c58",
		})

	// if there's a conflict (e.g. in this case trying to add same timeout again), it should be ignored
	err = ts.b.insertTimeoutFire(ctx, msg)
	ts.NoError(err)

	assertdb.Query(ts.T(), ts.b.db, `SELECT count(*) FROM contacts_contactfire`).Returns(1)
}

func (ts *BackendTestSuite) TestMailroomEvents() {
	ctx := context.Background()

	ts.clearValkey()

	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	clog := courier.NewChannelLog(courier.ChannelLogTypeUnknown, channel, nil)
	urn := urns.URN("tel:+12065551616")

	event := ts.b.NewChannelEvent(channel, courier.EventTypeReferral, urn, clog).
		WithExtra(map[string]string{"ref_id": "12345"}).
		WithContactName("kermit frog").
		WithOccurredOn(time.Date(2020, 8, 5, 13, 30, 0, 123456789, time.UTC))
	err := ts.b.WriteChannelEvent(ctx, event, clog)
	ts.NoError(err)

	contact, err := contactForURN(ctx, ts.b, channel.OrgID_, channel, urn, nil, "", true, clog)
	ts.NoError(err)
	ts.Equal(null.String("kermit frog"), contact.Name_)
	ts.False(contact.IsNew_)

	dbE := event.(*ChannelEvent)
	dbE = readChannelEventFromDB(ts.b, dbE.ID_)
	ts.Equal(dbE.EventType_, courier.EventTypeReferral)
	ts.Equal(map[string]string{"ref_id": "12345"}, dbE.Extra())
	ts.Equal(contact.ID_, dbE.ContactID_)
	ts.Equal(contact.URNID_, dbE.ContactURNID_)

	ts.assertQueuedContactTask(contact.ID_, "event_received", map[string]any{
		"event_id":    float64(dbE.ID_),
		"event_type":  "referral",
		"channel_id":  float64(10),
		"urn_id":      float64(contact.URNID_),
		"extra":       map[string]any{"ref_id": "12345"},
		"new_contact": false,
		"occurred_on": "2020-08-05T13:30:00.123456789Z",
	})
}

func (ts *BackendTestSuite) TestResolveMedia() {
	ctx := context.Background()
	rc := ts.b.rp.Get()
	defer rc.Close()

	tcs := []struct {
		url   string
		media courier.Media
		err   string
	}{
		{ // image upload that can be resolved
			url: "http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
			media: &Media{
				UUID_:        "ec6972be-809c-4c8d-be59-ba9dbd74c977",
				Path_:        "/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				ContentType_: "image/jpeg",
				URL_:         "http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				Size_:        123,
				Width_:       1024,
				Height_:      768,
				Alternates_:  []*Media{},
			},
		},
		{ // image upload that can be resolved
			url: "http://nyaruka.us-east-1.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
			media: &Media{
				UUID_:        "ec6972be-809c-4c8d-be59-ba9dbd74c977",
				Path_:        "/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				ContentType_: "image/jpeg",
				URL_:         "http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				Size_:        123,
				Width_:       1024,
				Height_:      768,
				Alternates_:  []*Media{},
			},
		},
		{ // same image upload, this time from cache
			url: "http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
			media: &Media{
				UUID_:        "ec6972be-809c-4c8d-be59-ba9dbd74c977",
				Path_:        "/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				ContentType_: "image/jpeg",
				URL_:         "http://nyaruka.s3.com/orgs/1/media/ec69/ec6972be-809c-4c8d-be59-ba9dbd74c977/test.jpg",
				Size_:        123,
				Width_:       1024,
				Height_:      768,
				Alternates_:  []*Media{},
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
			media: &Media{
				UUID_:        "5310f50f-9c8e-4035-9150-be5a1f78f21a",
				Path_:        "/orgs/1/media/5310/5310f50f-9c8e-4035-9150-be5a1f78f21a/test.mp3",
				ContentType_: "audio/mp3",
				URL_:         "http://nyaruka.s3.com/orgs/1/media/5310/5310f50f-9c8e-4035-9150-be5a1f78f21a/test.mp3",
				Size_:        123,
				Duration_:    500,
				Alternates_: []*Media{
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
	assertvk.HLen(ts.T(), rc, fmt.Sprintf("media-lookups:%s", time.Now().In(time.UTC).Format("2006-01-02")), 3)
}

func (ts *BackendTestSuite) assertNoQueuedContactTask(contactID ContactID) {
	rc := ts.b.rp.Get()
	defer rc.Close()

	assertvk.ZCard(ts.T(), rc, "tasks:handler:1", 0)
	assertvk.ZCard(ts.T(), rc, "tasks:handler:active", 0)
	assertvk.LLen(ts.T(), rc, fmt.Sprintf("c:1:%d", contactID), 0)
}

func (ts *BackendTestSuite) assertQueuedContactTask(contactID ContactID, expectedType string, expectedBody map[string]any) {
	rc := ts.b.rp.Get()
	defer rc.Close()

	assertvk.ZCard(ts.T(), rc, "tasks:handler:1", 1)
	assertvk.ZCard(ts.T(), rc, "tasks:handler:active", 1)
	assertvk.LLen(ts.T(), rc, fmt.Sprintf("c:1:%d", contactID), 1)

	data, err := redis.Bytes(rc.Do("LPOP", fmt.Sprintf("c:1:%d", contactID)))
	ts.NoError(err)

	// created_on is usually DB time so exclude it from task body comparison
	data = jsonparser.Delete(data, "task", "created_on")

	var body map[string]any
	jsonx.MustUnmarshal(data, &body)
	ts.Equal(expectedType, body["type"])
	ts.Equal(expectedBody, body["task"])
}

func TestMsgSuite(t *testing.T) {
	suite.Run(t, new(BackendTestSuite))
}

func TestBackendSuite(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}

type ServerTestSuite struct {
	suite.Suite
}

// for testing only, returned DBMsg object is not fully populated
func readMsgFromDB(b *backend, id courier.MsgID) *Msg {
	m := &Msg{
		ID_: id,
	}
	err := b.db.Get(m, sqlSelectMsg, id)
	if err != nil {
		panic(err)
	}

	ch := &Channel{
		ID_: m.ChannelID_,
	}
	err = b.db.Get(ch, selectChannelSQL, m.ChannelID_)
	if err != nil {
		panic(err)
	}

	m.channel = ch
	return m
}

const sqlSelectMsg = `
SELECT
	org_id,
	direction,
	text,
	attachments,
	msg_count,
	error_count,
	failed_reason,
	high_priority,
	status,
	visibility,
	external_id,
	channel_id,
	contact_id,
	contact_urn_id,
	created_on,
	modified_on,
	next_attempt,
	sent_on,
	log_uuids
FROM
	msgs_msg
WHERE
	id = $1`

const selectChannelSQL = `
SELECT
	org_id,
	ch.id as id,
	ch.uuid as uuid,
	ch.name as name,
	channel_type, schemes,
	address, role,
	ch.country as country,
	ch.config as config,
	org.config as org_config,
	org.is_anon as org_is_anon
FROM
	channels_channel ch
	JOIN orgs_org org on ch.org_id = org.id
WHERE
    ch.id = $1
`

const sqlSelectEvent = `
SELECT id, uuid, org_id, channel_id, contact_id, contact_urn_id, event_type, optin_id, extra, occurred_on, created_on, log_uuids
  FROM channels_channelevent
 WHERE id = $1`

func readChannelEventFromDB(b *backend, id ChannelEventID) *ChannelEvent {
	e := &ChannelEvent{}
	err := b.db.Get(e, sqlSelectEvent, id)
	if err != nil {
		panic(err)
	}
	return e
}

// convenience way to call a func and panic if it errors, e.g. must(foo())
func must(err error) {
	if err != nil {
		panic(err)
	}
}

// if just checking an error is nil noError(err) reads better than must(err)
var noError = must
