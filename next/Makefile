.PHONY: test bench lint allocgate

test: # Run tests with the race detector
	go test -race ./...

bench: # Run benchmarks, no unit tests
	go test -bench=. -benchmem -run=^$$ ./...

lint: # Run golangci-lint
	golangci-lint run

allocgate: # Run alloc-gate tests (name-filtered; ordinary go test underneath)
	go test -run TestAllocs -tags allocgate ./...
