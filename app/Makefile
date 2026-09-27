.PHONY: build run dev test smoke fmt vet clean

build:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
	go build -tags lambda.norpc -trimpath -ldflags="-s -w" \
			 -o "bin/bootstrap" ./cmd/skipli

run:
	go run ./cmd/skipli

dev:
	find . -name '*.go' | entr -rcn go run ./cmd/skipli

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin
