// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/siderolabs/gen/xslices"
	"github.com/siderolabs/image-factory/pkg/client"
)

type talosImageFactoryExtensionsVersionsDataSource struct {
	imageFactoryClient *client.Client
}

type talosImageFactoryExtensionsVersionsDataSourceModelV0 struct {
	ID             types.String                               `tfsdk:"id"`
	TalosVersion   types.String                               `tfsdk:"talos_version"`
	Filters        *talosImageFactoryExtensionsVersionsFilter `tfsdk:"filters"`
	ExactFilters   *talosImageFactoryExtensionsVersionsFilter `tfsdk:"exact_filters"`
	ExtensionsInfo []extensionInfo                            `tfsdk:"extensions_info"`
}

type talosImageFactoryExtensionsVersionsFilter struct {
	Names types.List `tfsdk:"names"`
}

type extensionInfo struct {
	Name        types.String `tfsdk:"name"`
	Ref         types.String `tfsdk:"ref"`
	Digest      types.String `tfsdk:"digest"`
	Author      types.String `tfsdk:"author"`
	Description types.String `tfsdk:"description"`
}

var _ datasource.DataSourceWithConfigure = &talosImageFactoryExtensionsVersionsDataSource{}

// NewTalosImageFactoryExtensionsVersionsDataSource implements the datasource.DataSource interface.
func NewTalosImageFactoryExtensionsVersionsDataSource() datasource.DataSource {
	return &talosImageFactoryExtensionsVersionsDataSource{}
}

func (d *talosImageFactoryExtensionsVersionsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_image_factory_extensions_versions"
}

func (d *talosImageFactoryExtensionsVersionsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The image factory extensions versions data source provides a list of available extensions for a specific talos version from the image factory.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"talos_version": schema.StringAttribute{
				Required:    true,
				Description: "The talos version to get extensions for.",
			},
			"filters": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "The filter to apply to the extensions list.",
				Attributes: map[string]schema.Attribute{
					"names": schema.ListAttribute{
						ElementType: types.StringType,
						Optional:    true,
						Description: "The name of the extension to filter by.",
					},
				},
			},
			"exact_filters": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "The filter to apply to the extensions list.",
				Attributes: map[string]schema.Attribute{
					"names": schema.ListAttribute{
						ElementType: types.StringType,
						Optional:    true,
						Description: "The exact name match of the extension to filter by.",
					},
				},
			},
			"extensions_info": schema.ListAttribute{
				ElementType: types.ObjectType{
					AttrTypes: map[string]attr.Type{
						"name":        types.StringType,
						"ref":         types.StringType,
						"digest":      types.StringType,
						"author":      types.StringType,
						"description": types.StringType,
					},
				},
				Computed:    true,
				Description: "The list of available extensions for the specified talos version.",
			},
		},
	}
}

func (d *talosImageFactoryExtensionsVersionsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

//nolint:gocyclo,cyclop,gocognit
func (d *talosImageFactoryExtensionsVersionsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.imageFactoryClient == nil {
		resp.Diagnostics.AddError("image factory client is not configured", "Please report this issue to the provider developers.")

		return
	}

	var config talosImageFactoryExtensionsVersionsDataSourceModelV0

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if config.TalosVersion.IsNull() || config.TalosVersion.IsUnknown() {
		return
	}

	extensionsInfo, err := d.imageFactoryClient.ExtensionsVersions(ctx, config.TalosVersion.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("failed to get talos extensions versions", err.Error())

		return
	}

	var exactNames, names []string
	if config.ExactFilters != nil && !config.ExactFilters.Names.IsNull() && !config.ExactFilters.Names.IsUnknown() {
		resp.Diagnostics.Append(config.ExactFilters.Names.ElementsAs(ctx, &exactNames, true)...)

		if resp.Diagnostics.HasError() {
			return
		}
	}

	if config.Filters != nil && !config.Filters.Names.IsNull() && !config.Filters.Names.IsUnknown() {
		resp.Diagnostics.Append(config.Filters.Names.ElementsAs(ctx, &names, true)...)

		if resp.Diagnostics.HasError() {
			return
		}
	}

	if len(exactNames) > 0 || len(names) > 0 {
		extensionsInfo = xslices.Filter(extensionsInfo, func(e client.ExtensionInfo) bool {
			if len(exactNames) > 0 {
				if slices.Contains(exactNames, e.Name) {
					return true
				}
			}

			if len(names) > 0 {
				for _, n := range names {
					if strings.Contains(e.Name, n) {
						return true
					}
				}
			}

			return false
		})
	}

	tfExtensionsInfo := xslices.Map(extensionsInfo, func(e client.ExtensionInfo) extensionInfo {
		return extensionInfo{
			Name:        types.StringValue(e.Name),
			Ref:         types.StringValue(e.Ref),
			Digest:      types.StringValue(e.Digest),
			Author:      types.StringValue(e.Author),
			Description: types.StringValue(e.Description),
		}
	})

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), "extensions_info")...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("extensions_info"), &tfExtensionsInfo)...)

	if resp.Diagnostics.HasError() {
		return
	}
}
