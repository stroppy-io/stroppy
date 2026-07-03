.PHONY: test bench lint allocgate cli

test: # Run tests with the race detector
	go test -race ./...

cli: # Build the stroppy2 CLI to build/stroppy2
	go build -o build/stroppy2 ./cmd/stroppy2

bench: # Run benchmarks, no unit tests
	go test -bench=. -benchmem -run=^$$ ./...

lint: # Run golangci-lint
	golangci-lint run

allocgate: # Run every alloc-gate test (all named TestAllocs*), race-free
	go test -run '^TestAllocs' ./...
