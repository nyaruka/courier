package rapidpro

import (
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	"github.com/nyaruka/courier"
	"github.com/nyaruka/courier/config"
	"github.com/stretchr/testify/suite"
)

var testConfig = config.Courier{
	Backend: "rapidpro",
	DB:      "postgres://courier@localhost/courier_test?sslmode=disable",
	Redis:   "redis://localhost:6379/10",
}

type MsgTestSuite struct {
	suite.Suite
	b *backend
}

func (ts *MsgTestSuite) SetupSuite() {
	b, _ := courier.NewBackend(&testConfig)
	ts.b = b.(*backend)

	err := ts.b.Start()
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

func (ts *MsgTestSuite) TestCheckMsgExists() {
	channelUUID, _ := courier.NewChannelUUID("dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	channel, err := ts.b.GetChannel(courier.ChannelType("KN"), channelUUID)
	if err != nil {
		ts.FailNow("Error getting channel: ", err.Error())
	}

	// check with invalid message id
	err = checkMsgExists(ts.b, courier.NewStatusUpdateForID(channel, courier.NewMsgID(-1), courier.MsgStatus("S")))
	ts.Equal(err, courier.ErrMsgNotFound)

	// check with valid message id
	err = checkMsgExists(ts.b, courier.NewStatusUpdateForID(channel, courier.NewMsgID(104), courier.MsgStatus("S")))
	ts.Nil(err)

	// check with invalid external id
	err = checkMsgExists(ts.b, courier.NewStatusUpdateForExternalID(channel, "ext-invalid", courier.MsgStatus("S")))
	ts.Equal(err, courier.ErrMsgNotFound)

	// check with valid external id
	status := courier.NewStatusUpdateForExternalID(channel, "ext1", courier.MsgStatus("S"))
	err = checkMsgExists(ts.b, status)
	ts.Nil(err)
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
