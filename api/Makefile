.PHONY: build run dev test smoke fmt vet clean

run dev: export AWS_PROFILE ?= lambda-role
run dev: export AWS_REGION ?= $(shell terraform -chdir=../infra output -raw region)
run dev: export TABLE_NAME ?= Skipli
run dev: export USER_POOL_ID ?= $(shell terraform -chdir=../infra output -raw user_pool_id)
run dev: export USER_POOL_CLIENT_ID ?= $(shell terraform -chdir=../infra output -raw user_pool_client_id)

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
