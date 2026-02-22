// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"golang.org/x/mod/semver"
)

var _ ephemeral.EphemeralResource = &talosMachineSecretsEphemeralResource{}

type talosMachineSecretsEphemeralResource struct{}

type talosMachineSecretsEphemeralResourceModel struct {
	TalosVersion        types.String        `tfsdk:"talos_version"`
	MachineSecrets      machineSecrets      `tfsdk:"machine_secrets"`
	ClientConfiguration clientConfiguration `tfsdk:"client_configuration"`
}

// NewTalosMachineSecretsEphemeralResource implements the ephemeral.EphemeralResource interface.
func NewTalosMachineSecretsEphemeralResource() ephemeral.EphemeralResource {
	return &talosMachineSecretsEphemeralResource{}
}

func (r *talosMachineSecretsEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_secrets"
}

func (r *talosMachineSecretsEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generate machine secrets for Talos cluster. This is an ephemeral resource that does not persist secrets in Terraform state.",
		Attributes: map[string]schema.Attribute{
			"talos_version": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The version of talos features to use in generated machine configuration",
				Validators: []validator.String{
					talosVersionValid(),
				},
			},
			"machine_secrets": machineSecretsOutputSchemaAttribute(),
			"client_configuration": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"ca_certificate": schema.StringAttribute{
						Computed:    true,
						Description: "The client CA certificate",
					},
					"client_certificate": schema.StringAttribute{
						Computed:    true,
						Description: "The client certificate",
					},
					"client_key": schema.StringAttribute{
						Computed:    true,
						Sensitive:   true,
						Description: "The client key",
					},
				},
				Computed:    true,
				Description: "The generated client configuration data",
			},
		},
	}
}

func (r *talosMachineSecretsEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var config talosMachineSecretsEphemeralResourceModel

	diags = obj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Apply defaults for talos_version if not set, matching the regular resource's
	// plan modifier behavior which uses the compiled-in Talos version
	talosVersion := config.TalosVersion.ValueString()
	if talosVersion == "" {
		talosVersion = semver.MajorMinor(gendata.VersionTag)
	}

	versionContract, err := validateVersionContract(talosVersion)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to validate talos version",
			err.Error(),
		)

		return
	}

	// Generate secrets
	secretsBundle, err := secrets.NewBundle(secrets.NewFixedClock(time.Now()), versionContract)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to generate secrets bundle",
			err.Error(),
		)

		return
	}

	// Convert to model
	temp, err := secretsBundleTomachineSecrets(secretsBundle)
	if err != nil {
		resp.Diagnostics.AddError("failed to convert secrets bundle to machine secrets", err.Error())

		return
	}

	// Build the ephemeral model (without ID field)
	result := talosMachineSecretsEphemeralResourceModel{
		TalosVersion:        types.StringValue(talosVersion),
		MachineSecrets:      temp.MachineSecrets,
		ClientConfiguration: temp.ClientConfiguration,
	}

	// Set result - ephemeral resources use Result instead of State
	diags = resp.Result.Set(ctx, &result)
	resp.Diagnostics.Append(diags...)
}
