// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
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
	_ resource.Resource               = &talosClusterKubeConfigResource{}
	_ resource.ResourceWithModifyPlan = &talosClusterKubeConfigResource{}
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
	resp.TypeName = req.ProviderTypeName + "_cluster_kubeconfig"
}

func (r *talosClusterKubeConfigResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves the kubeconfig for a Talos cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"node": schema.StringAttribute{
				Required:    true,
				Description: "controlplane node to retrieve the kubeconfig from",
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "endpoint to use for the talosclient. If not set, the node value will be used",
			},
			"client_configuration": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"ca_certificate": schema.StringAttribute{
						Required:    true,
						Description: "The client CA certificate",
					},
					"client_certificate": schema.StringAttribute{
						Required:    true,
						Description: "The client certificate",
					},
					"client_key": schema.StringAttribute{
						Required:    true,
						Sensitive:   true,
						Description: "The client key",
					},
				},
				Required:    true,
				Description: "The client configuration data",
			},
			"kubeconfig_raw": schema.StringAttribute{
				Computed:    true,
				Description: "The raw kubeconfig",
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"kubernetes_client_configuration": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"host": schema.StringAttribute{
						Computed:    true,
						Description: "The kubernetes host",
					},
					"ca_certificate": schema.StringAttribute{
						Computed:    true,
						Description: "The kubernetes CA certificate",
					},
					"client_certificate": schema.StringAttribute{
						Computed:    true,
						Description: "The kubernetes client certificate",
					},
					"client_key": schema.StringAttribute{
						Computed:    true,
						Sensitive:   true,
						Description: "The kubernetes client key",
					},
				},
				Computed:    true,
				Description: "The kubernetes client configuration",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.UseStateForUnknown(),
				},
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
			}),
		},
	}
}

// Create implements the resource.Resource interface.
//
//nolint:dupl
func (r *talosClusterKubeConfigResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state talosClusterKubeConfigResourceModelV0
	diags = obj.As(ctx, &state, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig(
		"dynamic",
		state.ClientConfiguration.CA.ValueString(),
		state.ClientConfiguration.Cert.ValueString(),
		state.ClientConfiguration.Key.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate talos config", err.Error())

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
		resp.Diagnostics.AddError("failed to retrieve kubeconfig", retryErr.Error())

		return
	}

	kubeConfig, err := clientcmd.Load([]byte(state.KubeConfigRaw.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("failed to parse kubeconfig", err.Error())

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

	var config talosClusterKubeConfigResourceModelV0

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

	var planState talosClusterKubeConfigResourceModelV0

	diags = configObj.As(ctx, &planState, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if planState.Endpoint.IsUnknown() || planState.Endpoint.IsNull() {
		diags = resp.Plan.SetAttribute(ctx, path.Root("endpoint"), planState.Node.ValueString())
		resp.Diagnostics.Append(diags...)

		if diags.HasError() {
			return
		}
	}

	kubernetesClientConfigPath := path.Root("kubernetes_client_configuration")

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
		resp.Diagnostics.AddError("failed to decode kubernetes client certificate", err.Error())

		return
	}

	block, _ := pem.Decode(kubernetesClientCertificateBytes)
	if block == nil {
		resp.Diagnostics.AddError("failed to decode kubernetes client certificate", "failed to decode PEM block")

		return
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		resp.Diagnostics.AddError("failed to parse kubernetes client certificate", err.Error())

		return
	}

	// check if NotAfter expires in a month
	if x509Cert.NotAfter.Before(OverridableTimeFunc().AddDate(0, 1, 0)) {
		tflog.Info(ctx, "kubernetes client certificate expires in a month, needs regeneration")

		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("kubernetes_client_configuration").AtName("host"), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("kubernetes_client_configuration").AtName("client_certificate"), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("kubernetes_client_configuration").AtName("client_key"), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("kubernetes_client_configuration").AtName("ca_certificate"), types.StringUnknown())...)
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("kubeconfig_raw"), types.StringUnknown())...)

		if resp.Diagnostics.HasError() {
			return
		}
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

	var state talosClusterKubeConfigResourceModelV0

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

	kubernetesClientConfigPath := path.Root("kubernetes_client_configuration")

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
		resp.Diagnostics.AddError("failed to decode kubernetes client certificate", err.Error())

		return
	}

	block, _ := pem.Decode(kubernetesClientCertificateBytes)
	if block == nil {
		resp.Diagnostics.AddError("failed to decode kubernetes client certificate", "failed to decode PEM block")

		return
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		resp.Diagnostics.AddError("failed to parse kubernetes client certificate", err.Error())

		return
	}

	// check if NotAfter expires in a month
	if x509Cert.NotAfter.Before(OverridableTimeFunc().AddDate(0, 1, 0)) {
		tflog.Info(ctx, "kubernetes client certificate expires in a month, regenerating")

		talosConfig, err := talosClientTFConfigToTalosClientConfig(
			"dynamic",
			state.ClientConfiguration.CA.ValueString(),
			state.ClientConfiguration.Cert.ValueString(),
			state.ClientConfiguration.Key.ValueString(),
		)
		if err != nil {
			resp.Diagnostics.AddError("failed to generate talos config", err.Error())

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
			resp.Diagnostics.AddError("failed to retrieve kubeconfig", retryErr.Error())

			return
		}

		kubeConfig, err := clientcmd.Load([]byte(state.KubeConfigRaw.ValueString()))
		if err != nil {
			resp.Diagnostics.AddError("failed to parse kubeconfig", err.Error())

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

		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("kubernetes_client_configuration").AtName("host"), &state.KubernetesClientConfiguration.Host)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("kubernetes_client_configuration").AtName("client_certificate"), &state.KubernetesClientConfiguration.ClientCertificate)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("kubernetes_client_configuration").AtName("client_key"), &state.KubernetesClientConfiguration.ClientKey)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("kubernetes_client_configuration").AtName("ca_certificate"), &state.KubernetesClientConfiguration.CACertificate)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("kubeconfig_raw"), &state.KubeConfigRaw)...)

		if resp.Diagnostics.HasError() {
			return
		}
	}
}
