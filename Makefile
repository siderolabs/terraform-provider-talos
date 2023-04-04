TAG ?= $(shell git describe --tag --always --dirty)
ARTIFACTS ?= _out

ifneq ($(origin TESTS), undefined)
	RUNARGS = -run='$(TESTS)'
endif

.PHONY: generate
generate:
	go generate

.PHONY: testacc
testacc:
	# TF_CLI_CONFIG_FILE is set here to avoid using the user's .terraformrc file. Ref: https://github.com/hashicorp/terraform-plugin-sdk/issues/1171
	TF_CLI_CONFIG_FILE="thisfiledoesnotexist" TF_ACC=1 go test -v github.com/siderolabs/terraform-provider-talos/talos -timeout 300s $(RUNARGS)

.PHONY: check-dirty
check-dirty: generate fmt ## Verifies that source tree is not dirty
	@if test -n "`git status --porcelain`"; then echo "Source tree is dirty"; git status; exit 1 ; fi

.PHONY: fmt
fmt:
	@find . -type f -name "*.tf" -exec terraform fmt {} \;

install:
	go install .

release-notes:
	mkdir -p $(ARTIFACTS)
	@ARTIFACTS=$(ARTIFACTS) ./hack/release.sh $@ $(ARTIFACTS)/RELEASE_NOTES.md $(TAG)
