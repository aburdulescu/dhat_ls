.PHONY: all
all: vet lint test build

.PHONY: ci
ci: vet test build

.PHONY: build
build:
	go build

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test -cover ./...

.PHONY: lint
lint:
	which golangci-lint || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run

.PHONY: coverage
coverage: test
	go tool cover -html=cov.out -o=cov.html

.PHONY: release
release: all
	which goreleaser || go install github.com/goreleaser/goreleaser@latest
	goreleaser release --snapshot --clean
