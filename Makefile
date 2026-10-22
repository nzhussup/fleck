.PHONY: build test vet race fmt

build:
	go build -o fleck ./cmd/fleck

test:
	go test ./...
	cd examples/payment-service && go test ./...

vet:
	go vet ./...
	cd examples/payment-service && go vet ./...

race:
	go test -race ./...

fmt:
	gofmt -w cmd internal examples/payment-service/cmd evaluation/fixtures/go
