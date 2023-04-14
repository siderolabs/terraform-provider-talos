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
)

// talosProvider is the provider implementation.
type talosProvider struct{}

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
	resp.Schema = schema.Schema{}
}

// Configure prepares a Talos client for data sources and resources.
func (p *talosProvider) Configure(_ context.Context, _ provider.ConfigureRequest, _ *provider.ConfigureResponse) {
}

// DataSources defines the data sources implemented in the provider.
func (p *talosProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewTalosMachineDisksDataSource,
		NewTalosMachineConfigurationDataSource,
		NewTalosClientConfigurationDataSource,
		NewTalosClusterKubeConfigDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *talosProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewTalosMachineSecretsResource,
		NewTalosMachineConfigurationApplyResource,
		NewTalosMachineBootstrapResource,
	}
}
