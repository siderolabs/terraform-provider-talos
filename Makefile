TAG ?= $(shell git describe --tag --always --dirty)
ARTIFACTS ?= _out

ifneq ($(origin TESTS), undefined)
	RUNARGS = -run='$(TESTS)'
endif

ifneq ($(origin CI), undefined)
	RUNARGS += -parallel=3
	RUNARGS += -timeout=40m
	RUNARGS += -exec="sudo -E"
endif

.PHONY: generate
generate:
	go generate ./pkg/talos
	go generate

.PHONY: testacc
testacc:
	# TF_CLI_CONFIG_FILE is set here to avoid using the user's .terraformrc file. Ref: https://github.com/hashicorp/terraform-plugin-sdk/issues/1171
	TF_CLI_CONFIG_FILE="thisfiledoesnotexist" TF_ACC=1 go test -v -failfast -cover $(RUNARGS) ./...

.PHONY: check-dirty
check-dirty: generate ## Verifies that source tree is not dirty
	@if test -n "`git status --porcelain`"; then echo "Source tree is dirty"; git status; exit 1 ; fi

build-debug:
	go build -gcflags='all=-N -l'

install:
	go install .

$(ARTIFACTS):
	mkdir -p $(ARTIFACTS)

release-notes: $(ARTIFACTS)
	@ARTIFACTS=$(ARTIFACTS) ./hack/release.sh $@ $(ARTIFACTS)/RELEASE_NOTES.md $(TAG)

go-vulncheck:
	go tool -modfile tools/go.mod golang.org/x/vuln/cmd/govulncheck ./...

sbom: $(ARTIFACTS)
	SYFT_FORMAT_PRETTY=1 SYFT_FORMAT_SPDX_JSON_DETERMINISTIC_UUID=1 go tool -modfile tools/go.mod github.com/anchore/syft/cmd/syft dir:. -o spdx-json > $(ARTIFACTS)/sbom.spdx.json
	SYFT_FORMAT_PRETTY=1 SYFT_FORMAT_SPDX_JSON_DETERMINISTIC_UUID=1 go tool -modfile tools/go.mod github.com/anchore/syft/cmd/syft dir:. -o cyclonedx-json > $(ARTIFACTS)/sbom.cyclonedx.json
