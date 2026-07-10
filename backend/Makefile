.PHONY: build generate test test-unit test-integration test-e2e

LITE_UNIT_TEST_PACKAGES := \
	./ent/schema \
	./internal/config \
	./internal/domain \
	./internal/handler/dto \
	./internal/handler/quotaview \
	./internal/middleware \
	./internal/payment \
	./internal/pkg/anthropicfp \
	./internal/pkg/apicompat \
	./internal/pkg/errors \
	./internal/pkg/gemini \
	./internal/pkg/geminicli \
	./internal/pkg/googleapi \
	./internal/pkg/httpclient \
	./internal/pkg/httputil \
	./internal/pkg/ip \
	./internal/pkg/logger \
	./internal/pkg/oauth \
	./internal/pkg/openai \
	./internal/pkg/openai_compat \
	./internal/pkg/pagination \
	./internal/pkg/proxyurl \
	./internal/pkg/proxyutil \
	./internal/pkg/response \
	./internal/pkg/timezone \
	./internal/pkg/tlsfingerprint \
	./internal/pkg/usagestats \
	./internal/pkg/xai \
	./internal/server/routes \
	./internal/service/openai_ws_v2 \
	./internal/setup \
	./internal/util/logredact \
	./internal/util/responseheaders \
	./internal/util/urlvalidator \
	./migrations

LITE_INTEGRATION_TEST_PACKAGES := \
	./internal/service/openai_ws_v2 \
	./migrations

VERSION ?= $(shell ./scripts/resolve-version.sh)
LDFLAGS ?= -s -w -X main.Version=$(VERSION)

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -trimpath -o bin/server ./cmd/server

generate:
	go generate ./ent
	go generate ./cmd/server

test:
	go test $(LITE_UNIT_TEST_PACKAGES)
	golangci-lint run ./...

test-unit:
	go test -tags=unit $(LITE_UNIT_TEST_PACKAGES)

test-integration:
	go test -tags=integration $(LITE_INTEGRATION_TEST_PACKAGES)

test-e2e:
	./scripts/e2e-test.sh

test-e2e-local:
	go test -tags=e2e -v -timeout=300s ./internal/integration/...
