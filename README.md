# Courier [![Build Status](https://travis-ci.org/nyaruka/courier.svg?branch=master)](https://travis-ci.org/nyaruka/courier) [![codecov](https://codecov.io/gh/nyaruka/courier/branch/master/graph/badge.svg)](https://codecov.io/gh/nyaruka/courier) [![Go Report Card](https://goreportcard.com/badge/github.com/nyaruka/courier)](https://goreportcard.com/report/github.com/nyaruka/courier) 

Install Courier in your workspace with:

```
go get github.com/nyaruka/courier
```

Build Courier with:

```
go install github.com/nyaruka/courier/cmd/...
```

This will create a new executable in $GOPATH/bin called `courier`. 

To run the tests you need to create the test database:

```
$ createdb courier_test
$ createuser -P -E courier
$ psql -d courier_test -f backends/rapidpro/schema.sql
$ psql -d courier_test -c "GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO courier;"
$ psql -d courier_test -c "GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO courier;"
```

To run all of the tests including benchmarks:

```
go test $(go list ./... | grep -v /vendor/) -cover -bench=.
```
