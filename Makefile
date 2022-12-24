BINARY=simplevisor

$(BINARY): $(shell find . -name '*.go') go.sum
	go generate ./...
	CGO_ENABLED=0 go build \
		-ldflags "-X main.appVersion=$(shell git describe --long --tags --dirty --always)"  \
		-o $@

go.sum: go.mod
	go mod tidy

.PHONY: vet
vet: main.go
	go vet $<

.PHONY: test
test: $(BINARY)
	go test ./...

.PHONY: cover
cover: $(BINARY)
	rm coverage.out || true
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

.PHONY: clean
clean:
	rm \
		$(BINARY) \
		supervise/zzz_syscall_map.go \
		coverage.out \
	|| true
