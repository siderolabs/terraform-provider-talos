// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/gen/maps"
	"github.com/siderolabs/go-blockdevice/blockdevice/util/disk"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// const (
// 	nodiskFoundError = "no disk matching the filter found"
// )

func (e nodiskFoundError) Error() string {
	return "no disk matching the filter found"
}

type nodiskFoundError struct{}

type talosMachineDisksDataSource struct{}

type talosMachineDisksDataSourceModelV0 struct { //nolint:govet
	ID                  types.String           `tfsdk:"id"`
	Node                types.String           `tfsdk:"node"`
	Endpoint            types.String           `tfsdk:"endpoint"`
	ClientConfiguration clientConfiguration    `tfsdk:"client_configuration"`
	Filters             talosMachineDiskFilter `tfsdk:"filters"`
	Disks               []talosMachineDisk     `tfsdk:"disks"`
	Timeouts            timeouts.Value         `tfsdk:"timeouts"`
}

type talosMachineDisk struct {
	Size     types.String `tfsdk:"size"`
	Name     types.String `tfsdk:"name"`
	Model    types.String `tfsdk:"model"`
	Serial   types.String `tfsdk:"serial"`
	Modalias types.String `tfsdk:"modalias"`
	UUID     types.String `tfsdk:"uuid"`
	WWID     types.String `tfsdk:"wwid"`
	Type     types.String `tfsdk:"type"`
	BusPath  types.String `tfsdk:"bus_path"`
}

type talosMachineDiskFilter struct {
	Size     types.String `tfsdk:"size"`
	Name     types.String `tfsdk:"name"`
	Model    types.String `tfsdk:"model"`
	Serial   types.String `tfsdk:"serial"`
	Modalias types.String `tfsdk:"modalias"`
	UUID     types.String `tfsdk:"uuid"`
	WWID     types.String `tfsdk:"wwid"`
	Type     types.String `tfsdk:"type"`
	BusPath  types.String `tfsdk:"bus_path"`
}

type diskSizeMatcher struct {
	Op   string
	Size uint64
}

func (m *diskSizeMatcher) Matcher(d *disk.Disk) bool {
	switch m.Op {
	case ">=":
		return d.Size >= m.Size
	case "<=":
		return d.Size <= m.Size
	case ">":
		return d.Size > m.Size
	case "<":
		return d.Size < m.Size
	case "":
		fallthrough
	case "==":
		return d.Size == m.Size
	default:
		return false
	}
}

var (
	_ datasource.DataSource                   = &talosMachineDisksDataSource{}
	_ datasource.DataSourceWithValidateConfig = &talosMachineDisksDataSource{}
)

// NewTalosMachineDisksDataSource implements the datasource.DataSource interface.
func NewTalosMachineDisksDataSource() datasource.DataSource {
	return &talosMachineDisksDataSource{}
}

func (d *talosMachineDisksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_disks"
}

