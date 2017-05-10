# Courier

[![Build Status](https://travis-ci.org/nyaruka/courier.svg?branch=master)](https://travis-ci.org/nyaruka/courier)
[![Coverage Status](https://coveralls.io/repos/github/nyaruka/courier/badge.svg?branch=master)](https://coveralls.io/github/nyaruka/courier?branch=master)

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
$ psql -d courier_test -f schema.sql
$ psql -d courier_test -c "GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO courier;"
$ psql -d courier_test -c "GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO courier;"
```

To run all of the tests including benchmarks:

```
go test $(go list ./... | grep -v /vendor/) -cover -bench=.
```
