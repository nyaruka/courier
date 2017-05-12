package courier

import (
	"fmt"
	"io/ioutil"
	"log"
	"testing"

	"github.com/stretchr/testify/suite"
)

type MsgTestSuite struct {
	suite.Suite
	s *server
}

func (ts *MsgTestSuite) SetupSuite() {
	ts.s = NewServer(&testConfig).(*server)
	err := ts.s.Start()
	if err != nil {
		log.Fatalf("unable to start server for testing: %v", err)
	}

	// read our testdata sql
	sql, err := ioutil.ReadFile("testdata.sql")
	if err != nil {
		panic(fmt.Errorf("Unable to read testdata.sql: %s", err))
	}
	ts.s.db.MustExec(string(sql))
}

func (ts *MsgTestSuite) TearDownSuite() {
	ts.s.Stop()
}

func (ts *MsgTestSuite) TestCheckMsgExists() {
	channel, err := ChannelFromUUID(ts.s, ChannelType("KN"), "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	if err != nil {
		ts.FailNow("Error getting channel: ", err.Error())
	}

	// check with invalid message id
	err = checkMsgExists(ts.s, NewStatusUpdateForID(channel, "-1", MsgStatus("S")))
	ts.Equal(err, ErrMsgNotFound)

	// check with valid message id
	err = checkMsgExists(ts.s, NewStatusUpdateForID(channel, "104", MsgStatus("S")))
	ts.Nil(err)

	// check with invalid external id
	err = checkMsgExists(ts.s, NewStatusUpdateForExternalID(channel, "ext-invalid", MsgStatus("S")))
	ts.Equal(err, ErrMsgNotFound)

	// check with valid external id
	status := NewStatusUpdateForExternalID(channel, "ext1", MsgStatus("S"))
	err = checkMsgExists(ts.s, status)
	ts.Nil(err)

	// check with neither kind of id
	status.clear()
	err = checkMsgExists(ts.s, status)
	ts.EqualError(err, "no id or external id for status update")
}

func TestMsgSuite(t *testing.T) {
	suite.Run(t, new(MsgTestSuite))
}
