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
PROTOC_GEN_PROST_SERDE_VERSION:= 0.4.0
BETTERPROTO2_COMPILER_VERSION := 0.10.1
GOLANGCI_LINT_VERSION         := v2.6.2
UV_VERSION                    := 0.9.28
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
	echo "--- protoc-gen-prost-serde $(PROTOC_GEN_PROST_SERDE_VERSION)"
	cargo install --quiet --locked --root "$(TOOLS)/cargo" protoc-gen-prost-serde --version $(PROTOC_GEN_PROST_SERDE_VERSION)
	ln -sf "$(TOOLS)/cargo/bin/protoc-gen-prost-serde" "$(BIN)/protoc-gen-prost-serde"
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
	echo "--- uv $(UV_VERSION) (py/ dev tool; consumers just pip install)"
	"$(TOOLS)/py/bin/pip" install --quiet uv==$(UV_VERSION)
	ln -sf "$(TOOLS)/py/bin/uv" "$(BIN)/uv"
	cd py && "$(BIN)/uv" sync
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

.PHONY: lint-py
lint-py: ## Lint + typecheck Python (ruff + mypy via uv)
	cd py && "$(BIN)/uv" run ruff check src tests && "$(BIN)/uv" run ruff format --check src tests && "$(BIN)/uv" run mypy

.PHONY: test-py
test-py: ## Run Python tests (pytest)
	cd py && "$(BIN)/uv" run pytest -q

.PHONY: test-go
test-go: ## Run Go tests
	cd go && go test ./...

.PHONY: lint-rust
lint-rust: ## Lint Rust code (clippy pedantic+nursery + rustfmt check)
	cd rust && cargo clippy --all-targets -- -D warnings && cargo fmt --check

.PHONY: test-rust
test-rust: ## Run Rust tests (conformance)
	cd rust && cargo test --quiet

# v2+ would require semantic import versioning for the Go module (/v2 in the
# module path), which we don't support yet — keep releases on v0/v1.
MAX_MAJOR := 1

.PHONY: release
release: ## Interactive lockstep release: one version, tag pair vX.Y.Z + go/vX.Y.Z
	set -euo pipefail
	cd "$$(git rev-parse --show-toplevel)"

	# 1. everything committed?
	if [ -n "$$(git status --porcelain)" ]; then
	  echo "✗ Working tree is not clean — commit or stash first:"
	  git status --short
	  exit 1
	fi

	cur="$$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' | sed 's/^v//' | sort -t. -k1,1n -k2,2n -k3,3n | tail -1)"
	cur="$${cur:-0.0.0}"
	head="$$(git rev-parse --short HEAD)"
	echo "Latest release: v$$cur    HEAD: $$head"
	echo
	echo "  1) recreate last tag (v$$cur) on HEAD   [force]"
	echo "  2) bump version"
	echo "  3) cancel"
	read -r -p "> " action

	# One release = one version across all four languages. The go/ prefix
	# twin is required by the Go module proxy (module lives in go/); the
	# release workflow triggers on the bare v-tag only.
	tags_for() { echo "v$$1"; echo "go/v$$1"; }

	case "$$action" in
	1)
	  if ! git tag -l "v$$cur" | grep -q .; then
	    echo "✗ No release tags to recreate."; exit 1
	  fi
	  mapfile -t TAGS < <(tags_for "$$cur")
	  echo
	  echo "Will DELETE and recreate $${#TAGS[@]} tags of v$$cur on $$head, then force-push."
	  read -r -p "Type 'yes' to proceed: " ok
	  [ "$$ok" = "yes" ] || { echo "Aborted."; exit 0; }
	  for t in "$${TAGS[@]}"; do
	    git tag -d "$$t" 2>/dev/null || true
	    git push origin ":refs/tags/$$t" 2>/dev/null || true
	  done
	  for t in "$${TAGS[@]}"; do git tag -a "$$t" -m "$$t"; done
	  git push origin --force "$${TAGS[@]}"
	  echo "✓ Recreated v$$cur on $$head."
	  ;;
	2)
	  IFS=. read -r MA MI PA <<< "$$cur"
	  echo
	  echo "  1) major  -> v$$((MA+1)).0.0"
	  echo "  2) minor  -> v$$MA.$$((MI+1)).0"
	  echo "  3) patch  -> v$$MA.$$MI.$$((PA+1))"
	  read -r -p "> " comp
	  case "$$comp" in
	    1) MA=$$((MA+1)); MI=0; PA=0 ;;
	    2) MI=$$((MI+1)); PA=0 ;;
	    3) PA=$$((PA+1)) ;;
	    *) echo "Aborted."; exit 0 ;;
	  esac
	  if [ "$$MA" -gt "$(MAX_MAJOR)" ]; then
	    echo "✗ v$$MA requires semantic import versioning (/v$$MA in the Go module path)."
	    echo "  Not supported yet — stay on v0/v1."
	    exit 1
	  fi
	  new="$$MA.$$MI.$$PA"
	  mapfile -t TAGS < <(tags_for "$$new")
	  echo
	  echo "Release v$$new — will create $${#TAGS[@]} tags on $$head and push."
	  echo "(versions are stamped from the tag by CI; manifests stay 0.0.0)"
	  read -r -p "Type 'yes' to proceed: " ok
	  [ "$$ok" = "yes" ] || { echo "Aborted."; exit 0; }
	  for t in "$${TAGS[@]}"; do git tag -a "$$t" -m "$$t"; done
	  git push origin "$${TAGS[@]}"
	  echo "✓ Released v$$new."
	  ;;
	*) echo "Aborted." ;;
	esac

.PHONY: breaking
breaking: ## Check proto files for breaking changes against main
	$(EASYP) --cfg schemapb/easyp.go.yaml breaking $(EASYP_ROOT) -p schemapb

.PHONY: clean
clean: ## Remove tools and generated code
	rm -rf "$(BIN)" "$(TOOLS)" go/schemapb ts/src/gen py/src/schemapb/_gen rust/src/gen/schemapb
