package courier

import (
	"fmt"
	"log"
	"testing"

	"github.com/stretchr/testify/suite"
)

var insertChannelSQL = `INSERT INTO channels_channel(uuid, channel_type, address, country, config, is_active) VALUES($1, $2, $3, $4, $5, TRUE) RETURNING id`
var insertMsgSQL = `INSERT INTO msgs_msg(uuid, external_id, channel_id) VALUES($1, $2, $3) RETURNING id`

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

	ts.s.db.Exec("DELETE FROM msgs_msg")
	ts.s.db.Exec("DELETE FROM channels_channel")
}

func (ts *MsgTestSuite) TearDownSuite() {
	ts.s.Stop()
}

func (ts *MsgTestSuite) TestCheckMsgExists() {
	row := ts.s.db.QueryRow(insertChannelSQL, "dbc126ed-66bc-4e28-b67b-81dc3327c95d", "KN", "12345", "RW", "{}")
	var ch1Id int
	row.Scan(&ch1Id)

	row = ts.s.db.QueryRow(insertMsgSQL, "de4e333b-1111-4fa6-be65-a355df933035", "ext1", ch1Id)
	var msg1Id int
	row.Scan(&msg1Id)

	channel, err := ChannelFromUUID(ts.s, ChannelType("KN"), "dbc126ed-66bc-4e28-b67b-81dc3327c95d")
	if err != nil {
		ts.FailNow("Error getting channel: ", err.Error())
	}

	// check with invalid message id
	err = checkMsgExists(ts.s, NewStatusUpdateForID(channel, "-1", MsgStatus("S")))
	ts.Equal(err, ErrMsgNotFound)

	// check with valid message id
	err = checkMsgExists(ts.s, NewStatusUpdateForID(channel, fmt.Sprint(msg1Id), MsgStatus("S")))
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
