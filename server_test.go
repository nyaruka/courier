package courier

import (
	"testing"

	"github.com/nyaruka/courier/config"
	"github.com/stretchr/testify/suite"
)

type ServerTestSuite struct {
	suite.Suite
}

type configTestCase struct {
	config        config.Courier
	expectedError string
}

var invalidConfigTestCases = []configTestCase{
	{config: config.Courier{DB: ":foo"}, expectedError: "unable to parse DB URL"},
	{config: config.Courier{DB: "mysql:test"}, expectedError: "only postgres is supported"},
	{config: config.Courier{DB: testDatabaseURL, Redis: ":foo"}, expectedError: "unable to parse Redis URL"},
}

func (ts *ServerTestSuite) TestInvalidConfigs() {
	for _, testCase := range invalidConfigTestCases {
		err := NewServer(&testCase.config).Start()
		ts.Contains(err.Error(), testCase.expectedError)
	}
}

func TestServerSuite(t *testing.T) {
	suite.Run(t, new(ServerTestSuite))
}