func (d *talosMachineDisksDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generate a machine configuration for a node type",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The generated ID of this resource",
				Computed:    true,
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
			"filters": schema.SingleNestedAttribute{
				Description: "Filters to apply to the disks",
				Attributes: map[string]schema.Attribute{
					"size": schema.StringAttribute{
						Description: "Filter disks by size",
						Optional:    true,
					},
					"name": schema.StringAttribute{
						Description: "Filter disks by name",
						Optional:    true,
					},
					"model": schema.StringAttribute{
						Description: "Filter disks by model",
						Optional:    true,
					},
					"serial": schema.StringAttribute{
						Description: "Filter disks by serial number",
						Optional:    true,
					},
					"modalias": schema.StringAttribute{
						Description: "Filter disks by modalias",
						Optional:    true,
					},
					"uuid": schema.StringAttribute{
						Description: "Filter disks by uuid",
						Optional:    true,
					},
					"wwid": schema.StringAttribute{
						Description: "Filter disks by wwid",
						Optional:    true,
					},
					"type": schema.StringAttribute{
						Description: "Filter disks by type",
						Optional:    true,
					},
					"bus_path": schema.StringAttribute{
						Description: "Filter disks by bus path",
						Optional:    true,
					},
				},
				Optional: true,
			},
			"disks": schema.ListNestedAttribute{
				Description: "The disks that match the filters",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"size": schema.StringAttribute{
							Description: "The size of the disk",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The name of the disk",
							Computed:    true,
						},
						"model": schema.StringAttribute{
							Description: "The model of the disk",
							Computed:    true,
						},
						"serial": schema.StringAttribute{
							Description: "The serial number of the disk",
							Computed:    true,
						},
						"modalias": schema.StringAttribute{
							Description: "The modalias of the disk",
							Computed:    true,
						},
						"uuid": schema.StringAttribute{
							Description: "The uuid of the disk",
							Computed:    true,
						},
						"wwid": schema.StringAttribute{
							Description: "The wwid of the disk",
							Computed:    true,
						},
						"type": schema.StringAttribute{
							Description: "The type of the disk",
							Computed:    true,
						},
						"bus_path": schema.StringAttribute{
							Description: "The bus path of the disk",
							Computed:    true,
						},
					},
				},
				Computed: true,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Read: true,
			}),
		},
	}
}

