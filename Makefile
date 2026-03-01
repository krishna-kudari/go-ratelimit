.PHONY: test bench lint fmt vet ci clean

test:
	go test -race -count=1 ./...

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
