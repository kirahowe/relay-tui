.PHONY: build check update

build:
	go build -o bin/relay-tui ./cmd/relay-tui

check:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }
	go vet ./...
	go test -race -count=1 ./...
	# The surface rules are rare-sequence properties: run them deep.
	go test -count=1 -run TestSurfaceRules -rapid.checks=1000 ./internal/ui

# Keep dependencies current.
update:
	go get -u ./...
	go mod tidy
	$(MAKE) check