func (d *talosMachineDisksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) { //nolint:gocognit,gocyclo,cyclop
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state talosMachineDisksDataSourceModelV0
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

	matchers := []disk.Matcher{}

	if state.Filters.Size.ValueString() != "" {
		matcher, err := parseSizeFilter(state.Filters.Size.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("failed to parse size filter", err.Error())

			return
		}

		matchers = append(matchers, matcher.Matcher)
	}

	if state.Filters.Name.ValueString() != "" {
		matchers = append(matchers, disk.WithName(state.Filters.Name.ValueString()))
	}

	if state.Filters.Model.ValueString() != "" {
		matchers = append(matchers, disk.WithModel(state.Filters.Model.ValueString()))
	}

	if state.Filters.Serial.ValueString() != "" {
		matchers = append(matchers, disk.WithSerial(state.Filters.Serial.ValueString()))
	}

	if state.Filters.Modalias.ValueString() != "" {
		matchers = append(matchers, disk.WithModalias(state.Filters.Modalias.ValueString()))
	}

	if state.Filters.UUID.ValueString() != "" {
		matchers = append(matchers, disk.WithUUID(state.Filters.UUID.ValueString()))
	}

	if state.Filters.WWID.ValueString() != "" {
		matchers = append(matchers, disk.WithWWID(state.Filters.WWID.ValueString()))
	}

	if state.Filters.Type.ValueString() != "" {
		diskType, err := disk.ParseType(state.Filters.Type.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("failed to parse disk type", err.Error())

			return
		}

		matchers = append(matchers, disk.WithType(diskType))
	}

	if state.Filters.BusPath.ValueString() != "" {
		matchers = append(matchers, disk.WithBusPath(state.Filters.BusPath.ValueString()))
	}

	if err := retry.RetryContext(ctxDeadline, readTimeout, func() *retry.RetryError {
		if err := talosClientOp(ctx, state.Endpoint.ValueString(), state.Node.ValueString(), talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			diskResp, err := c.Disks(nodeCtx)
			if err != nil {
				return err
			}

			foundDisks := make([]disk.Disk, len(diskResp.Messages[0].Disks))

			for i, diskResp := range diskResp.Messages[0].Disks {
				foundDisks[i] = disk.Disk{
					Name:     diskResp.DeviceName,
					Model:    diskResp.Model,
					Serial:   diskResp.Serial,
					Modalias: diskResp.Modalias,
					Size:     diskResp.Size,
					UUID:     diskResp.Uuid,
					WWID:     diskResp.Wwid,
					Type:     disk.Type(diskResp.Type),
					BusPath:  diskResp.BusPath,
				}
			}

			if len(matchers) > 0 {
				for _, foundDisk := range foundDisks {
					if disk.Match(&foundDisk, matchers...) {
						state.Disks = append(state.Disks, talosMachineDisk{
							Size:     basetypes.NewStringValue(humanize.Bytes(foundDisk.Size)),
							Name:     basetypes.NewStringValue(foundDisk.Name),
							Model:    basetypes.NewStringValue(foundDisk.Model),
							Serial:   basetypes.NewStringValue(foundDisk.Serial),
							Modalias: basetypes.NewStringValue(foundDisk.Modalias),
							UUID:     basetypes.NewStringValue(foundDisk.UUID),
							WWID:     basetypes.NewStringValue(foundDisk.WWID),
							Type:     basetypes.NewStringValue(foundDisk.Type.String()),
							BusPath:  basetypes.NewStringValue(foundDisk.BusPath),
						})

						// if there was a filter and we found a match, we can stop looking
						return nil
					}
				}

				// if there was a filter and we didn't find a match, we can stop looking and return an error
				return nodiskFoundError{}
			}

			// if there was no filter, we can return all disks
			for _, foundDisk := range foundDisks {
				state.Disks = append(state.Disks, talosMachineDisk{
					Size:     basetypes.NewStringValue(humanize.Bytes(foundDisk.Size)),
					Name:     basetypes.NewStringValue(foundDisk.Name),
					Model:    basetypes.NewStringValue(foundDisk.Model),
					Serial:   basetypes.NewStringValue(foundDisk.Serial),
					Modalias: basetypes.NewStringValue(foundDisk.Modalias),
					UUID:     basetypes.NewStringValue(foundDisk.UUID),
					WWID:     basetypes.NewStringValue(foundDisk.WWID),
					Type:     basetypes.NewStringValue(foundDisk.Type.String()),
					BusPath:  basetypes.NewStringValue(foundDisk.BusPath),
				})
			}

			return nil
		}); err != nil {
			if s := status.Code(err); s == codes.InvalidArgument {
				return retry.NonRetryableError(err)
			}

			if errors.Is(err, nodiskFoundError{}) {
				return retry.NonRetryableError(err)
			}

			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		resp.Diagnostics.AddError("failed to get list of disks", err.Error())

		return
	}

	state.ID = basetypes.NewStringValue("machine_disks")

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (d *talosMachineDisksDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state talosMachineDisksDataSourceModelV0
	diags = obj.As(ctx, &state, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if !state.Filters.Type.IsUnknown() && !state.Filters.Type.IsNull() {
		if _, ok := diskTypeMap()[state.Filters.Type.ValueString()]; !ok {
			resp.Diagnostics.AddError("invalid disk type, disk type must be one of: %s", strings.Join(maps.Keys(diskTypeMap()), ", "))
		}
	}

	if !state.Filters.Size.IsUnknown() && !state.Filters.Size.IsNull() {
		if _, err := parseSizeFilter(state.Filters.Size.ValueString()); err != nil {
			resp.Diagnostics.AddError("invalid disk size filter: %s", err.Error())
		}
	}
}

func diskTypeMap() map[string]struct{} {
	return map[string]struct{}{
		"ssd":  {},
		"hdd":  {},
		"nvme": {},
		"sd":   {},
	}
}

func parseSizeFilter(filter string) (*diskSizeMatcher, error) {
	filter = strings.TrimSpace(filter)

	re := regexp.MustCompile(`(>=|<=|>|<|==)?\b*(.*)$`)

	parts := re.FindStringSubmatch(filter)
	if len(parts) < 2 {
		return nil, fmt.Errorf("failed to parse the condition: expected [>=|<=|>|<|==]<size>[units], got %s", filter)
	}

	var op string

	switch parts[1] {
	case ">=", "<=", ">", "<", "", "==":
		op = parts[1]
	default:
		return nil, fmt.Errorf("unknown binary operator %s", parts[1])
	}

	size, err := humanize.ParseBytes(strings.TrimSpace(parts[2]))
	if err != nil {
		return nil, fmt.Errorf("failed to parse disk size %s: %w", parts[2], err)
	}

	return &diskSizeMatcher{
		Op:   op,
		Size: size,
	}, nil
}
