// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"errors"
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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	kubernetesAPIUnavailableError = "kubernetes api is unavailable"
)

type talosClusterKubeConfigDataSource struct{}

type talosClusterKubeConfigDataSourceModel struct {
	Id                            types.String                  `tfsdk:"id"`
	Node                          types.String                  `tfsdk:"node"`
	Endpoint                      types.String                  `tfsdk:"endpoint"`
	ClientConfiguration           clientConfiguration           `tfsdk:"client_configuration"`
	KubeConfigRaw                 types.String                  `tfsdk:"kubeconfig_raw"`
	KubernetesClientConfiguration kubernetesClientConfiguration `tfsdk:"kubernetes_client_configuration"`
	Wait                          types.Bool                    `tfsdk:"wait"`
	Timeouts                      timeouts.Value                `tfsdk:"timeouts"`
}

type kubernetesClientConfiguration struct {
	Host              types.String `tfsdk:"host"`
	CACertificate     types.String `tfsdk:"ca_certificate"`
	ClientCertificate types.String `tfsdk:"client_certificate"`
	ClientKey         types.String `tfsdk:"client_key"`
}

var (
	_ datasource.DataSource = &talosClusterKubeConfigDataSource{}
)

func NewTalosClusterKubeConfigDataSource() datasource.DataSource {
	return &talosClusterKubeConfigDataSource{}
}

func (d *talosClusterKubeConfigDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_kubeconfig"
}

func (d *talosClusterKubeConfigDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves the kubeconfig for a Talos cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"node": schema.StringAttribute{
				Required:    true,
				Description: "controlplane node to retrieve the kubeconfig from",
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "endpoint to use for the talosclient. if not set, the node value will be used",
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
			"wait": schema.BoolAttribute{
				Optional:    true,
				Description: "Wait for the kubernetes api to be available",
			},
			"kubeconfig_raw": schema.StringAttribute{
				Computed:    true,
				Description: "The raw kubeconfig",
				Sensitive:   true,
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
				Read: true,
			}),
		},
	}
}

func (d *talosClusterKubeConfigDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state talosClusterKubeConfigDataSourceModel
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

	readTimeout, diags := state.Timeouts.Read(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ctxDeadline, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	if err := retry.RetryContext(ctxDeadline, readTimeout, func() *retry.RetryError {
		if err := talosClientOp(ctx, state.Node.ValueString(), state.Endpoint.ValueString(), talosConfig, func(opFuncCtx context.Context, c *client.Client) error {
			kubeConfigBytes, err := c.Kubeconfig(opFuncCtx)
			if err != nil {
				return err
			}

			state.KubeConfigRaw = basetypes.NewStringValue(string(kubeConfigBytes))

			if state.Wait.ValueBool() {
				clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeConfigBytes)
				if err != nil {
					return err
				}
				restConfig, err := clientConfig.ClientConfig()
				if err != nil {
					return err
				}
				clientset, err := kubernetes.NewForConfig(restConfig)
				if err != nil {
					return err
				}

				if _, err := clientset.ServerVersion(); err != nil {
					return errors.New(kubernetesAPIUnavailableError)
				}
			}

			return nil
		}); err != nil {
			if s := status.Code(err); s == codes.InvalidArgument {
				return retry.NonRetryableError(err)
			}

			if state.Wait.ValueBool() {
				if errors.Is(err, errors.New(kubernetesAPIUnavailableError)) {
					return retry.RetryableError(err)
				}
			}

			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		resp.Diagnostics.AddError("failed to retrieve kubeconfig", err.Error())

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

	state.Id = basetypes.NewStringValue(clusterName)

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
