// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/generate"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"golang.org/x/mod/semver"
)

var (
	_ resource.Resource = &talosMachineSecretsResource{}
)

type talosMachineSecretsResource struct{}

type talosMachineSecretsResourceModel struct {
	TalosVersion        types.String        `tfsdk:"talos_version"`
	MachineSecrets      machineSecrets      `tfsdk:"machine_secrets"`
	ClientConfiguration clientConfiguration `tfsdk:"client_configuration"`
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
		Description: "Generate machine secrets for Talos cluster.",
		Attributes: map[string]schema.Attribute{
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

	var plan talosMachineSecretsResourceModel

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

	machineSecrets := machineSecrets{
		Cluster: machineSecretsCluster{
			ID:     types.StringValue(secretsBundle.Cluster.ID),
			Secret: types.StringValue(secretsBundle.Cluster.Secret),
		},
		Secrets: machineSecretsSecrets{
			BootstrapToken:            types.StringValue(secretsBundle.Secrets.BootstrapToken),
			SecretboxEncryptionSecret: types.StringValue(secretsBundle.Secrets.SecretboxEncryptionSecret),
		},
		TrustdInfo: machineSecretsTrustdInfo{
			Token: types.StringValue(secretsBundle.TrustdInfo.Token),
		},
		Certs: machineSecretsCerts{
			Etcd: machineSecretsCertKeyPair{
				Cert: types.StringValue(bytesToBase64(secretsBundle.Certs.Etcd.Crt)),
				Key:  types.StringValue(bytesToBase64(secretsBundle.Certs.Etcd.Key)),
			},
			K8s: machineSecretsCertKeyPair{
				Cert: types.StringValue(bytesToBase64(secretsBundle.Certs.K8s.Crt)),
				Key:  types.StringValue(bytesToBase64(secretsBundle.Certs.K8s.Key)),
			},
			K8sAggregator: machineSecretsCertKeyPair{
				Cert: types.StringValue(bytesToBase64(secretsBundle.Certs.K8sAggregator.Crt)),
				Key:  types.StringValue(bytesToBase64(secretsBundle.Certs.K8sAggregator.Key)),
			},
			K8sServiceAccount: machineSecretsCertsK8sServiceAccount{
				Key: types.StringValue(bytesToBase64(secretsBundle.Certs.K8sServiceAccount.Key)),
			},
			OS: machineSecretsCertKeyPair{
				Cert: types.StringValue(bytesToBase64(secretsBundle.Certs.OS.Crt)),
				Key:  types.StringValue(bytesToBase64(secretsBundle.Certs.OS.Key)),
			},
		},
	}

	// support for talos < 1.3
	if secretsBundle.Secrets.AESCBCEncryptionSecret != "" {
		machineSecrets.Secrets.AESCBCEncryptionSecret = types.StringValue(secretsBundle.Secrets.AESCBCEncryptionSecret)
	}

	plan.MachineSecrets = machineSecrets

	generateInput, err := generate.NewInput("", "", "", secretsBundle)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate talosconfig inputs", err.Error())

		return
	}

	talosConfig, err := generate.Talosconfig(generateInput)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate talosconfig", err.Error())

		return
	}

	plan.ClientConfiguration = clientConfiguration{
		CA:   types.StringValue(talosConfig.Contexts[talosConfig.Context].CA),
		Cert: types.StringValue(talosConfig.Contexts[talosConfig.Context].Crt),
		Key:  types.StringValue(talosConfig.Contexts[talosConfig.Context].Key),
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, plan)
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
	// setting default value
	if req.PlanValue.IsUnknown() || req.PlanValue.IsNull() {
		res.PlanValue = basetypes.NewStringValue(semver.MajorMinor(gendata.VersionTag))

		return
	}

	if semver.MajorMinor(req.PlanValue.ValueString()) != semver.MajorMinor(req.StateValue.ValueString()) {
		res.RequiresReplace = true
	}
}
