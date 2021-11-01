build:
	go build ./cmd/courier

test:
	go test -p=1 -coverprofile=coverage.text -covermode=atomic ./...

test-cover:
	make test
	go tool cover -html=coverage.text

test-cover-total:
	make test
	go tool cover -func coverage.text | grep total | awk '{print $3}'