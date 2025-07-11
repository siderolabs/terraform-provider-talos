// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/tools/clientcmd"
)

type talosClusterKubeConfigEphemeralResource struct{}

var (
	_ resource.Resource = &talosClusterKubeConfigEphemeralResource{}
)

type talosClusterKubeConfigEphemeralResourceModel struct {
	ID                            types.String                           `tfsdk:"id"`
	Node                          types.String                           `tfsdk:"node"`
	Endpoint                      types.String                           `tfsdk:"endpoint"`
	ClientConfiguration           clientConfiguration                    `tfsdk:"client_configuration"`
	KubeConfigRaw                 types.String                           `tfsdk:"kubeconfig_raw"`
	KubernetesClientConfiguration ephemeralKubernetesClientConfiguration `tfsdk:"kubernetes_client_configuration"`
	Timeouts                      timeouts.Value                         `tfsdk:"timeouts"`
}

type ephemeralKubernetesClientConfiguration struct {
	Host              types.String `tfsdk:"host"`
	CACertificate     types.String `tfsdk:"ca_certificate"`
	ClientCertificate types.String `tfsdk:"client_certificate"`
	ClientKey         types.String `tfsdk:"client_key"`
}

// NewTalosClusterKubeConfigEphemeralResource implements the resource.Resource interface.
func NewTalosClusterKubeConfigEphemeralResource() resource.Resource {
	return &talosClusterKubeConfigEphemeralResource{}
}

func (r *talosClusterKubeConfigEphemeralResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_kubeconfig_ephemeral"
}

func (r *talosClusterKubeConfigEphemeralResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves the ephemeral kubeconfig for a Talos cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"node": schema.StringAttribute{
				Required:    true,
				Description: "controlplane node to retrieve the ephemeral kubeconfig from",
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
				Description: "The raw ephemeral kubeconfig",
				Sensitive:   true,
				// WriteOnly Arguments accept ephemeral values
				WriteOnly: true,
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
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
			}),
		},
	}
}

// Create implements the resource.Resource interface.
func (r *talosClusterKubeConfigEphemeralResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state talosClusterKubeConfigEphemeralResourceModel
	diags := req.Config.Get(ctx, &state)
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

	state.KubernetesClientConfiguration = ephemeralKubernetesClientConfiguration{
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

// Delete func does nothing, since there is no remote object to destroy from due to ephemeral nature
func (r *talosClusterKubeConfigEphemeralResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// Read func does nothing, since there is no state to refresh from due to ephemeral nature
func (r *talosClusterKubeConfigEphemeralResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

// Update calls Create function again, due to the fact that this resource is ephemeral and does not support updates.
func (r *talosClusterKubeConfigEphemeralResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddWarning("unexpected update", "talos_cluster_kubeconfig_ephemeral does not support updates, recreating resource")
	r.Create(ctx, resource.CreateRequest{
		Config:       req.Config,
		ProviderMeta: req.ProviderMeta,
	}, &resource.CreateResponse{
		State:   resp.State,
		Private: resp.Private,
	})
}
