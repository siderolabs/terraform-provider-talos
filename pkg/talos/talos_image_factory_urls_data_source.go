// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"text/template"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/gen/xslices"
	"github.com/siderolabs/image-factory/pkg/client"
	"github.com/siderolabs/talos/pkg/machinery/platforms"
)

type talosImageFactoryURLSDataSource struct {
	imageFactoryClient *client.Client
}

var _ datasource.DataSource = &talosImageFactoryURLSDataSource{}

var (
	metalPlatforms                      = []string{"metal"}
	cloudPlatforms                      = xslices.Map(platforms.CloudPlatforms(), func(platform platforms.Platform) string { return platform.Name })
	sbcs                                = xslices.Map(platforms.SBCs(), func(platform platforms.SBC) string { return platform.Name })
	allPlatforms                        = slices.Concat(metalPlatforms, cloudPlatforms)
	platformMarkdownDescriptionTemplate = `
The platform for which the URLs are generated.

	#### Metal

		- metal

    #### Cloud Platforms

        {{- range .FactoryPlatforms }}
        - {{ . }}
        {{- end }}
`
	sbcMarkdownDescriptionTemplate = `
The SBC's (Single Board Copmuters) for which the url are generated.

    #### Single Board Computers

        {{- range .SBCs }}
        - {{ . }}
        {{- end }}
`
)

var (
	_ datasource.DataSource              = &talosImageFactoryURLSDataSource{}
	_ datasource.DataSourceWithConfigure = &talosImageFactoryURLSDataSource{}
)

type talosImageFactoryURLSDataSourceModelV0 struct {
	ID           types.String `tfsdk:"id"`
	Architecture types.String `tfsdk:"architecture"`
	TalosVersion types.String `tfsdk:"talos_version"`
	SchematicID  types.String `tfsdk:"schematic_id"`
	Platform     types.String `tfsdk:"platform"`
	SBC          types.String `tfsdk:"sbc"`
	URLs         urls         `tfsdk:"urls"`
}

type urls struct {
	Installer           types.String `tfsdk:"installer"`
	InstallerSecureboot types.String `tfsdk:"installer_secureboot"`
	ISO                 types.String `tfsdk:"iso"`
	ISOSecureboot       types.String `tfsdk:"iso_secureboot"`
	DiskImage           types.String `tfsdk:"disk_image"`
	DiskImageSecureboot types.String `tfsdk:"disk_image_secureboot"`
	PXE                 types.String `tfsdk:"pxe"`
	Kernel              types.String `tfsdk:"kernel"`
	KernelCommandLine   types.String `tfsdk:"kernel_command_line"`
	Initramfs           types.String `tfsdk:"initramfs"`
	UKI                 types.String `tfsdk:"uki"`
}

// NewTalosImageFactoryURLSDataSource implements the datasource.Datasource interface.
func NewTalosImageFactoryURLSDataSource() datasource.DataSource {
	return &talosImageFactoryURLSDataSource{}
}

func (d *talosImageFactoryURLSDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_image_factory_urls"
}

func (d *talosImageFactoryURLSDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	var platformMarkdownDescription strings.Builder

	template.Must(template.New("platformMarkdownDescription").Parse(platformMarkdownDescriptionTemplate)).Execute(&platformMarkdownDescription, struct { //nolint:errcheck
		FactoryPlatforms []string
	}{
		FactoryPlatforms: cloudPlatforms,
	})

	var sbcMarkdownDescription strings.Builder

	template.Must(template.New("sbcMarkdownDescription").Parse(sbcMarkdownDescriptionTemplate)).Execute(&sbcMarkdownDescription, struct { //nolint:errcheck
		SBCs []string
	}{
		SBCs: sbcs,
	})

	resp.Schema = schema.Schema{
		Description: "Generates URLs for different assets supported by the Talos image factory.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"architecture": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The platform architecture for which the URLs are generated. Defaults to amd64.",
				Validators: []validator.String{
					stringvalidator.OneOf("amd64", "arm64"),
				},
			},
			"talos_version": schema.StringAttribute{
				Required:    true,
				Description: "The Talos version for which the URLs are generated.",
			},
			"schematic_id": schema.StringAttribute{
				Required:    true,
				Description: "The schematic ID for which the URLs are generated.",
			},
			"platform": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: platformMarkdownDescription.String(),
				Validators: []validator.String{
					stringvalidator.All(
						stringvalidator.OneOf(allPlatforms...),
						stringvalidator.ExactlyOneOf(path.Expressions{
							path.MatchRoot("sbc"),
						}...),
					),
				},
			},
			"sbc": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: sbcMarkdownDescription.String(),
				Validators: []validator.String{
					stringvalidator.All(
						stringvalidator.OneOf(sbcs...),
						stringvalidator.ExactlyOneOf(path.Expressions{
							path.MatchRoot("platform"),
						}...),
					),
				},
			},
			"urls": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The URLs for different assets supported by the Talos image factory. If the URL is not available for a specific asset, it will be an empty string.",
				Attributes: map[string]schema.Attribute{
					"installer": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the installer image.",
					},
					"installer_secureboot": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the installer image with secure boot.",
					},
					"iso": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the ISO image.",
					},
					"iso_secureboot": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the ISO image with secure boot.",
					},
					"disk_image": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the disk image.",
					},
					"disk_image_secureboot": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the disk image with secure boot.",
					},
					"pxe": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the PXE image.",
					},
					"kernel": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the kernel image.",
					},
					"kernel_command_line": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the kernel command line.",
					},
					"initramfs": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the initramfs image.",
					},
					"uki": schema.StringAttribute{
						Computed:    true,
						Description: "The URL for the UKI image.",
					},
				},
			},
		},
	}
}

