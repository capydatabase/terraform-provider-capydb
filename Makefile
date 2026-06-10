BINARY  := terraform-provider-capydb
VERSION ?= dev

.PHONY: build install test vet lint fmt check release-snapshot

build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o bin/$(BINARY) .

install:
	go install -ldflags "-X main.version=$(VERSION)" .

test:
	go test -race ./...

vet:
	go vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed; falling back to go vet"; \
		go vet ./...; \
	fi

fmt:
	gofmt -w .

check: fmt vet lint test

release-snapshot:
	goreleaser release --snapshot --clean
