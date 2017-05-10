# Courier

This is a private repo so need to configure git to use token authentication for HTTPS requests. 
Create a github auth token and set it as an environment variable called `GITHUB_TOKEN`. Then run:

```
git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/".insteadOf "https://github.com/"
```

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
