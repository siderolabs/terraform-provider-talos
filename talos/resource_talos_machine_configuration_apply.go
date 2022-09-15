// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	machineapi "github.com/talos-systems/talos/pkg/machinery/api/machine"
	"github.com/talos-systems/talos/pkg/machinery/client"
	"github.com/talos-systems/talos/pkg/machinery/config/configpatcher"
	"github.com/talos-systems/talos/pkg/machinery/generic/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func resourceTalosMachineConfigurationApply() *schema.Resource {
	return &schema.Resource{
		Description:   "Applies machine configuration to a Talos node.",
		CreateContext: resourceTalosMachineConfigurationApplyCreate,
		ReadContext:   resourceTalosMachineConfigurationApplyRead,
		UpdateContext: resourceTalosMachineConfigurationApplyUpdate,
		DeleteContext: resourceTalosMachineConfigurationApplyDelete,
		Schema: map[string]*schema.Schema{
			"mode": {
				Type:        schema.TypeString,
				Description: "The mode to apply the configuration in.",
				Optional:    true,
				Default:     "auto",
				ValidateFunc: validation.StringInSlice([]string{
					"auto",
					"no_reboot",
					"reboot",
					"staged",
				}, true),
			},
			"node": {
				Type:        schema.TypeString,
				Description: "node to apply the config against",
				Required:    true,
			},
			"endpoint": {
				Type:        schema.TypeString,
				Description: "machine endpoint",
				Required:    true,
			},
			"talos_config": {
				Type:        schema.TypeString,
				Description: "talos client configuration for authentication",
				Required:    true,
			},
			"machine_configuration": {
				Type:        schema.TypeString,
				Description: "machine configuration",
				Required:    true,
			},
			"config_patches": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "config patches to apply to the generated config",
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
		},
	}
}

func resourceTalosMachineConfigurationApplyCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	machineConfig := d.Get("machine_configuration").(string)
	talosConfig := d.Get("talos_config").(string)
	applyMode := d.Get("mode").(string)
	endpoint := d.Get("endpoint").(string)
	node := d.Get("node").(string)

	if val, ok := d.GetOk("config_patches"); ok {
		configPatchesRaw := val.([]interface{})

		configPatches := slices.Map(configPatchesRaw, func(val interface{}) string {
			return val.(string)
		})

		patches, err := configpatcher.LoadPatches(configPatches)
		if err != nil {
			return diag.FromErr(err)
		}

		cfg, err := configpatcher.Apply(configpatcher.WithBytes([]byte(machineConfig)), patches)
		if err != nil {
			return diag.FromErr(err)
		}

		cfgBytes, err := cfg.Bytes()
		if err != nil {
			return diag.FromErr(err)
		}

		machineConfig = string(cfgBytes)

	}

	if err := resource.RetryContext(ctx, d.Timeout(schema.TimeoutCreate)-time.Minute, func() *resource.RetryError {
		if err := talosClientOp(ctx, endpoint, node, talosConfig, func(ctx context.Context, c *client.Client) error {
			_, err := c.ApplyConfiguration(ctx, &machineapi.ApplyConfigurationRequest{
				Mode: machineapi.ApplyConfigurationRequest_Mode(machineapi.ApplyConfigurationRequest_Mode_value[strings.ToUpper(applyMode)]),
				Data: []byte(machineConfig),
			})
			if err != nil {
				return err
			}

			return nil
		}); err != nil {
			// TODO: remove status.Unknown check once we have 1.2.3
			if s := status.Code(err); s == codes.InvalidArgument || s == codes.Unknown {
				return resource.NonRetryableError(err)
			}

			return resource.RetryableError(err)
		}

		return nil
	}); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("machine-configuration-apply")

	return resourceTalosMachineConfigurationApplyRead(ctx, d, meta)
}

func resourceTalosMachineConfigurationApplyRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func resourceTalosMachineConfigurationApplyUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return resourceTalosMachineConfigurationApplyCreate(ctx, d, meta)
}

func resourceTalosMachineConfigurationApplyDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.SetId("")

	return nil
}
