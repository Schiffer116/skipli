.PHONY: build run dev test smoke fmt vet clean FORCE

run dev: export AWS_PROFILE ?= lambda-role
run dev: export AWS_REGION ?= $(shell terraform -chdir=../infra output -raw region)
run dev: export TABLE_NAME ?= Skipli
run dev: export USER_POOL_ID ?= $(shell terraform -chdir=../infra output -raw user_pool_id)
run dev: export USER_POOL_CLIENT_ID ?= $(shell terraform -chdir=../infra output -raw user_pool_client_id)

# One Lambda binary per API, each in its own folder since Lambda wants the
# file named bootstrap.
build: bin/http/bootstrap bin/realtime/bootstrap

bin/%/bootstrap: FORCE
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
	go build -tags lambda,lambda.norpc -trimpath -ldflags="-s -w" \
			 -o "$@" ./cmd/$*

FORCE:

run:
	go run ./cmd/http

dev:
	find . -name '*.go' | entr -rcn go run ./cmd/http

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin
