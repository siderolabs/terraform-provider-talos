// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/tools/clientcmd"
)

type talosClusterKubeConfigResource struct{}

var (
	_ resource.Resource                 = &talosClusterKubeConfigResource{}
	_ resource.ResourceWithModifyPlan   = &talosClusterKubeConfigResource{}
	_ resource.ResourceWithUpgradeState = &talosClusterKubeConfigResource{}
)

type talosClusterKubeConfigResourceModelV0 struct {
	ID                            types.String                  `tfsdk:"id"`
	Node                          types.String                  `tfsdk:"node"`
	Endpoint                      types.String                  `tfsdk:"endpoint"`
	ClientConfiguration           clientConfiguration           `tfsdk:"client_configuration"`
	KubeConfigRaw                 types.String                  `tfsdk:"kubeconfig_raw"`
	KubernetesClientConfiguration kubernetesClientConfiguration `tfsdk:"kubernetes_client_configuration"`
	Timeouts                      timeouts.Value                `tfsdk:"timeouts"`
}

type talosClusterKubeConfigResourceModelV1 struct {
	ID                            types.String                  `tfsdk:"id"`
	Node                          types.String                  `tfsdk:"node"`
	Endpoint                      types.String                  `tfsdk:"endpoint"`
	ClientConfiguration           clientConfiguration           `tfsdk:"client_configuration"`
	KubeConfigRaw                 types.String                  `tfsdk:"kubeconfig_raw"`
	KubernetesClientConfiguration kubernetesClientConfiguration `tfsdk:"kubernetes_client_configuration"`
	CertificateRenewalDuration    types.String                  `tfsdk:"certificate_renewal_duration"`
	Timeouts                      timeouts.Value                `tfsdk:"timeouts"`
}

type kubernetesClientConfiguration struct {
	Host              types.String `tfsdk:"host"`
	CACertificate     types.String `tfsdk:"ca_certificate"`
	ClientCertificate types.String `tfsdk:"client_certificate"`
	ClientKey         types.String `tfsdk:"client_key"`
}

// NewTalosClusterKubeConfigResource implements the resource.Resource interface.
func NewTalosClusterKubeConfigResource() resource.Resource {
	return &talosClusterKubeConfigResource{}
}

func (r *talosClusterKubeConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + FieldClusterKubeconfig
}

func (r *talosClusterKubeConfigResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     1,
		Description: "Retrieves the kubeconfig for a Talos cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			FieldNode: schema.StringAttribute{
				Required:    true,
				Description: DescControlplaneNodeForKubeconfig,
			},
			FieldEndpoint: schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: DescEndpointForTalosClient,
			},
			FieldClientConfiguration: schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					FieldCACertificate: schema.StringAttribute{
						Required:    true,
						Description: DescClientCACertificate,
					},
					FieldClientCertificate: schema.StringAttribute{
						Required:    true,
						Description: DescClientCertificate,
					},
					FieldClientKey: schema.StringAttribute{
						Required:    true,
						Sensitive:   true,
						Description: DescClientKey,
					},
				},
				Required:    true,
				Description: DescClientConfig,
			},
			FieldKubeconfigRaw: schema.StringAttribute{
				Computed:    true,
				Description: DescRawKubeconfig,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			FieldKubernetesClientConfig: schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					FieldHost: schema.StringAttribute{
						Computed:    true,
						Description: DescKubernetesHost,
					},
					FieldCACertificate: schema.StringAttribute{
						Computed:    true,
						Description: DescKubernetesCACertificate,
					},
					FieldClientCertificate: schema.StringAttribute{
						Computed:    true,
						Description: DescKubernetesClientCertificate,
					},
					FieldClientKey: schema.StringAttribute{
						Computed:    true,
						Sensitive:   true,
						Description: DescKubernetesClientKey,
					},
				},
				Computed:    true,
				Description: DescKubernetesClientConfig,
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
			},
			"certificate_renewal_duration": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The duration in hours before the certificate is renewed, defaults to 720h. Must be a valid duration string",
				Default:     stringdefault.StaticString("720h"),
			},
			FieldTimeouts: timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
			}),
		},
	}
}

