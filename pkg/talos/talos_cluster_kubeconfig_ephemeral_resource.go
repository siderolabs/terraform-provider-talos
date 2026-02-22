// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/tools/clientcmd"
)

var _ ephemeral.EphemeralResource = &talosClusterKubeConfigEphemeralResource{}

type talosClusterKubeConfigEphemeralResource struct{}

type talosClusterKubeConfigEphemeralResourceModel struct {
	Node                          types.String                  `tfsdk:"node"`
	Endpoint                      types.String                  `tfsdk:"endpoint"`
	ClientConfiguration           clientConfiguration           `tfsdk:"client_configuration"`
	KubeConfigRaw                 types.String                  `tfsdk:"kubeconfig_raw"`
	KubernetesClientConfiguration kubernetesClientConfiguration `tfsdk:"kubernetes_client_configuration"`
}

// NewTalosClusterKubeConfigEphemeralResource implements the ephemeral.EphemeralResource interface.
func NewTalosClusterKubeConfigEphemeralResource() ephemeral.EphemeralResource {
	return &talosClusterKubeConfigEphemeralResource{}
}

func (r *talosClusterKubeConfigEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_kubeconfig"
}

func (r *talosClusterKubeConfigEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Retrieves the kubeconfig for a Talos cluster. This is an ephemeral resource that does not persist secrets in Terraform state.",
		Attributes: map[string]schema.Attribute{
			"node": schema.StringAttribute{
				Required:    true,
				Description: "controlplane node to retrieve the kubeconfig from",
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
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
		},
	}
}

func (r *talosClusterKubeConfigEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var config talosClusterKubeConfigEphemeralResourceModel

	diags = obj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig(
		"dynamic",
		config.ClientConfiguration.CA.ValueString(),
		config.ClientConfiguration.Cert.ValueString(),
		config.ClientConfiguration.Key.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate talos config", err.Error())

		return
	}

	endpoint := config.Endpoint.ValueString()
	if endpoint == "" {
		endpoint = config.Node.ValueString()
	}

	// Use a reasonable timeout for ephemeral resources
	readTimeout := 10 * time.Minute

	ctxDeadline, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	var kubeConfigRaw string

	if retryErr := retry.RetryContext(ctxDeadline, readTimeout, func() *retry.RetryError {
		if clientOpErr := talosClientOp(ctx, endpoint, config.Node.ValueString(), talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			kubeConfigBytes, clientErr := c.Kubeconfig(nodeCtx)
			if clientErr != nil {
				return clientErr
			}

			kubeConfigRaw = string(kubeConfigBytes)

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

	kubeConfig, err := clientcmd.Load([]byte(kubeConfigRaw))
	if err != nil {
		resp.Diagnostics.AddError("failed to parse kubeconfig", err.Error())

		return
	}

	currentContext, ok := kubeConfig.Contexts[kubeConfig.CurrentContext]
	if !ok || currentContext == nil {
		resp.Diagnostics.AddError("failed to parse kubeconfig", "current context not found in kubeconfig")

		return
	}

	clusterName := currentContext.Cluster
	authName := currentContext.AuthInfo

	clusterInfo, ok := kubeConfig.Clusters[clusterName]
	if !ok || clusterInfo == nil {
		resp.Diagnostics.AddError("failed to parse kubeconfig", fmt.Sprintf("cluster %q not found in kubeconfig", clusterName))

		return
	}

	authInfo, ok := kubeConfig.AuthInfos[authName]
	if !ok || authInfo == nil {
		resp.Diagnostics.AddError("failed to parse kubeconfig", fmt.Sprintf("auth info %q not found in kubeconfig", authName))

		return
	}

	// Build result
	result := talosClusterKubeConfigEphemeralResourceModel{
		Node:                config.Node,
		Endpoint:            basetypes.NewStringValue(endpoint),
		ClientConfiguration: config.ClientConfiguration,
		KubeConfigRaw:       basetypes.NewStringValue(kubeConfigRaw),
		KubernetesClientConfiguration: kubernetesClientConfiguration{
			Host:              basetypes.NewStringValue(clusterInfo.Server),
			CACertificate:     basetypes.NewStringValue(bytesToBase64(clusterInfo.CertificateAuthorityData)),
			ClientCertificate: basetypes.NewStringValue(bytesToBase64(authInfo.ClientCertificateData)),
			ClientKey:         basetypes.NewStringValue(bytesToBase64(authInfo.ClientKeyData)),
		},
	}

	diags = resp.Result.Set(ctx, &result)
	resp.Diagnostics.Append(diags...)
}
