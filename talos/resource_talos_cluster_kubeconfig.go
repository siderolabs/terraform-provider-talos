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
	"github.com/siderolabs/talos/pkg/machinery/client"
)

func resourceTalosClusterKubeconfig() *schema.Resource {
	return &schema.Resource{
		Description:   "Retrieve Kubeconfig for a Talos cluster",
		CreateContext: resourceTalosClusterKubeconfigCreate,
		ReadContext:   resourceTalosClusterKubeconfigRead,
		UpdateContext: resourceTalosClusterKubeconfigUpdate,
		DeleteContext: resourceTalosClusterKubeconfigDelete,
		Schema: map[string]*schema.Schema{
			"node": {
				Type:        schema.TypeString,
				Description: "node to use",
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
			"kube_config": {
				Type:        schema.TypeString,
				Description: "The retrieved Kubeconfig",
				Computed:    true,
				Sensitive:   true,
			},
		},
	}
}

func resourceTalosClusterKubeconfigCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var kubeConfig string

	talosConfig := d.Get("talos_config").(string)
	endpoint := d.Get("endpoint").(string)
	node := d.Get("node").(string)

	if err := resource.RetryContext(ctx, d.Timeout(schema.TimeoutCreate)-time.Minute, func() *resource.RetryError {
		if err := talosClientOp(ctx, endpoint, node, talosConfig, func(opContext context.Context, c *client.Client) error {
			kubeConfigBytes, err := c.Kubeconfig(opContext)
			if err != nil {
				return err
			}

			kubeConfig = string(kubeConfigBytes)

			return nil
		}); err != nil {
			return resource.RetryableError(err)
		}

		return nil
	}); err != nil {
		return diag.FromErr(err)
	}

	d.Set("kube_config", kubeConfig)

	d.SetId("kubeconfig")

	return resourceTalosClusterKubeconfigRead(ctx, d, meta)
}

func resourceTalosClusterKubeconfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func resourceTalosClusterKubeconfigUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return resourceTalosClusterKubeconfigCreate(ctx, d, meta)
}

func resourceTalosClusterKubeconfigDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.SetId("")

	return nil
}