// Create implements the resource.Resource interface.
func (r *talosClusterKubeConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state talosClusterKubeConfigResourceModelV1

	diags = obj.As(ctx, &state, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig(
		FieldDynamic,
		state.ClientConfiguration.CA.ValueString(),
		state.ClientConfiguration.Cert.ValueString(),
		state.ClientConfiguration.Key.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(ErrGenerateTalosConfig, err.Error())

		return
	}

	if state.Endpoint.IsNull() {
		state.Endpoint = state.Node
	}

	readTimeout, diags := state.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctxDeadline, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if retryErr := retry.RetryContext(ctxDeadline, readTimeout, func() *retry.RetryError {
		if clientOpErr := talosClientOp(ctx, state.Endpoint.ValueString(), state.Node.ValueString(), talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			kubeConfigBytes, clientErr := c.Kubeconfig(nodeCtx)
			if clientErr != nil {
				return clientErr
			}

			state.KubeConfigRaw = basetypes.NewStringValue(string(kubeConfigBytes))

			return nil
		}); clientOpErr != nil {
			if s := status.Code(clientOpErr); s == codes.InvalidArgument {
				return retry.NonRetryableError(clientOpErr)
			}

			return retry.RetryableError(clientOpErr)
		}

		return nil
	}); retryErr != nil {
		resp.Diagnostics.AddError(ErrRetrieveKubeconfig, retryErr.Error())

		return
	}

	kubeConfig, err := clientcmd.Load([]byte(state.KubeConfigRaw.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError(ErrParseKubeconfig, err.Error())

		return
	}

	clusterName := kubeConfig.Contexts[kubeConfig.CurrentContext].Cluster
	authName := kubeConfig.Contexts[kubeConfig.CurrentContext].AuthInfo

	state.KubernetesClientConfiguration = kubernetesClientConfiguration{
		Host:              basetypes.NewStringValue(kubeConfig.Clusters[clusterName].Server),
		CACertificate:     basetypes.NewStringValue(bytesToBase64(kubeConfig.Clusters[clusterName].CertificateAuthorityData)),
		ClientCertificate: basetypes.NewStringValue(bytesToBase64(kubeConfig.AuthInfos[authName].ClientCertificateData)),
		ClientKey:         basetypes.NewStringValue(bytesToBase64(kubeConfig.AuthInfos[authName].ClientKeyData)),
	}

	state.ID = basetypes.NewStringValue(clusterName)

	var planObj types.Object

	diags = req.Plan.Get(ctx, &planObj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var planState talosClusterKubeConfigResourceModelV1

	diags = planObj.As(ctx, &planState, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if state.CertificateRenewalDuration.IsNull() || state.CertificateRenewalDuration.IsUnknown() {
		state.CertificateRenewalDuration = planState.CertificateRenewalDuration
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *talosClusterKubeConfigResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *talosClusterKubeConfigResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

// ModifyPlan implements the resource.ResourceWithModifyPlan interface.
//
//nolint:gocyclo,cyclop
func (r *talosClusterKubeConfigResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// delete is a no-op
	if req.Plan.Raw.IsNull() {
		return
	}

	var configObj types.Object

	diags := req.Config.Get(ctx, &configObj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var config talosClusterKubeConfigResourceModelV1

	diags = configObj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// if either endpoint or node is unknown return early
	if config.Endpoint.IsUnknown() || config.Node.IsUnknown() {
		return
	}

	var planObj types.Object

	diags = req.Plan.Get(ctx, &planObj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var planState talosClusterKubeConfigResourceModelV1

	diags = configObj.As(ctx, &planState, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if planState.Endpoint.IsUnknown() || planState.Endpoint.IsNull() {
		diags = resp.Plan.SetAttribute(ctx, path.Root(FieldEndpoint), planState.Node.ValueString())
		resp.Diagnostics.Append(diags...)

		if diags.HasError() {
			return
		}
	}

	kubernetesClientConfigPath := path.Root(FieldKubernetesClientConfig)

	var obj types.Object

	resp.Diagnostics.Append(req.State.GetAttribute(ctx, kubernetesClientConfigPath, &obj)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var kubernetesClientConfig kubernetesClientConfiguration

	resp.Diagnostics.Append(obj.As(ctx, &kubernetesClientConfig, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})...)

	if resp.Diagnostics.HasError() {
		return
	}

	if kubernetesClientConfig.ClientCertificate.IsNull() || kubernetesClientConfig.ClientCertificate.IsUnknown() {
		return
	}

	kubernetesClientCertificate := kubernetesClientConfig.ClientCertificate.ValueString()

	kubernetesClientCertificateBytes, err := base64ToBytes(kubernetesClientCertificate)
	if err != nil {
		resp.Diagnostics.AddError(ErrDecodeKubernetesClientCert, err.Error())

		return
	}

	block, _ := pem.Decode(kubernetesClientCertificateBytes)
	if block == nil {
		resp.Diagnostics.AddError(ErrDecodeKubernetesClientCert, "failed to decode PEM block")

		return
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		resp.Diagnostics.AddError("failed to parse kubernetes client certificate", err.Error())

		return
	}

	var exisitingStateObj types.Object

	diags = req.State.Get(ctx, &exisitingStateObj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var existingState talosClusterKubeConfigResourceModelV1

	diags = exisitingStateObj.As(ctx, &existingState, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if planState.CertificateRenewalDuration.IsNull() || planState.CertificateRenewalDuration.IsUnknown() {
		planState.CertificateRenewalDuration = existingState.CertificateRenewalDuration
	}

	renewalDuration, err := time.ParseDuration(planState.CertificateRenewalDuration.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("failed to parse certificate renewal duration in plan", err.Error())

		return
	}

	// check if NotAfter expires in the given duration
	if x509Cert.NotAfter.Before(OverridableTimeFunc().Add(renewalDuration)) {
		tflog.Info(ctx, fmt.Sprintf("kubernetes client certificate expires in %s, needs regeneration", existingState.CertificateRenewalDuration.ValueString()))

		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(FieldKubernetesClientConfig).AtName(FieldHost), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(FieldKubernetesClientConfig).AtName(FieldClientCertificate), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(FieldKubernetesClientConfig).AtName(FieldClientKey), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(FieldKubernetesClientConfig).AtName(FieldCACertificate), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(FieldKubeconfigRaw), types.StringUnknown())...)

		if resp.Diagnostics.HasError() {
			return
		}
	}
}

func (r *talosClusterKubeConfigResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Computed: true,
					},
					FieldNode: schema.StringAttribute{
						Required: true,
					},
					FieldEndpoint: schema.StringAttribute{
						Optional: true,
						Computed: true,
					},
					FieldClientConfiguration: schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							FieldCACertificate: schema.StringAttribute{
								Required: true,
							},
							FieldClientCertificate: schema.StringAttribute{
								Required: true,
							},
							FieldClientKey: schema.StringAttribute{
								Required:  true,
								Sensitive: true,
							},
						},
						Required: true,
					},
					FieldKubeconfigRaw: schema.StringAttribute{
						Computed: true,

						Sensitive: true,
					},
					FieldKubernetesClientConfig: schema.SingleNestedAttribute{
						Attributes: map[string]schema.Attribute{
							FieldHost: schema.StringAttribute{
								Computed: true,
							},
							FieldCACertificate: schema.StringAttribute{
								Computed: true,
							},
							FieldClientCertificate: schema.StringAttribute{
								Computed: true,
							},
							FieldClientKey: schema.StringAttribute{
								Computed:  true,
								Sensitive: true,
							},
						},
						Computed: true,
					},
					FieldTimeouts: timeouts.Attributes(ctx, timeouts.Opts{
						Create: true,
						Update: true,
					}),
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var obj types.Object

				diags := req.State.Get(ctx, &obj)
				resp.Diagnostics.Append(diags...)

				if diags.HasError() {
					return
				}

				var priorStateData talosClusterKubeConfigResourceModelV0

				diags = obj.As(ctx, &priorStateData, basetypes.ObjectAsOptions{
					UnhandledNullAsEmpty:    true,
					UnhandledUnknownAsEmpty: true,
				})
				resp.Diagnostics.Append(diags...)

				if diags.HasError() {
					return
				}

				state := talosClusterKubeConfigResourceModelV1{
					ID:                  priorStateData.ID,
					Node:                priorStateData.Node,
					Endpoint:            priorStateData.Endpoint,
					ClientConfiguration: priorStateData.ClientConfiguration,
					KubeConfigRaw:       priorStateData.KubeConfigRaw,
					KubernetesClientConfiguration: kubernetesClientConfiguration{
						Host:              priorStateData.KubernetesClientConfiguration.Host,
						CACertificate:     priorStateData.KubernetesClientConfiguration.CACertificate,
						ClientCertificate: priorStateData.KubernetesClientConfiguration.ClientCertificate,
						ClientKey:         priorStateData.KubernetesClientConfiguration.ClientKey,
					},
					CertificateRenewalDuration: basetypes.NewStringValue("720h"),
					Timeouts:                   priorStateData.Timeouts,
				}

				// Set state to fully populated data
				diags = resp.State.Set(ctx, &state)

				resp.Diagnostics.Append(diags...)

				if resp.Diagnostics.HasError() {
					return
				}
			},
		},
	}
}

// Update implements the resource.ResourceWithModifyPlan interface.
//
//nolint:gocognit,gocyclo,cyclop
func (r *talosClusterKubeConfigResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var planObj types.Object

	resp.Diagnostics.Append(req.Plan.Get(ctx, &planObj)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state talosClusterKubeConfigResourceModelV1

	resp.Diagnostics.Append(planObj.As(ctx, &state, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	kubernetesClientConfigPath := path.Root(FieldKubernetesClientConfig)

	var stateObj types.Object

	resp.Diagnostics.Append(req.State.GetAttribute(ctx, kubernetesClientConfigPath, &stateObj)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var kubernetesClientConfig kubernetesClientConfiguration

	resp.Diagnostics.Append(stateObj.As(ctx, &kubernetesClientConfig, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})...)

	if resp.Diagnostics.HasError() {
		return
	}

	if kubernetesClientConfig.ClientCertificate.IsNull() || kubernetesClientConfig.ClientCertificate.IsUnknown() {
		return
	}

	kubernetesClientCertificate := kubernetesClientConfig.ClientCertificate.ValueString()

	kubernetesClientCertificateBytes, err := base64ToBytes(kubernetesClientCertificate)
	if err != nil {
		resp.Diagnostics.AddError(ErrDecodeKubernetesClientCert, err.Error())

		return
	}

	block, _ := pem.Decode(kubernetesClientCertificateBytes)
	if block == nil {
		resp.Diagnostics.AddError(ErrDecodeKubernetesClientCert, "failed to decode PEM block")

		return
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		resp.Diagnostics.AddError("failed to parse kubernetes client certificate", err.Error())

		return
	}

	renewalDuration, err := time.ParseDuration(state.CertificateRenewalDuration.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("failed to parse certificate renewal duration", err.Error())

		return
	}

	// check if NotAfter expires in the given duration
	if x509Cert.NotAfter.Before(OverridableTimeFunc().Add(renewalDuration)) {
		tflog.Info(ctx, fmt.Sprintf("kubernetes client certificate expires in %s, regenerating", state.CertificateRenewalDuration.ValueString()))

		talosConfig, err := talosClientTFConfigToTalosClientConfig(
			FieldDynamic,
			state.ClientConfiguration.CA.ValueString(),
			state.ClientConfiguration.Cert.ValueString(),
			state.ClientConfiguration.Key.ValueString(),
		)
		if err != nil {
			resp.Diagnostics.AddError(ErrGenerateTalosConfig, err.Error())

			return
		}

		if state.Endpoint.IsNull() {
			state.Endpoint = state.Node
		}

		updateTimeout, diags := state.Timeouts.Update(ctx, 10*time.Minute)
		resp.Diagnostics.Append(diags...)

		if resp.Diagnostics.HasError() {
			return
		}

		ctxDeadline, cancel := context.WithTimeout(ctx, updateTimeout)
		defer cancel()

		if retryErr := retry.RetryContext(ctxDeadline, updateTimeout, func() *retry.RetryError {
			if clientOpErr := talosClientOp(ctx, state.Endpoint.ValueString(), state.Node.ValueString(), talosConfig, func(nodeCtx context.Context, c *client.Client) error {
				kubeConfigBytes, clientErr := c.Kubeconfig(nodeCtx)
				if clientErr != nil {
					return clientErr
				}

				state.KubeConfigRaw = basetypes.NewStringValue(string(kubeConfigBytes))

				return nil
			}); clientOpErr != nil {
				if s := status.Code(clientOpErr); s == codes.InvalidArgument {
					return retry.NonRetryableError(clientOpErr)
				}

				return retry.RetryableError(clientOpErr)
			}

			return nil
		}); retryErr != nil {
			resp.Diagnostics.AddError(ErrRetrieveKubeconfig, retryErr.Error())

			return
		}

		kubeConfig, err := clientcmd.Load([]byte(state.KubeConfigRaw.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError(ErrParseKubeconfig, err.Error())

			return
		}

		clusterName := kubeConfig.Contexts[kubeConfig.CurrentContext].Cluster
		authName := kubeConfig.Contexts[kubeConfig.CurrentContext].AuthInfo

		state.KubernetesClientConfiguration = kubernetesClientConfiguration{
			Host:              basetypes.NewStringValue(kubeConfig.Clusters[clusterName].Server),
			CACertificate:     basetypes.NewStringValue(bytesToBase64(kubeConfig.Clusters[clusterName].CertificateAuthorityData)),
			ClientCertificate: basetypes.NewStringValue(bytesToBase64(kubeConfig.AuthInfos[authName].ClientCertificateData)),
			ClientKey:         basetypes.NewStringValue(bytesToBase64(kubeConfig.AuthInfos[authName].ClientKeyData)),
		}

		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(FieldKubernetesClientConfig).AtName(FieldHost), &state.KubernetesClientConfiguration.Host)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(FieldKubernetesClientConfig).AtName(FieldClientCertificate), &state.KubernetesClientConfiguration.ClientCertificate)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(FieldKubernetesClientConfig).AtName(FieldClientKey), &state.KubernetesClientConfiguration.ClientKey)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(FieldKubernetesClientConfig).AtName(FieldCACertificate), &state.KubernetesClientConfiguration.CACertificate)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root(FieldKubeconfigRaw), &state.KubeConfigRaw)...)

		if resp.Diagnostics.HasError() {
			return
		}
	}
}
