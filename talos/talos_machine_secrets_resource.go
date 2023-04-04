// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/generate"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

var (
	_ resource.Resource                 = &talosMachineSecretsResource{}
	_ resource.ResourceWithUpgradeState = &talosMachineSecretsResource{}
	_ resource.ResourceWithImportState  = &talosMachineSecretsResource{}
)

type talosMachineSecretsResource struct{}

type talosMachineSecretsResourceModelV0 struct {
	Id             types.String `tfsdk:"id"`
	TalosVersion   types.String `tfsdk:"talos_version"`
	MachineSecrets types.String `tfsdk:"machine_secrets"`
}

type talosMachineSecretsResourceModelV1 struct {
	TalosVersion        types.String        `tfsdk:"talos_version"`
	MachineSecrets      machineSecrets      `tfsdk:"machine_secrets"`
	ClientConfiguration clientConfiguration `tfsdk:"client_configuration"`
	Id                  types.String        `tfsdk:"id"`
}

type clientConfiguration struct {
	CA   types.String `tfsdk:"ca_certificate"`
	Cert types.String `tfsdk:"client_certificate"`
	Key  types.String `tfsdk:"client_key"`
}

type machineSecrets struct {
	Cluster    machineSecretsCluster    `tfsdk:"cluster"`
	Secrets    machineSecretsSecrets    `tfsdk:"secrets"`
	TrustdInfo machineSecretsTrustdInfo `tfsdk:"trustdinfo"`
	Certs      machineSecretsCerts      `tfsdk:"certs"`
}

type machineSecretsCluster struct {
	ID     types.String `tfsdk:"id"`
	Secret types.String `tfsdk:"secret"`
}

type machineSecretsSecrets struct {
	BootstrapToken            types.String `tfsdk:"bootstrap_token"`
	SecretboxEncryptionSecret types.String `tfsdk:"secretbox_encryption_secret"`
	AESCBCEncryptionSecret    types.String `tfsdk:"aescbc_encryption_secret"`
}

type machineSecretsTrustdInfo struct {
	Token types.String `tfsdk:"token"`
}

type machineSecretsCerts struct {
	Etcd              machineSecretsCertKeyPair            `tfsdk:"etcd"`
	K8s               machineSecretsCertKeyPair            `tfsdk:"k8s"`
	K8sAggregator     machineSecretsCertKeyPair            `tfsdk:"k8s_aggregator"`
	K8sServiceAccount machineSecretsCertsK8sServiceAccount `tfsdk:"k8s_serviceaccount"`
	OS                machineSecretsCertKeyPair            `tfsdk:"os"`
}

type machineSecretsCertsK8sServiceAccount struct {
	Key types.String `tfsdk:"key"`
}

type machineSecretsCertKeyPair struct {
	Cert types.String `tfsdk:"cert"`
	Key  types.String `tfsdk:"key"`
}

func NewTalosMachineSecretsResource() resource.Resource {
	return &talosMachineSecretsResource{}
}

func (r *talosMachineSecretsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_secrets"
}

func (r *talosMachineSecretsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     1,
		Description: "Generate machine secrets for Talos cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"talos_version": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The version of Talos for which to generate secrets",
				Validators: []validator.String{
					talosVersionValid(),
				},
				PlanModifiers: []planmodifier.String{
					TalosSecretsSchemaVersionCheck(),
				},
			},
			"machine_secrets": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"cluster": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"id": schema.StringAttribute{
								Computed: true,
							},
							"secret": schema.StringAttribute{
								Computed:  true,
								Sensitive: true,
							},
						},
						Computed: true,
					},
					"secrets": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"bootstrap_token": schema.StringAttribute{
								Computed:  true,
								Sensitive: true,
							},
							"secretbox_encryption_secret": schema.StringAttribute{
								Computed:  true,
								Sensitive: true,
							},
							"aescbc_encryption_secret": schema.StringAttribute{
								Computed:  true,
								Sensitive: true,
							},
						},
						Computed: true,
					},
					"trustdinfo": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"token": schema.StringAttribute{
								Computed:  true,
								Sensitive: true,
							},
						},
						Computed: true,
					},
					"certs": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"etcd":           certSchema(),
							"k8s":            certSchema(),
							"k8s_aggregator": certSchema(),
							"k8s_serviceaccount": schema.SingleNestedAttribute{
								Attributes: map[string]schema.Attribute{
									"key": schema.StringAttribute{
										Computed:  true,
										Sensitive: true,
									},
								},
								Computed: true,
							},
							"os": certSchema(),
						},
						Computed: true,
					},
				},
				Computed: true,
			},
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

func certSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Attributes: map[string]schema.Attribute{
			"cert": schema.StringAttribute{
				Computed: true,
			},
			"key": schema.StringAttribute{
				Computed:  true,
				Sensitive: true,
			},
		},
		Computed: true,
	}
}

