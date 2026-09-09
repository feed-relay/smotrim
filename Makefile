.DEFAULT_GOAL := help

# Get the latest commit branch, hash, and date
TAG=$(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
BRANCH=$(if $(TAG),$(TAG),$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null))
HASH=$(shell git rev-parse --short=7 HEAD 2>/dev/null)
TIMESTAMP=$(shell git log -1 --format=%ct HEAD 2>/dev/null | xargs -I{} date -u -r {} +%Y%m%dT%H%M%S)
GIT_REV=$(shell printf "%s-%s-%s" "$(BRANCH)" "$(HASH)" "$(TIMESTAMP)")
REV=$(if $(filter --,$(GIT_REV)),latest,$(GIT_REV))

.PHONY: upd version help \
		generate test race \
		fmt fmt-check vet lint check

version: ## show git version information
	@echo "branch: $(BRANCH), hash: $(HASH), timestamp: $(TIMESTAMP)"
	@echo "revision: $(REV)"

generate: ## run code generation
	go generate ./...

test: ## run tests with race detection and show coverage
	go clean -testcache
	go test -race -coverprofile=coverage.out ./...
	grep -v "_mock.go" coverage.out | grep -v mocks > coverage_no_mocks.out
	go tool cover -func=coverage_no_mocks.out
	rm coverage.out coverage_no_mocks.out

race: ## run tests with race detection
	go test -race -timeout=60s ./...

lint: ## run golangci-lint
	golangci-lint run --max-issues-per-linter=0 --max-same-issues=0

fmt: ## format Go source files exclude mocks
	gofmt -s -w $(find . -type f -name "*.go" -not -path "./vendor/*" -not -path "*/mocks/*")

fmt-check: ## check Go source formatting
	@test -z "$$(gofmt -l .)" || { echo "not formatted:"; gofmt -l .; exit 1; }

vet: ## run go vet
	go vet ./...

check: ## fmt-check + vet + lint — fast check before commit
	@$(MAKE) fmt-check vet lint

upd: ## update Go dependencies and vendor modules
	go get -u ./...
	go mod tidy
	go mod vendor

help: ## commands list
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
