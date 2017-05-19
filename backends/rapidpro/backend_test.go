package rapidpro

import (
	"fmt"
	"io/ioutil"
	"log"
	"testing"
	"time"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/config"
	"github.com/stretchr/testify/suite"
)

type MsgTestSuite struct {
	suite.Suite
	b *backend
}

func testConfig() *config.Courier {
	config := config.NewTest()
	config.DB = "postgres://courier@localhost/courier_test?sslmode=disable"
	config.Redis = "redis://localhost:6379/0"
	return config
}

func (ts *MsgTestSuite) SetupSuite() {
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
}

func (ts *MsgTestSuite) TearDownSuite() {
	ts.b.Stop()
}

func (ts *MsgTestSuite) getChannel(cType string, cUUID string) *DBChannel {
	channelUUID, err := courier.NewChannelUUID(cUUID)
	ts.NoError(err, "error building channel uuid")

	channel, err := ts.b.GetChannel(courier.ChannelType(cType), channelUUID)
	ts.NoError(err, "error building channel uuid")

	return channel.(*DBChannel)
}

func (ts *MsgTestSuite) TestCheckMsgExists() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	// check with invalid message id
	err := checkMsgExists(ts.b, courier.NewStatusUpdateForID(knChannel, courier.NewMsgID(-1), courier.MsgStatus("S")))
	ts.Equal(err, courier.ErrMsgNotFound)

	// check with valid message id
	err = checkMsgExists(ts.b, courier.NewStatusUpdateForID(knChannel, courier.NewMsgID(10000), courier.MsgStatus("S")))
	ts.Nil(err)

	// check with invalid external id
	err = checkMsgExists(ts.b, courier.NewStatusUpdateForExternalID(knChannel, "ext-invalid", courier.MsgStatus("S")))
	ts.Equal(err, courier.ErrMsgNotFound)

	// check with valid external id
	status := courier.NewStatusUpdateForExternalID(knChannel, "ext1", courier.MsgStatus("S"))
	err = checkMsgExists(ts.b, status)
	ts.Nil(err)
}

func (ts *MsgTestSuite) TestContact() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	urn := courier.NewTelURNForCountry("12065551518", "US")

	now := time.Now()

	// create our new contact
	contact, err := contactForURN(ts.b.db, knChannel.OrgID(), knChannel.ID(), urn, "Ryan Lewis")
	ts.NoError(err)

	now2 := time.Now()

	// load this contact again by URN, should be same contact, name unchanged
	contact2, err := contactForURN(ts.b.db, knChannel.OrgID(), knChannel.ID(), urn, "Other Name")
	ts.NoError(err)

	ts.Equal(contact.UUID, contact2.UUID)
	ts.Equal(contact.ID, contact2.ID)
	ts.Equal(knChannel.OrgID(), contact2.OrgID)
	ts.Equal("Ryan Lewis", contact2.Name)
	ts.True(contact2.ModifiedOn.After(now))
	ts.True(contact2.CreatedOn.After(now))
	ts.True(contact2.ModifiedOn.Before(now2))
	ts.True(contact2.CreatedOn.Before(now2))
}

func (ts *MsgTestSuite) TestContactURN() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	twChannel := ts.getChannel("TW", "dbc126ed-66bc-4e28-b67b-81dc3327c96a")
	urn := courier.NewTelURNForCountry("12065551515", "US")

	contact, err := contactForURN(ts.b.db, knChannel.OrgID_, knChannel.ID_, urn, "")
	ts.NoError(err)

	// first build a URN for our number with the kannel channel
	knURN, err := contactURNForURN(ts.b.db, knChannel.OrgID_, knChannel.ID_, contact.ID, urn)
	ts.Equal(knURN.OrgID, knChannel.OrgID_)
	ts.NoError(err)

	// then with our twilio channel
	twURN, err := contactURNForURN(ts.b.db, twChannel.OrgID_, twChannel.ID_, contact.ID, urn)
	ts.NoError(err)

	// should be the same URN
	ts.Equal(knURN.ID, twURN.ID)

	// same contact
	ts.Equal(knURN.ContactID, twURN.ContactID)

	// and channel should be set to twitter
	ts.Equal(twURN.ChannelID, twChannel.ID())
}

