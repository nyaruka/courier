# Gocommon [![Build Status](https://travis-ci.org/nyaruka/gocommon.svg?branch=master)](https://travis-ci.org/nyaruka/gocommon) [![Coverage Status](https://coveralls.io/repos/github/nyaruka/gocommon/badge.svg?branch=master)](https://coveralls.io/github/nyaruka/gocommon?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/nyaruka/gocommon)](https://goreportcard.com/report/github.com/nyaruka/gocommon)

Common functionality in goflow and courier.

## Running Tests

You can run all the tests (excluding tests in vendor packages) with:

```
% go test $(go list ./... | grep -v /vendor/)
```
