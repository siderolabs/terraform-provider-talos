.PHONY: generate
generate:
	go generate

.PHONY: testacc
testacc:
	TF_ACC=1 go test -v github.com/siderolabs/terraform-provider-talos/talos -timeout 30s

check-dirty: generate ## Verifies that source tree is not dirty
	@if test -n "`git status --porcelain`"; then echo "Source tree is dirty"; git status; exit 1 ; fi

build:
	go build -o terraform-provider-talos

install: build
	mkdir -p ~/.terraform.d/plugins/registry.local/siderolabs/talos/0.1.0/linux_amd64
	cp terraform-provider-talos ~/.terraform.d/plugins/registry.local/siderolabs/talos/0.1.0/linux_amd64/terraform-provider-talos
