// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/image-factory/pkg/client"
)

type talosImageFactoryVersionsDataSource struct {
	imageFactoryClient *client.Client
}

type talosImageFactoryVersionsDataSourceModelV0 struct {
	ID            types.String                     `tfsdk:"id"`
	Filters       *talosImageFactoryVersionsFilter `tfsdk:"filters"`
	TalosVersions []string                         `tfsdk:"talos_versions"`
}

type talosImageFactoryVersionsFilter struct {
	StableVersionOnly types.Bool `tfsdk:"stable_versions_only"`
}

var _ datasource.DataSourceWithConfigure = &talosImageFactoryVersionsDataSource{}

// NewTalosImageFactoryVersionsDataSource implements the datasource.DataSource interface.
func NewTalosImageFactoryVersionsDataSource() datasource.DataSource {
	return &talosImageFactoryVersionsDataSource{}
}

func (d *talosImageFactoryVersionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_image_factory_versions"
}

func (d *talosImageFactoryVersionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The image factory versions data source provides a list of available talos versions from the image factory.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"talos_versions": schema.ListAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "The list of available talos versions.",
			},
			"filters": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "The filter to apply to the overlays list.",
				Attributes: map[string]schema.Attribute{
					"stable_versions_only": schema.BoolAttribute{
						Optional:    true,
						Description: "If set to true, only stable versions will be returned. If set to false, all versions will be returned.",
					},
				},
			},
		},
	}
}

func (d *talosImageFactoryVersionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	imageFactoryClient, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"failed to get image factory client",
			fmt.Sprintf("Expected *client.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	d.imageFactoryClient = imageFactoryClient
}

func (d *talosImageFactoryVersionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.imageFactoryClient == nil {
		resp.Diagnostics.AddError("image factory client is not configured", "Please report this issue to the provider developers.")

		return
	}

	versions, err := d.imageFactoryClient.Versions(ctx)
	if err != nil {
		resp.Diagnostics.AddError("failed to get talos versions", err.Error())

		return
	}

	var config talosImageFactoryVersionsDataSourceModelV0

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if config.Filters != nil && config.Filters.StableVersionOnly.ValueBool() {
		var filteredVersions []string

		for _, version := range versions {
			semVer, err := semver.Parse(strings.TrimPrefix(version, "v"))
			if err != nil {
				resp.Diagnostics.AddError("failed to parse talos version", err.Error())

				return
			}

			if len(semVer.Pre) > 0 {
				continue
			}

			filteredVersions = append(filteredVersions, version)
		}

		if resp.Diagnostics.HasError() {
			return
		}

		versions = filteredVersions
	}

	state := talosImageFactoryVersionsDataSourceModelV0{
		ID:            basetypes.NewStringValue("talos_versions"),
		TalosVersions: versions,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}
}
