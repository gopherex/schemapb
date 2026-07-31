# schemapb — multi-language schema/validation library monorepo.
#
# Conventions:
#   - ALL tools live in ./bin (version-pinned below); nothing is installed
#     globally. `git clone` + `make configure` = fully working environment.
#   - `make help` lists every target.
#
# Prerequisites (system): go, node/npm, cargo, python3.

SHELL := /bin/bash
.ONESHELL:
.DEFAULT_GOAL := help

BIN   := $(CURDIR)/bin
TOOLS := $(CURDIR)/.tools

# ---- pinned tool versions (bumps are explicit commits) ----------------------
EASYP_VERSION                 := v0.16.6
PROTOC_GEN_GO_VERSION         := v1.36.11
PROTOC_GEN_ES_VERSION         := 2.11.0
PROTOC_GEN_PROST_VERSION      := 0.5.0
PROTOC_GEN_PROST_CRATE_VERSION:= 0.5.0
BETTERPROTO2_COMPILER_VERSION := 0.10.1
GOLANGCI_LINT_VERSION         := v2.6.2
YARN_BOOTSTRAP_VERSION        := 1.22.22

# easyp resolves protoc-gen-* plugins from PATH: put ./bin first. Paths in
# the config are relative to the config file; --root pins search to repo root.
EASYP := PATH="$(BIN):$$PATH" "$(BIN)/easyp"
EASYP_ROOT := --root "$(CURDIR)"

.PHONY: help
help: ## List all targets with explanations
	@grep -hE '^[a-zA-Z0-9_.-]+:.*## ' $(MAKEFILE_LIST) | \
	  awk -F':.*## ' '{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: configure
configure: ## Bring environment to working state: fetch all pinned tools into ./bin
	set -e
	mkdir -p "$(BIN)" "$(TOOLS)"
	echo "--- easyp $(EASYP_VERSION)"
	GOBIN="$(BIN)" go install github.com/easyp-tech/easyp/cmd/easyp@$(EASYP_VERSION)
	echo "--- protoc-gen-go $(PROTOC_GEN_GO_VERSION)"
	GOBIN="$(BIN)" go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	echo "--- protoc-gen-es $(PROTOC_GEN_ES_VERSION)"
	mkdir -p "$(TOOLS)/npm"
	npm install --prefix "$(TOOLS)/npm" --no-save --silent @bufbuild/protoc-gen-es@$(PROTOC_GEN_ES_VERSION)
	ln -sf "$(TOOLS)/npm/node_modules/.bin/protoc-gen-es" "$(BIN)/protoc-gen-es"
	echo "--- protoc-gen-prost $(PROTOC_GEN_PROST_VERSION)"
	cargo install --quiet --locked --root "$(TOOLS)/cargo" protoc-gen-prost --version $(PROTOC_GEN_PROST_VERSION)
	ln -sf "$(TOOLS)/cargo/bin/protoc-gen-prost" "$(BIN)/protoc-gen-prost"
	echo "--- protoc-gen-prost-crate $(PROTOC_GEN_PROST_CRATE_VERSION)"
	cargo install --quiet --locked --root "$(TOOLS)/cargo" protoc-gen-prost-crate --version $(PROTOC_GEN_PROST_CRATE_VERSION)
	ln -sf "$(TOOLS)/cargo/bin/protoc-gen-prost-crate" "$(BIN)/protoc-gen-prost-crate"
	echo "--- protoc-gen-python_betterproto2 (compiler $(BETTERPROTO2_COMPILER_VERSION))"
	python3 -m venv "$(TOOLS)/py"
	"$(TOOLS)/py/bin/pip" install --quiet betterproto2_compiler==$(BETTERPROTO2_COMPILER_VERSION)
	ln -sf "$(TOOLS)/py/bin/protoc-gen-python_betterproto2" "$(BIN)/protoc-gen-python_betterproto2"
	# betterproto2 compiler shells out to ruff (installed as its dependency)
	ln -sf "$(TOOLS)/py/bin/ruff" "$(BIN)/ruff"
	echo "--- yarn bootstrap $(YARN_BOOTSTRAP_VERSION) (ts/ pins its own via yarnPath)"
	mkdir -p "$(TOOLS)/npm-yarn"
	npm install --prefix "$(TOOLS)/npm-yarn" --no-save --silent yarn@$(YARN_BOOTSTRAP_VERSION)
	ln -sf "$(TOOLS)/npm-yarn/node_modules/.bin/yarn" "$(BIN)/yarn"
	cd ts && "$(BIN)/yarn" install --immutable
	echo "--- golangci-lint $(GOLANGCI_LINT_VERSION)"
	GOBIN="$(BIN)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	echo "✓ configure done — tools in $(BIN)"

.PHONY: gen
gen: gen-go gen-ts gen-py gen-rust ## Generate protobuf code for all 4 languages

.PHONY: gen-go
gen-go: ## Generate Go protobuf code (go/schemapb)
	$(EASYP) --cfg schemapb/easyp.go.yaml generate $(EASYP_ROOT)

.PHONY: gen-ts
gen-ts: ## Generate TypeScript protobuf code (ts/src/gen)
	$(EASYP) --cfg schemapb/easyp.ts.yaml generate $(EASYP_ROOT)

.PHONY: gen-py
gen-py: ## Generate Python protobuf code (py/src/gen, betterproto2)
	$(EASYP) --cfg schemapb/easyp.py.yaml generate $(EASYP_ROOT)

.PHONY: gen-rust
gen-rust: ## Generate Rust protobuf code (rust/src/gen, prost)
	$(EASYP) --cfg schemapb/easyp.rust.yaml generate $(EASYP_ROOT)

.PHONY: lint
lint: ## Lint proto files
	$(EASYP) --cfg schemapb/easyp.go.yaml lint $(EASYP_ROOT) -p schemapb

.PHONY: lint-go
lint-go: ## Lint Go code (golangci-lint, pinned in ./bin)
	cd go && "$(BIN)/golangci-lint" run ./...

.PHONY: lint-ts
lint-ts: ## Lint + typecheck TypeScript (biome + tsc, yarn pinned via yarnPath)
	cd ts && "$(BIN)/yarn" lint && "$(BIN)/yarn" typecheck

.PHONY: test-ts
test-ts: ## Run TypeScript tests (vitest)
	cd ts && "$(BIN)/yarn" test

.PHONY: breaking
breaking: ## Check proto files for breaking changes against main
	$(EASYP) --cfg schemapb/easyp.go.yaml breaking $(EASYP_ROOT) -p schemapb

.PHONY: clean
clean: ## Remove tools and generated code
	rm -rf "$(BIN)" "$(TOOLS)" go/schemapb ts/src/gen py/src/gen rust/src rust/Cargo.toml
