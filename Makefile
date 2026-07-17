benchmarks:
	@(cd benchmarks/ && go test -bench=. -benchmem -benchtime=1s ./...)
.PHONY: benchmarks

test:
	@go test \
		-count=1 \
		-shuffle=on \
		-short \
		-timeout=5m \
		-vet=all \
		./...
.PHONY: test

test-acc:
	@go test \
		-count=1 \
		-shuffle=on \
		-race \
		-timeout=10m \
		-vet=all \
		./...
.PHONY: test-acc
