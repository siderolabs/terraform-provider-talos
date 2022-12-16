// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

func resourceTalosMachineBootstrap() *schema.Resource {
	return &schema.Resource{
		Description:   "Applies machine configuration to a Talos node.",
		CreateContext: resourceTalosMachineBootstrapCreate,
		ReadContext:   resourceTalosMachineBootstrapRead,
		UpdateContext: resourceTalosMachineBootstrapUpdate,
		DeleteContext: resourceTalosMachineBootstrapDelete,
		Schema: map[string]*schema.Schema{
			"node": {
				Type:        schema.TypeString,
				Description: "node to bootstrap",
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
		},
	}
}

func resourceTalosMachineBootstrapCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	endpoint := d.Get("endpoint").(string)
	node := d.Get("node").(string)
	talosConfig := d.Get("talos_config").(string)

	if err := resource.RetryContext(ctx, d.Timeout(schema.TimeoutCreate)-time.Minute, func() *resource.RetryError {
		if err := talosClientOp(ctx, endpoint, node, talosConfig, func(opContext context.Context, c *client.Client) error {
			if err := c.Bootstrap(opContext, &machineapi.BootstrapRequest{}); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return resource.RetryableError(err)
		}

		return nil
	}); err != nil {
		return diag.FromErr(err)
	}

	d.SetId("machine-bootstrap")

	return resourceTalosMachineBootstrapRead(ctx, d, meta)
}

func resourceTalosMachineBootstrapRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func resourceTalosMachineBootstrapUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return resourceTalosMachineBootstrapRead(ctx, d, meta)
}

func resourceTalosMachineBootstrapDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.SetId("")

	return nil
}
