// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package talos is a Terraform provider for Talos.
package talos

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/siderolabs/image-factory/pkg/client"
)

const (
	// ImageFactoryURL is the default URL of Image Factory.
	ImageFactoryURL = "https://factory.talos.dev"
)

// talosProvider is the provider implementation.
type talosProvider struct{}

type talosProviderModelV0 struct {
	ImageFactoryURL types.String `tfsdk:"image_factory_url"`
}

// New is a helper function to simplify provider server and testing implementation.
func New() provider.Provider {
	return &talosProvider{}
}

// Metadata returns the provider type name.
func (p *talosProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "talos"
}

// Schema defines the provider-level schema for configuration data.
func (p *talosProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"image_factory_url": schema.StringAttribute{
				Optional:    true,
				Description: "The URL of Image Factory to generate schematics. If not set defaults to https://factory.talos.dev.",
			},
		},
	}
}

// Configure prepares a Talos client for data sources and resources.
func (p *talosProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config talosProviderModelV0

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	imageFactoryURL := config.ImageFactoryURL.ValueString()

	if imageFactoryURL == "" && !config.ImageFactoryURL.IsUnknown() {
		imageFactoryURL = ImageFactoryURL
	}

	imageFactoryClient, err := client.New(imageFactoryURL)
	if err != nil {
		resp.Diagnostics.AddError("failed to create Image Factory client", err.Error())

		return
	}

	resp.DataSourceData = imageFactoryClient
	resp.ResourceData = imageFactoryClient
}

// DataSources defines the data sources implemented in the provider.
func (p *talosProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewTalosMachineDisksDataSource,
		NewTalosMachineConfigurationDataSource,
		NewTalosClientConfigurationDataSource,
		NewTalosClusterHealthDataSource,
		NewTalosClusterKubeConfigDataSource,
		NewTalosImageFactoryVersionsDataSource,
		NewTalosImageFactoryExtensionsVersionsDataSource,
		NewTalosImageFactoryOverlaysVersionsDataSource,
		NewTalosImageFactoryURLSDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *talosProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewTalosMachineSecretsResource,
		NewTalosMachineConfigurationApplyResource,
		NewTalosMachineBootstrapResource,
		NewTalosClusterKubeConfigResource,
		NewTalosImageFactorySchematicResource,
	}
}
