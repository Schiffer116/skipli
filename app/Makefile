.PHONY: build run dev test smoke fmt vet clean

build:
	go build -o bin/skipli ./cmd/skipli

run:
	go run ./cmd/skipli

dev:
	./scripts/dev.sh

test:
	go test ./...

smoke:
	./scripts/test.sh $(ARGS)

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin
