.EXPORT_ALL_VARIABLES:

# https://developer.hashicorp.com/terraform/internals/debugging
TF_LOG := INFO

.PHONY: help
help: ## Get more info on available commands
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
hooks: ## install pre commit.
	@pre-commit install
	@pre-commit gc
	@pre-commit autoupdate

hooks-install: ## Install pre-commit hooks
	@pre-commit install
	@pre-commit gc

hooks-uninstall: ## Uninstall hooks
	@pre-commit uninstall

hooks-validate: ## Validate files with pre-commit hooks
	@pre-commit run --all-files

.PHONY: kwok-bench
kwok-bench: ## Run KWOK benchmark against pinned go.mod version (disables go.work)
	@GOWORK=off go run . --config bench.yaml

.PHONY: kwok-bench-local
kwok-bench-local: ## Run KWOK benchmark using local fork-external-dns (go.work)
	@GOWORK=$(PWD)/go.work go run . --config bench.yaml

.PHONY: kwok-clean
kwok-clean: ## Delete all KWOK clusters created by this project
	@kwokctl get clusters 2>/dev/null | xargs -r -I{} kwokctl delete cluster --name {}

.PHONY: go-deps
go-deps: ## Install go dependencies
	@go get sigs.k8s.io/external-dns@latest
	@go mod tidy
	@go mod download
