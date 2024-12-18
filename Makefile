TAG ?= $(shell git describe --tag --always --dirty)
ARTIFACTS ?= _out
TEST_TIMEOUT ?= 600s

ifneq ($(origin TESTS), undefined)
	RUNARGS = -run='$(TESTS)'
endif

ifneq ($(origin CI), undefined)
	RUNARGS += -parallel=3
	RUNARGS += -timeout=25m
	RUNARGS += -exec="sudo -E"
endif

.PHONY: generate
generate:
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

release-notes:
	mkdir -p $(ARTIFACTS)
	@ARTIFACTS=$(ARTIFACTS) ./hack/release.sh $@ $(ARTIFACTS)/RELEASE_NOTES.md $(TAG)
