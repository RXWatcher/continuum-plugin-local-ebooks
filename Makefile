BINARY := silo-plugin-local-ebooks
GO ?= go

.PHONY: build test fmt clean
build:
	$(GO) build -o $(BINARY) ./cmd/silo-plugin-local-ebooks
test:
	$(GO) test ./...
fmt:
	$(GO) fmt ./...
clean:
	rm -f $(BINARY)
