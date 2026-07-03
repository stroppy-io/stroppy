.PHONY: test bench lint allocgate

test: # Run tests with the race detector
	go test -race ./...

bench: # Run benchmarks, no unit tests
	go test -bench=. -benchmem -run=^$$ ./...

lint: # Run golangci-lint
	golangci-lint run

allocgate: # Run every alloc-gate test (all named TestAllocs*), race-free
	go test -run '^TestAllocs' ./...
