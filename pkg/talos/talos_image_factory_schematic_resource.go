// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package talos is a Terraform provider for Talos.
package talos

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/image-factory/pkg/client"
	"github.com/siderolabs/image-factory/pkg/schematic"
	"gopkg.in/yaml.v3"
)

type talosImageFactorySchematicResource struct {
	imageFactoryClient *client.Client
}

var (
	_ resource.Resource              = &talosImageFactorySchematicResource{}
	_ resource.ResourceWithConfigure = &talosImageFactorySchematicResource{}
)

var schematicAttributeMarkdownDescription = `
The schematic yaml respresentation to generate the image.

If not set, a vanilla Talos image schematic will be generated.

> Refer to [image-factory](https://github.com/siderolabs/image-factory?tab=readme-ov-file#post-schematics) for the schema.
`

type talosImageFactorySchematicResourceModelV0 struct {
	ID        types.String `tfsdk:"id"`
	Schematic types.String `tfsdk:"schematic"`
}

// NewTalosImageFactorySchematicResource implements the resource.Resource interface.
func NewTalosImageFactorySchematicResource() resource.Resource {
	return &talosImageFactorySchematicResource{}
}

func (r *talosImageFactorySchematicResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_image_factory_schematic"
}

func (r *talosImageFactorySchematicResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The image factory schematic resource allows you to create a schematic for a Talos image.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique ID of the schematic, returned from Image Factory.",
			},
			"schematic": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: schematicAttributeMarkdownDescription,
			},
		},
	}
}

func (r *talosImageFactorySchematicResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	imageFactoryClient, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"failed to get image factory client",
			"Expected *client.Client, got: %T. Please report this issue to the provider developers.",
		)

		return
	}

	r.imageFactoryClient = imageFactoryClient
}

func (r *talosImageFactorySchematicResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config talosImageFactorySchematicResourceModelV0

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var schematic schematic.Schematic

	if err := yaml.Unmarshal([]byte(config.Schematic.ValueString()), &schematic); err != nil {
		resp.Diagnostics.AddError("failed to unmarshal schematic", err.Error())

		return
	}

	schematicID, err := r.imageFactoryClient.SchematicCreate(ctx, schematic)
	if err != nil {
		resp.Diagnostics.AddError("failed to create schematic", err.Error())

		return
	}

	config.ID = basetypes.NewStringValue(schematicID)

	resp.Diagnostics.Append(resp.State.Set(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *talosImageFactorySchematicResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *talosImageFactorySchematicResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *talosImageFactorySchematicResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan talosImageFactorySchematicResourceModelV0

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var schematic schematic.Schematic

	if err := yaml.Unmarshal([]byte(plan.Schematic.ValueString()), &schematic); err != nil {
		resp.Diagnostics.AddError("failed to unmarshal schematic", err.Error())

		return
	}

	schematicID, err := r.imageFactoryClient.SchematicCreate(ctx, schematic)
	if err != nil {
		resp.Diagnostics.AddError("failed to update schematic", err.Error())

		return
	}

	plan.ID = basetypes.NewStringValue(schematicID)

	// Set state to fully populated data
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}
}
