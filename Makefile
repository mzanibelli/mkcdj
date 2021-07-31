all: tidy lint test install

tidy:
	go mod tidy

lint:
	golangci-lint run ./...

test:
	go test -race ./...

install:
	go install cmd/mkcdj.go
