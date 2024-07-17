// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/siderolabs/gen/xslices"
	"github.com/siderolabs/image-factory/pkg/client"
)

type talosImageFactoryOverlaysVersionsDataSource struct {
	imageFactoryClient *client.Client
}

type talosImageFactoryOverlaysVersionsDataSourceModelV0 struct {
	ID           types.String                             `tfsdk:"id"`
	TalosVersion types.String                             `tfsdk:"talos_version"`
	Filters      *talosImageFactoryOverlaysVersionsFilter `tfsdk:"filters"`
	OverlaysInfo []overlayInfo                            `tfsdk:"overlays_info"`
}

type talosImageFactoryOverlaysVersionsFilter struct {
	Name types.String `tfsdk:"name"`
}

type overlayInfo struct {
	Name   types.String `tfsdk:"name"`
	Image  types.String `tfsdk:"image"`
	Ref    types.String `tfsdk:"ref"`
	Digest types.String `tfsdk:"digest"`
}

var _ datasource.DataSourceWithConfigure = &talosImageFactoryOverlaysVersionsDataSource{}

// NewTalosImageFactoryOverlaysVersionsDataSource implements the datasource.DataSource interface.
func NewTalosImageFactoryOverlaysVersionsDataSource() datasource.DataSource {
	return &talosImageFactoryOverlaysVersionsDataSource{}
}

func (d *talosImageFactoryOverlaysVersionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_image_factory_overlays_versions"
}

func (d *talosImageFactoryOverlaysVersionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The image factory overlays versions data source provides a list of available overlays for a specific talos version from the image factory.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"talos_version": schema.StringAttribute{
				Required:    true,
				Description: "The talos version to get overlays for.",
			},
			"filters": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "The filter to apply to the overlays list.",
				Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						Optional:    true,
						Description: "The name of the overlay to filter by.",
					},
				},
			},
			"overlays_info": schema.ListAttribute{
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"name":   types.StringType,
						"image":  types.StringType,
						"ref":    types.StringType,
						"digest": types.StringType,
					},
				},
				Computed:    true,
				Description: "The list of available extensions for the specified talos version.",
			},
		},
	}
}

func (d *talosImageFactoryOverlaysVersionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *talosImageFactoryOverlaysVersionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.imageFactoryClient == nil {
		resp.Diagnostics.AddError("image factory client is not configured", "Please report this issue to the provider developers.")

		return
	}

	var config talosImageFactoryOverlaysVersionsDataSourceModelV0

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if config.TalosVersion.IsNull() || config.TalosVersion.IsUnknown() {
		return
	}

	overlaysInfo, err := d.imageFactoryClient.OverlaysVersions(ctx, config.TalosVersion.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("failed to get talos overlays versions", err.Error())

		return
	}

	if config.Filters != nil && !config.Filters.Name.IsNull() && !config.Filters.Name.IsUnknown() {
		overlaysInfo = xslices.Filter(overlaysInfo, func(e client.OverlayInfo) bool {
			return strings.Contains(e.Name, config.Filters.Name.ValueString())
		})
	}

	tfOverlaysInfo := xslices.Map(overlaysInfo, func(e client.OverlayInfo) overlayInfo {
		return overlayInfo{
			Name:   types.StringValue(e.Name),
			Image:  types.StringValue(e.Image),
			Ref:    types.StringValue(e.Ref),
			Digest: types.StringValue(e.Digest),
		}
	})

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "overlays_info")...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("overlays_info"), &tfOverlaysInfo)...)

	if resp.Diagnostics.HasError() {
		return
	}
}
