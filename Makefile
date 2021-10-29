build:
	go build ./cmd/courier

test:
	go test -p=1 -covermode=atomic -coverprofile=coverage.text ./...

test-cover:
	make test
	go tool cover -html=coverage.text
