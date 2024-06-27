// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

var (
	_ resource.Resource                 = &talosMachineSecretsResource{}
	_ resource.ResourceWithUpgradeState = &talosMachineSecretsResource{}
	_ resource.ResourceWithImportState  = &talosMachineSecretsResource{}
	_ resource.ResourceWithModifyPlan   = &talosMachineSecretsResource{}
)

// OverridableTimeFunc is a function that returns the current time. It is used to allow tests to override the current time.
//
//nolint:gocritic
var OverridableTimeFunc = func() time.Time {
	return time.Now()
}

type talosMachineSecretsResource struct{}

type talosMachineSecretsResourceModelV0 struct {
	ID             types.String `tfsdk:"id"`
	TalosVersion   types.String `tfsdk:"talos_version"`
	MachineSecrets types.String `tfsdk:"machine_secrets"`
}

type talosMachineSecretsResourceModelV1 struct {
	ID                  types.String        `tfsdk:"id"`
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

// NewTalosMachineSecretsResource implements the resource.Resource interface.
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
				Description: "The computed ID of the Talos cluster",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"talos_version": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The version of talos features to use in generated machine configuration",
				Validators: []validator.String{
					talosVersionValid(),
				},
				PlanModifiers: []planmodifier.String{
					talosMachineFeaturesVersionDefaults(),
				},
			},
			"machine_secrets": schema.SingleNestedAttribute{
				Description: "The secrets for the talos cluster",
				Attributes: map[string]schema.Attribute{
					"cluster": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"id": schema.StringAttribute{
								Description: "The cluster ID",
								Computed:    true,
							},
							"secret": schema.StringAttribute{
								Description: "The cluster secret",
								Computed:    true,
								Sensitive:   true,
							},
						},
						Description: "The cluster secrets",
						Computed:    true,
					},
					"secrets": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"bootstrap_token": schema.StringAttribute{
								Description: "The bootstrap token",
								Computed:    true,
								Sensitive:   true,
							},
							"secretbox_encryption_secret": schema.StringAttribute{
								Description: "The secretbox encryption secret",
								Computed:    true,
								Sensitive:   true,
							},
							"aescbc_encryption_secret": schema.StringAttribute{
								Description: "The AES-CBC encryption secret",
								Computed:    true,
								Sensitive:   true,
							},
						},
						Description: "kubernetes cluster secrets",
						Computed:    true,
					},
					"trustdinfo": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"token": schema.StringAttribute{
								Description: "The trustd token",
								Computed:    true,
								Sensitive:   true,
							},
						},
						Description: "trustd secrets",
						Computed:    true,
					},
					"certs": schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							"etcd":           certSchema(),
							"k8s":            certSchema(),
							"k8s_aggregator": certSchema(),
							"k8s_serviceaccount": schema.SingleNestedAttribute{
								Attributes: map[string]schema.Attribute{
									"key": schema.StringAttribute{
										Description: "The service account key",
										Computed:    true,
										Sensitive:   true,
									},
								},
								Description: "The service account secrets",
								Computed:    true,
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
		Description: "The certificate and key pair",
		Attributes: map[string]schema.Attribute{
			"cert": schema.StringAttribute{
				Description: "certificate data",
				Computed:    true,
			},
			"key": schema.StringAttribute{
				Description: "key data",
				Computed:    true,
				Sensitive:   true,
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

	versionContract, err := validateVersionContract(plan.TalosVersion.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to validate talos version",
			err.Error(),
		)

		return
	}

	secretsBundle, err := secrets.NewBundle(secrets.NewFixedClock(time.Now()), versionContract)
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

func (r *talosMachineSecretsResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *talosMachineSecretsResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// delete is a no-op
	if req.Plan.Raw.IsNull() {
		return
	}

	clientConfigurationPath := path.Root("client_configuration")

	var obj types.Object

	resp.Diagnostics.Append(req.State.GetAttribute(ctx, clientConfigurationPath, &obj)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var clientConfigurationData clientConfiguration

	diags := obj.As(ctx, &clientConfigurationData, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if clientConfigurationData.CA.IsNull() || clientConfigurationData.CA.IsUnknown() {
		return
	}

	clientCertificate := clientConfigurationData.Cert.ValueString()

	clientCertificateBytes, err := base64ToBytes(clientCertificate)
	if err != nil {
		resp.Diagnostics.AddError("failed to decode client certificate", err.Error())

		return
	}

	block, _ := pem.Decode(clientCertificateBytes)
	if block == nil {
		resp.Diagnostics.AddError("failed to decode client certificate", "failed to parse PEM block")

		return
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		resp.Diagnostics.AddError("failed to parse client certificate", err.Error())

		return
	}

	// check if NotAfter expires in a month
	if x509Cert.NotAfter.Before(OverridableTimeFunc().AddDate(0, 1, 0)) {
		tflog.Info(ctx, "client certificate expires in a month, regenerating")

		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("client_configuration").AtName("ca_certificate"), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("client_configuration").AtName("client_certificate"), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("client_configuration").AtName("client_key"), types.StringUnknown())...)

		if resp.Diagnostics.HasError() {
			return
		}
	}
}

func (r *talosMachineSecretsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var talosVersion string

	diags := req.Plan.GetAttribute(ctx, path.Root("talos_version"), &talosVersion)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Set state to fully populated data
	diags = resp.State.SetAttribute(ctx, path.Root("talos_version"), talosVersion)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	clientConfigurationPath := path.Root("client_configuration")

	var obj types.Object

	resp.Diagnostics.Append(req.State.GetAttribute(ctx, clientConfigurationPath, &obj)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var clientConfigurationData clientConfiguration

	diags = obj.As(ctx, &clientConfigurationData, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if clientConfigurationData.CA.IsNull() || clientConfigurationData.CA.IsUnknown() {
		return
	}

	clientCertificate := clientConfigurationData.Cert.ValueString()

	clientCertificateBytes, err := base64ToBytes(clientCertificate)
	if err != nil {
		resp.Diagnostics.AddError("failed to decode client certificate", err.Error())

		return
	}

	block, _ := pem.Decode(clientCertificateBytes)
	if block == nil {
		resp.Diagnostics.AddError("failed to decode client certificate", "failed to parse PEM block")

		return
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		resp.Diagnostics.AddError("failed to parse client certificate", err.Error())

		return
	}

	// check if NotAfter expires in a month
	if x509Cert.NotAfter.Before(OverridableTimeFunc().AddDate(0, 1, 0)) {
		tflog.Info(ctx, "client certificate expires in a month, regenerating")

		var obj types.Object

		diags := req.State.Get(ctx, &obj)
		resp.Diagnostics.Append(diags...)

		if resp.Diagnostics.HasError() {
			return
		}

		var config talosMachineSecretsResourceModelV1

		diags = obj.As(ctx, &config, basetypes.ObjectAsOptions{
			UnhandledNullAsEmpty:    true,
			UnhandledUnknownAsEmpty: true,
		})
		resp.Diagnostics.Append(diags...)

		if resp.Diagnostics.HasError() {
			return
		}

		secretsBundle, err := machineSecretsToSecretsBundle(config)
		if err != nil {
			resp.Diagnostics.AddError("failed to convert machine secrets to secrets bundle", err.Error())

			return
		}

		if secretsBundle.Clock == nil {
			secretsBundle.Clock = secrets.NewFixedClock(time.Now())
		}

		state, err := secretsBundleTomachineSecrets(secretsBundle)
		if err != nil {
			resp.Diagnostics.AddError("failed to convert secrets bundle to machine secrets", err.Error())

			return
		}

		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("client_configuration").AtName("ca_certificate"), &state.ClientConfiguration.CA)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("client_configuration").AtName("client_certificate"), &state.ClientConfiguration.Cert)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("client_configuration").AtName("client_key"), &state.ClientConfiguration.Key)...)

		if resp.Diagnostics.HasError() {
			return
		}
	}
}

func (r *talosMachineSecretsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func talosMachineFeaturesVersionDefaults() planmodifier.String {
	return &talosMachineFeaturesVersionPlanModifier{}
}

type talosMachineFeaturesVersionPlanModifier struct{}

var _ planmodifier.String = (*talosMachineFeaturesVersionPlanModifier)(nil)

func (apm *talosMachineFeaturesVersionPlanModifier) Description(_ context.Context) string {
	return "sets default value for talos_version if not set"
}

func (apm *talosMachineFeaturesVersionPlanModifier) MarkdownDescription(ctx context.Context) string {
	return apm.Description(ctx)
}

func (apm *talosMachineFeaturesVersionPlanModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, res *planmodifier.StringResponse) {
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

func (r *talosMachineSecretsResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
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

				var secretsBundle *secrets.Bundle
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

	var secretsBundle *secrets.Bundle
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