func (ts *MsgTestSuite) TestStatus() {
	channel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	now := time.Now().In(time.UTC)

	// update by id
	status := courier.NewStatusUpdateForID(channel, courier.NewMsgID(10001), courier.MsgSent)
	err := ts.b.WriteMsgStatus(status)
	ts.NoError(err)
	m, err := readMsgFromDB(ts.b, courier.NewMsgID(10001))
	ts.NoError(err)
	ts.Equal(m.Status, courier.MsgSent)
	ts.True(m.ModifiedOn.After(now))

	// update by external id
	status = courier.NewStatusUpdateForExternalID(channel, "ext1", courier.MsgFailed)
	err = ts.b.WriteMsgStatus(status)
	ts.NoError(err)
	m, err = readMsgFromDB(ts.b, courier.NewMsgID(10000))
	ts.NoError(err)
	ts.Equal(m.Status, courier.MsgFailed)
	ts.True(m.ModifiedOn.After(now))

	// no such external id
	status = courier.NewStatusUpdateForExternalID(channel, "ext2", courier.MsgSent)
	err = ts.b.WriteMsgStatus(status)
	ts.Error(err)
}

func (ts *MsgTestSuite) TestChannel() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	ts.Equal("2500", knChannel.Address())
	ts.Equal("RW", knChannel.Country())

	// assert our config values
	val := knChannel.ConfigForKey("use_national", false)
	boolVal, isBool := val.(bool)
	ts.True(isBool)
	ts.True(boolVal)

	val = knChannel.ConfigForKey("encoding", "default")
	stringVal, isString := val.(string)
	ts.True(isString)
	ts.Equal("smart", stringVal)

	// missing value
	val = knChannel.ConfigForKey("missing", "missingValue")
	stringVal, isString = val.(string)
	ts.True(isString)
	ts.Equal("missingValue", val)
}

func (ts *MsgTestSuite) TestWriteMsg() {
	knChannel := ts.getChannel("KN", "dbc126ed-66bc-4e28-b67b-81dc3327c95d")

	// have to round to microseconds because postgres can't store nanos
	now := time.Now().Round(time.Microsecond).In(time.UTC)

	// create a new courier msg
	urn := courier.NewTelURNForChannel("12065551212", knChannel)
	msg := courier.NewIncomingMsg(knChannel, urn, "test123").WithExternalID("ext123").WithReceivedOn(now).WithContactName("test contact")

	// try to write it to our db
	err := ts.b.WriteMsg(msg)
	ts.NoError(err)

	// check we had an id set
	ts.NotZero(msg.ID)

	// load it back from the id
	m, err := readMsgFromDB(ts.b, msg.ID)
	ts.NoError(err)

	// load our URN
	contactURN, err := contactURNForURN(ts.b.db, m.OrgID, m.ChannelID, m.ContactID, urn)
	ts.NoError(err)

	// make sure our values are set appropriately
	ts.Equal(knChannel.ID_, m.ChannelID)
	ts.Equal(knChannel.OrgID_, m.OrgID)
	ts.Equal(contactURN.ContactID, m.ContactID)
	ts.Equal(contactURN.ID, m.ContactURNID)
	ts.Equal(MsgIncoming, m.Direction)
	ts.Equal(courier.MsgPending, m.Status)
	ts.Equal(DefaultPriority, m.Priority)
	ts.Equal("ext123", m.ExternalID)
	ts.Equal("test123", m.Text)
	ts.Equal([]string(nil), m.Attachments)
	ts.Equal(1, m.MessageCount)
	ts.Equal(0, m.ErrorCount)
	ts.Equal(now, m.SentOn.In(time.UTC))
	ts.NotNil(m.NextAttempt)
	ts.NotNil(m.CreatedOn)
	ts.NotNil(m.ModifiedOn)
	ts.NotNil(m.QueuedOn)

	contact, err := contactForURN(ts.b.db, m.OrgID, m.ChannelID, urn, "")
	ts.Equal("test contact", contact.Name)
	ts.Equal(m.OrgID, contact.OrgID)
	ts.Equal(m.ContactID, contact.ID)
	ts.NotNil(contact.UUID)
	ts.NotNil(contact.ID)
}

func TestMsgSuite(t *testing.T) {
	suite.Run(t, new(MsgTestSuite))
}

var invalidConfigTestCases = []struct {
	config        config.Courier
	expectedError string
}{
	{config: config.Courier{DB: ":foo"}, expectedError: "unable to parse DB URL"},
	{config: config.Courier{DB: "mysql:test"}, expectedError: "only postgres is supported"},
	{config: config.Courier{DB: "postgres://courier@localhost/courier", Redis: ":foo"}, expectedError: "unable to parse Redis URL"},
}

func (ts *ServerTestSuite) TestInvalidConfigs() {
	for _, testCase := range invalidConfigTestCases {
		config := &testCase.config
		config.Backend = "rapidpro"
		backend := newBackend(config)
		err := backend.Start()
		ts.Contains(err.Error(), testCase.expectedError)
	}
}

func TestBackendSuite(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}

type ServerTestSuite struct {
	suite.Suite
}
