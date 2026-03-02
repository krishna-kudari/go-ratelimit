.PHONY: test test-json test-short bench lint fmt vet ci clean setup tools

# ─── Setup ────────────────────────────────────────────────────────────────────

setup: tools
	git config core.hooksPath .githooks
	@echo "Git hooks installed from .githooks/"

tools:
	go install gotest.tools/gotestsum@latest

# ─── Test ─────────────────────────────────────────────────────────────────────

test:
	gotestsum --format testdox -- -race -count=1 ./...

test-json:
	gotestsum --jsonfile test-output.json --format testdox -- -race -count=1 ./...
	@echo "JSON report → test-output.json"

test-short:
	gotestsum --format testdox -- -race -count=1 -short ./...

test-v:
	gotestsum --format standard-verbose -- -race -count=1 ./...

# ─── Quality ──────────────────────────────────────────────────────────────────

bench:
	go test -bench=. -benchmem -count=1 -run=^$$ ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -s -w .

vet:
	go vet ./...

ci: fmt vet lint test

clean:
	go clean -testcache
	rm -f test-output.json
