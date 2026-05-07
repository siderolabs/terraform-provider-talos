// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/tools/clientcmd"
)

type talosClusterKubeConfigDataSource struct{}

type talosClusterKubeConfigDataSourceModelV0 struct { //nolint:govet
	ID                            types.String                  `tfsdk:"id"`
	Node                          types.String                  `tfsdk:"node"`
	Endpoint                      types.String                  `tfsdk:"endpoint"`
	ClientConfiguration           clientConfiguration           `tfsdk:"client_configuration"`
	KubeConfigRaw                 types.String                  `tfsdk:"kubeconfig_raw"`
	KubernetesClientConfiguration kubernetesClientConfiguration `tfsdk:"kubernetes_client_configuration"`
	Wait                          types.Bool                    `tfsdk:"wait"`
	Timeouts                      timeouts.Value                `tfsdk:"timeouts"`
}

var _ datasource.DataSource = &talosClusterKubeConfigDataSource{}

// NewTalosClusterKubeConfigDataSource implements the datasource.DataSource interface.
func NewTalosClusterKubeConfigDataSource() datasource.DataSource {
	return &talosClusterKubeConfigDataSource{}
}

func (d *talosClusterKubeConfigDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + FieldClusterKubeconfig
}

func (d *talosClusterKubeConfigDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		DeprecationMessage: "Use `talos_cluster_kubeconfig` resource instead. This data source will be removed in the next minor version of the provider.",
		Description:        "Retrieves the kubeconfig for a Talos cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
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
			"wait": schema.BoolAttribute{
				Optional:           true,
				Description:        "Wait for the kubernetes api to be available",
				DeprecationMessage: "This attribute is deprecated and no-op. Will be removed in a future version. Use talos_cluster_health instead.",
			},
			FieldKubeconfigRaw: schema.StringAttribute{
				Computed:    true,
				Description: DescRawKubeconfig,
				Sensitive:   true,
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
			},
			FieldTimeouts: timeouts.Attributes(ctx, timeouts.Opts{
				Read: true,
			}),
		},
	}
}

// Read implements the datasource.DataSource interface.
func (d *talosClusterKubeConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state talosClusterKubeConfigDataSourceModelV0

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

	readTimeout, diags := state.Timeouts.Read(ctx, 10*time.Minute)
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

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}
