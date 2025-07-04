# 🛫 Courier

[![tag](https://img.shields.io/github/tag/nyaruka/courier.svg)](https://github.com/nyaruka/courier/releases)
[![Build Status](https://github.com/nyaruka/courier/workflows/CI/badge.svg)](https://github.com/nyaruka/courier/actions?query=workflow%3ACI) 
[![codecov](https://codecov.io/gh/nyaruka/courier/branch/main/graph/badge.svg)](https://codecov.io/gh/nyaruka/courier)
[![Go Report Card](https://goreportcard.com/badge/github.com/nyaruka/courier)](https://goreportcard.com/report/github.com/nyaruka/courier)

Courier is a messaging gateway for text-based messaging channels. It abstracts out various different
texting mediums and providers, allowing applications to focus on the creation and processing of those messages.

Currently it supports over 50 different channel types, ranging for SMS aggregators like Twilio to
IP channels like Facebook and Telegram messenger. The goal is for Courier to support all popular
messaging channels and aggregators and we are happy to accept pull requests to help accomplish that.

Courier is currently used to power [RapidPro](https://rapidpro.io) and [TextIt](https://textit.com)
but the backend is pluggable, so you can add your own backend to read and write messages.

## Deploying

As a Go application, it compiles to a binary and that binary along with the config file is all
you need to run it on your server. You can find bundles for each platform in the
[releases directory](https://github.com/nyaruka/courier/releases). We recommend running it
behind a reverse proxy such as nginx or Elastic Load Balancer that provides HTTPs encryption.

## Configuration

The service uses a tiered configuration system, each option takes precendence over the ones above it:

 1. The configuration file
 2. Environment variables starting with `COURIER_` 
 3. Command line parameters

We recommend running courier with no changes to the configuration and no parameters, using only
environment variables to configure it. You can use `% courier --help` to see a list of the
environment variables and parameters and for more details on each option.

 * `COURIER_DOMAIN`: The root domain which courier is exposed as (ex `textit.in`)
 * `COURIER_SPOOL_DIR`: A local path where courier can spool files if the database is down, should be writable. (ex: `/home/courier/spool`)
 * `COURIER_DB`: Details parameters used to connect to the Postgres RapidPro database (ex: `postgres://textit:fooman@rds.courier.io/5432/textit`)
 * `COURIER_VALKEY`: Details parameters to use to connect to Valkey RapidPro database (ex: `valkey://valkey.courier.io:6379/13`)
 * `COURIER_AUTH_TOKEN`: authentication token to require for requests from Mailroom

### AWS services:

 * `COURIER_AWS_ACCESS_KEY_ID`: AWS access key id used to authenticate to AWS
 * `COURIER_AWS_SECRET_ACCESS_KEY`: AWS secret access key used to authenticate to AWS
 * `COURIER_AWS_REGION`: AWS region (ex: `eu-west-1`)
 * `COURIER_S3_ATTACHMENTS_BUCKET`: name of your S3 bucket (ex: `rp-attachments`)

### Logging and error reporting:

 * `COURIER_DEPLOYMENT_ID`: used for metrics reporting
 * `COURIER_SENTRY_DSN`: DSN to use when logging errors to Sentry
 * `COURIER_LOG_LEVEL`: logging level to use (default is `warn`)

## Development

Once you've checked out the code, you can build it with:

```
go install github.com/nyaruka/courier/cmd/courier
```

This will create a new executable in $GOPATH/bin called `courier`. 

To run the tests you need to create the test database:

```
$ createdb courier_test
$ createuser -P -E courier_test
$ psql -d courier_test -f backends/rapidpro/schema.sql
$ psql -d courier_test -c "GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO courier;"
$ psql -d courier_test -c "GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO courier;"
```

To run all of the tests including benchmarks:

```
go test ./... -p=1 -bench=.
```
