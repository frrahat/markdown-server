# Usage:
#   make run ROOT=~/notes [ADDR=127.0.0.1:9090] [FLAGS=-unsafe-html]
#   make build
#   make test

ROOT  ?= .
# Empty means the binary's own default (127.0.0.1:8642), kept in one place.
ADDR  ?=
FLAGS ?=
BIN   := bin/mdserver

.PHONY: run build test check clean

run:
	go run ./cmd/mdserver $(if $(ADDR),-addr $(ADDR)) $(FLAGS) $(ROOT)

# Always invokes go build; Go's build cache makes it a no-op when nothing changed.
build:
	go build -o $(BIN) ./cmd/mdserver

test:
	go test -race ./...

# The pre-commit bar from SPEC.md: formatting, vet, and race-enabled tests.
check:
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	go test -race ./...

clean:
	rm -rf bin
