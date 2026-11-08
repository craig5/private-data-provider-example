# Run `make` with no target for the list.

.DEFAULT_GOAL := help
SHELL := bash

# Prefer OpenTofu; fall back to Terraform. Override with `make TF=terraform`.
TF ?= $(shell command -v tofu || command -v terraform)

VENV := .venv
REQUIREMENTS := tf-hcl-only/tests/requirements.txt

# The provider's address and version, matching required_providers in
# tf-with-provider/main.tf.
SOURCE := providers.example.internal/craig5/c5egprov
VERSION := 0.1.0

# OpenTofu and Terraform both look for providers in ~/.terraform.d/plugins
# before trying a registry, laid out as <source>/<version>/<os>_<arch>/.
PLUGIN_DIR := $(HOME)/.terraform.d/plugins/$(SOURCE)
PLUGIN := $(PLUGIN_DIR)/$(VERSION)/$(shell go env GOOS)_$(shell go env GOARCH)/terraform-provider-c5egprov_v$(VERSION)

# Output format for `show`: table or json, e.g. `make show FORMAT=json`.
FORMAT ?= table

# The outputs section of a plan: what `compare` diffs.
PLAN_OUTPUTS = $(TF) plan -no-color | sed -n '/^Changes to Outputs/,/^$$/p'

# --- Run --------------------------------------------------------------------

# No init: this version uses no providers beyond the built-in one.
tf-hcl-only:
	cd tf-hcl-only && $(TF) plan

tf-with-provider: tf-with-provider/.terraform.lock.hcl
	cd tf-with-provider && $(TF) plan

# Print the parsed data with no OpenTofu involved, using the provider's own
# parsing code.
show:
	go -C provider run ./cmd/c5egprov-show -format $(FORMAT) -data-dir $(CURDIR)/data

# pipefail, so a failing plan stops make here instead of diffing two empty
# outputs and calling them equal. (Set per line: macOS ships make 3.81, which
# ignores .SHELLFLAGS.)
compare: tf-with-provider/.terraform.lock.hcl
	@set -o pipefail; cd tf-hcl-only && $(PLAN_OUTPUTS) > $(CURDIR)/.compare-hcl-only
	@set -o pipefail; cd tf-with-provider && $(PLAN_OUTPUTS) > $(CURDIR)/.compare-with-provider
	@test -s .compare-hcl-only || { echo "no plan outputs to compare"; exit 1; }
	diff .compare-hcl-only .compare-with-provider
	@rm -f .compare-hcl-only .compare-with-provider
	@echo "tf-hcl-only and tf-with-provider agree"

# --- Test -------------------------------------------------------------------

test: test-tf-hcl-only test-tf-with-provider

# The HCL-only version needs a second copy of its rules, in Python, to test
# the data.
test-tf-hcl-only: $(VENV)
	$(VENV)/bin/pytest tf-hcl-only/tests

# The provider version tests the rules where they live.
test-tf-with-provider:
	go -C provider test ./...

# --- Build and install ------------------------------------------------------

install: $(PLUGIN)

$(PLUGIN): $(shell find provider -name '*.go') provider/go.mod provider/go.sum
	go -C provider build -o $@ .

uninstall:
	rm -rf $(PLUGIN_DIR)

# A rebuilt provider has a new checksum under the same version, and init
# rejects it against the lock file - even with -upgrade. So start the lock file
# over after every install. Outside a demo, bump VERSION instead.
tf-with-provider/.terraform.lock.hcl: $(PLUGIN)
	rm -f $@
	cd tf-with-provider && $(TF) init -input=false

$(VENV): $(REQUIREMENTS)
	python3 -m venv $(VENV)
	$(VENV)/bin/pip install --quiet -r $(REQUIREMENTS)
	touch $(VENV)

# Maintainers only. requirements.in lists what the tests import; uv pins it,
# and its dependencies, into requirements.txt.
requirements:
	uv pip compile --universal --quiet tf-hcl-only/tests/requirements.in -o $(REQUIREMENTS)

clean:
	rm -rf $(VENV) .compare-hcl-only .compare-with-provider
	rm -rf tf-with-provider/.terraform tf-with-provider/.terraform.lock.hcl

help:
	@echo "Targets:"
	@echo "  tf-hcl-only            Plan the HCL-only version"
	@echo "  tf-with-provider       Install the provider and plan the provider version"
	@echo "  show                   Print the parsed data, using the provider's code"
	@echo "                         (FORMAT=table or FORMAT=json)"
	@echo "  compare                Check both versions produce the same bindings"
	@echo "  test                   Run both test suites"
	@echo "  test-tf-hcl-only       Python checks on the data, for the HCL-only version"
	@echo "  test-tf-with-provider  Go tests for the provider"
	@echo "  install                Build the provider into ~/.terraform.d/plugins"
	@echo "  uninstall              Remove it from ~/.terraform.d/plugins"
	@echo "  requirements           Re-pin the Python requirements (needs uv)"
	@echo "  clean                  Remove OpenTofu state and the venv"
	@echo
	@echo "TF is $(TF)"

.PHONY: help tf-hcl-only tf-with-provider show compare test test-tf-hcl-only test-tf-with-provider install uninstall requirements clean
