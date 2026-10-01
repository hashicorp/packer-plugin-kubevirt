NAME=kubevirt
BINARY=packer-plugin-${NAME}

COUNT?=1
TEST?=$(shell go list ./...)
HASHICORP_PACKER_PLUGIN_SDK_VERSION?=$(shell go list -m github.com/hashicorp/packer-plugin-sdk | cut -d " " -f2)
PLUGIN_FQN=$(shell grep -E '^module' <go.mod | sed -E 's/module \s*//')
SDK_COMMUNICATOR_PARTIALS=cmd/packer-sdc/internal/renderdocs/docs-partials/packer-plugin-sdk/communicator

.PHONY: dev

build:
	@go build -o ${BINARY}

dev:
	go build -ldflags="-X '${PLUGIN_FQN}/version.VersionPrerelease=dev'" -o ${BINARY}
	packer plugins install --path ${BINARY} "$(shell echo "${PLUGIN_FQN}" | sed 's/packer-plugin-//')"

test:
	@go test -race -count $(COUNT) $(TEST) -timeout=3m

install-packer-sdc: ## Install packer sofware development command
	@go install github.com/hashicorp/packer-plugin-sdk/cmd/packer-sdc@${HASHICORP_PACKER_PLUGIN_SDK_VERSION}

plugin-check: install-packer-sdc build
	@packer-sdc plugin-check ${BINARY}

testacc: dev
	@PACKER_ACC=1 go test -count $(COUNT) -v $(TEST) -timeout=120m

# The SDK documentation of the SSH communicator is copied without the bastion
# and proxy options, which cannot reach the port forward of the builder.
generate: install-packer-sdc
	@go generate ./...
	@awk '/^(- `|<!--)/ { skip = ($$0 ~ /^- `ssh_(bastion|proxy)_/) } !skip' \
		"$$(go list -m -f '{{.Dir}}' github.com/hashicorp/packer-plugin-sdk)/${SDK_COMMUNICATOR_PARTIALS}/SSH-not-required.mdx" \
		> docs-partials/builder/kubevirt/iso/SSH-not-required.mdx
	@rm -rf .docs
	@packer-sdc renderdocs -src docs -partials docs-partials/ -dst .docs/
	@./.web-docs/scripts/compile-to-webdocs.sh "." ".docs" ".web-docs" "hashicorp"
	@rm -r ".docs"
