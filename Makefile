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
kwok-bench: ## Run KWOK benchmark with bench.yaml config
	@go run . --config bench.yaml

.PHONY: kwok-bench-local
kwok-bench-local: ## Run KWOK benchmark using local fork-external-dns (go.work)
	@GOWORK=$(PWD)/go.work go run . --config bench.yaml

.PHONY: go-deps
go-deps: ## Install go dependencies
	@go mod tidy
	@go mod download