func (d *talosImageFactoryURLSDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *talosImageFactoryURLSDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.imageFactoryClient == nil {
		resp.Diagnostics.AddError("image factory client is not configured", "Please report this issue to the provider developers.")

		return
	}

	var obj types.Object

	resp.Diagnostics.Append(req.Config.Get(ctx, &obj)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var config talosImageFactoryURLSDataSourceModelV0

	resp.Diagnostics.Append(obj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})...)

	if resp.Diagnostics.HasError() {
		return
	}

	if config.Architecture.IsNull() {
		config.Architecture = basetypes.NewStringValue("amd64")
	}

	architecture := config.Architecture.ValueString()
	platform := config.Platform.ValueString()
	talosVersion := config.TalosVersion.ValueString()
	schematicID := config.SchematicID.ValueString()

	uri, err := url.Parse(d.imageFactoryClient.BaseURL())
	if err != nil {
		resp.Diagnostics.AddError("failed to parse image factory base URL", err.Error())

		return
	}

	urlsData := urls{
		Installer: basetypes.NewStringValue(fmt.Sprintf("%s/%s-installer/%s:%s", uri.Host, platform, schematicID, talosVersion)),
	}

	switch platform {
	case "metal":
		platformData := platforms.MetalPlatform()

		urlsData.InstallerSecureboot = basetypes.NewStringValue(fmt.Sprintf("%s/%s-installer-secureboot/%s:%s", uri.Host, platform, schematicID, talosVersion))
		urlsData.ISO = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData.ISOPath(architecture)))
		urlsData.ISOSecureboot = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData.SecureBootISOPath(architecture)))
		urlsData.DiskImage = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData.DiskImageDefaultPath(architecture)))
		urlsData.DiskImageSecureboot = basetypes.NewStringValue(
			fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData.SecureBootDiskImageDefaultPath(architecture)),
		)
		urlsData.PXE = basetypes.NewStringValue(fmt.Sprintf("%s://pxe.%s/pxe/%s/%s/%s", uri.Scheme, uri.Host, schematicID, talosVersion, platformData.PXEScriptPath(architecture)))
		urlsData.Kernel = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData.KernelPath(architecture)))
		urlsData.KernelCommandLine = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData.CmdlinePath(architecture)))
		urlsData.Initramfs = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData.InitramfsPath(architecture)))
		urlsData.UKI = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData.SecureBootUKIPath(architecture)))
	case "": // empty platform means it's an SBC
		urlsData.Installer = basetypes.NewStringValue(fmt.Sprintf("%s/metal-installer/%s:%s", uri.Host, schematicID, talosVersion))
		urlsData.DiskImage = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/metal-arm64.raw.xz", d.imageFactoryClient.BaseURL(), schematicID, talosVersion))
	default:
		platformData := xslices.Filter(platforms.CloudPlatforms(), func(p platforms.Platform) bool { return p.Name == platform })

		if len(platformData) != 1 {
			resp.Diagnostics.AddError("failed to find platform", platform)

			return
		}

		if platformData[0].SecureBootSupported {
			urlsData.InstallerSecureboot = basetypes.NewStringValue(fmt.Sprintf("%s/installer-secureboot/%s:%s", uri.Host, schematicID, talosVersion))
		}

		for _, bootMethod := range platformData[0].BootMethods {
			switch bootMethod {
			case platforms.BootMethodDiskImage:
				urlsData.DiskImage = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData[0].DiskImageDefaultPath(architecture))) //nolint:lll
				if platformData[0].SecureBootSupported {
					urlsData.DiskImageSecureboot = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData[0].SecureBootDiskImageDefaultPath(architecture))) //nolint:lll
				}
			case platforms.BootMethodPXE:
				urlsData.PXE = basetypes.NewStringValue(fmt.Sprintf("%s://pxe.%s/pxe/%s/%s/%s", uri.Scheme, uri.Host, schematicID, talosVersion, platformData[0].PXEScriptPath(architecture))) //nolint:lll
			case platforms.BootMethodISO:
				urlsData.ISO = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData[0].ISOPath(architecture)))
				if platformData[0].SecureBootSupported {
					urlsData.ISOSecureboot = basetypes.NewStringValue(fmt.Sprintf("%s/image/%s/%s/%s", d.imageFactoryClient.BaseURL(), schematicID, talosVersion, platformData[0].SecureBootISOPath(architecture))) //nolint:lll
				}
			}
		}
	}

	config.ID = basetypes.NewStringValue(config.ID.ValueString())
	config.URLs = urlsData

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}
}
