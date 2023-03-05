TAG ?= $(shell git describe --tag --always --dirty)
ARTIFACTS ?= _out

.PHONY: generate
generate:
	go generate

.PHONY: testacc
testacc:
	TF_ACC=1 go test -v github.com/siderolabs/terraform-provider-talos/talos -timeout 30s

.PHONY: check-dirty
check-dirty: generate fmt ## Verifies that source tree is not dirty
	@if test -n "`git status --porcelain`"; then echo "Source tree is dirty"; git status; exit 1 ; fi

.PHONY: fmt
fmt:
	@find . -type f -name "*.tf" -exec terraform fmt {} \;

build:
	go build -o terraform-provider-talos .

install:
	go install .

release-notes:
	mkdir -p $(ARTIFACTS)
	@ARTIFACTS=$(ARTIFACTS) ./hack/release.sh $@ $(ARTIFACTS)/RELEASE_NOTES.md $(TAG)
