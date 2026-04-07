.PHONY: fmt vet test test-race lint vulncheck build build-cli build-daemon tidy coverage check

fmt:
	gofmt -w .

vet:
	go vet ./...

test:
	go test ./...

coverage:
	./scripts/check_coverage.sh 85.0

test-race:
	go test -race ./...

lint:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	staticcheck ./...

vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

build:
	go build ./...

build-cli:
	go build -o bin/goddgs ./cmd/goddgs

build-daemon:
	go build -o bin/goddgsd ./cmd/goddgsd

tidy:
	go mod tidy

check: fmt vet test coverage build