func (r *talosMachineSecretsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var obj types.Object

	diags := req.Plan.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var plan talosMachineSecretsResourceModelV1

	diags = obj.As(ctx, &plan, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	genOptions := make([]generate.GenOption, 0, 1)

	if plan.TalosVersion.ValueString() != "" {
		versionContract, err := validateVersionContract(plan.TalosVersion.ValueString())
		if err != nil {
			resp.Diagnostics.AddError(
				"failed to validate talos version",
				err.Error(),
			)

			return
		}

		genOptions = append(genOptions, generate.WithVersionContract(versionContract))
	}

	secretsBundle, err := generate.NewSecretsBundle(generate.NewClock(), genOptions...)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to generate secrets bundle",
			err.Error(),
		)

		return
	}

	state, err := secretsBundleTomachineSecrets(secretsBundle)
	if err != nil {
		resp.Diagnostics.AddError("failed to convert secrets bundle to machine secrets", err.Error())

		return
	}

	state.TalosVersion = plan.TalosVersion

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *talosMachineSecretsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
}

func (r *talosMachineSecretsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var obj string

	diags := req.Plan.GetAttribute(ctx, path.Root("talos_version"), &obj)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set state to fully populated data
	diags = resp.State.SetAttribute(ctx, path.Root("talos_version"), obj)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *talosMachineSecretsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
}

func TalosSecretsSchemaVersionCheck() planmodifier.String {
	return &talosSecretsSchemaVersionPlanModifier{}
}

type talosSecretsSchemaVersionPlanModifier struct{}

var _ planmodifier.String = (*talosSecretsSchemaVersionPlanModifier)(nil)

func (apm *talosSecretsSchemaVersionPlanModifier) Description(ctx context.Context) string {
	return ""
}

func (apm *talosSecretsSchemaVersionPlanModifier) MarkdownDescription(ctx context.Context) string {
	return ""
}

func (apm *talosSecretsSchemaVersionPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, res *planmodifier.StringResponse) {
	if req.ConfigValue.IsUnknown() {
		return
	}

	// setting default value
	if req.PlanValue.IsUnknown() || req.PlanValue.IsNull() {
		res.PlanValue = basetypes.NewStringValue(semver.MajorMinor(gendata.VersionTag))

		return
	}

	if semver.Compare(req.PlanValue.ValueString(), req.StateValue.ValueString()) < 0 {
		res.RequiresReplace = true
	}
}

func (r *talosMachineSecretsResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Computed: true,
					},
					"talos_version": schema.StringAttribute{
						Optional: true,
					},
					"machine_secrets": schema.StringAttribute{
						Computed: true,
					},
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var priorStateData talosMachineSecretsResourceModelV0

				diags := req.State.Get(ctx, &priorStateData)
				resp.Diagnostics.Append(diags...)

				if resp.Diagnostics.HasError() {
					return
				}

				var secretsBundle *generate.SecretsBundle
				if err := yaml.Unmarshal([]byte(priorStateData.MachineSecrets.ValueString()), &secretsBundle); err != nil {
					resp.Diagnostics.AddError("failed to unmarshal machine secrets", err.Error())

					return
				}

				state, err := secretsBundleTomachineSecrets(secretsBundle)
				if err != nil {
					resp.Diagnostics.AddError("failed to convert secrets bundle to machine secrets", err.Error())

					return
				}

				state.TalosVersion = basetypes.NewStringValue("v1.3")

				if secretsBundle.Secrets.AESCBCEncryptionSecret != "" {
					state.TalosVersion = basetypes.NewStringValue("v1.2")
				}

				// Set state to fully populated data
				diags = resp.State.Set(ctx, state)
				resp.Diagnostics.Append(diags...)
				if resp.Diagnostics.HasError() {
					return
				}
			},
		},
	}
}

func (r *talosMachineSecretsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID

	if _, err := os.Stat(id); err != nil {
		resp.Diagnostics.AddError("failed to import state", err.Error())

		return
	}

	secretBytes, err := os.ReadFile(id)
	if err != nil {
		resp.Diagnostics.AddError("failed to read machine secrets file", err.Error())

		return
	}

	var secretsBundle *generate.SecretsBundle
	if err = yaml.Unmarshal(secretBytes, &secretsBundle); err != nil {
		resp.Diagnostics.AddError("failed to unmarshal machine secrets", err.Error())

		return
	}

	state, err := secretsBundleTomachineSecrets(secretsBundle)
	if err != nil {
		resp.Diagnostics.AddError("failed to convert secrets bundle to machine secrets", err.Error())

		return
	}

	state.TalosVersion = basetypes.NewStringValue("v1.3")

	if secretsBundle.Secrets.AESCBCEncryptionSecret != "" {
		state.TalosVersion = basetypes.NewStringValue("v1.2")
	}

	// Set state to fully populated data
	diags := resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
