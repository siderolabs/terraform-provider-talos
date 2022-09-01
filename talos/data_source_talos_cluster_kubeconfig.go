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
	"github.com/talos-systems/talos/pkg/machinery/client"
	"github.com/talos-systems/talos/pkg/machinery/generic/slices"
)

func dataSourceTalosClusterKubeconfig() *schema.Resource {
	return &schema.Resource{
		Description: "Retrieve Kubeconfig for a Talos cluster",
		ReadContext: dataSourceTalosClusterKubeconfigRead,
		Schema: map[string]*schema.Schema{
			"nodes": {
				Type:        schema.TypeList,
				Description: "nodes to use",
				Optional:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"endpoints": {
				Type:        schema.TypeList,
				Description: "endpoints to use",
				Optional:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"talos_config": {
				Type:        schema.TypeString,
				Description: "talos client configuration for authentication",
				Required:    true,
				ForceNew:    true,
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

func dataSourceTalosClusterKubeconfigRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var nodes, endpoints []string
	var kubeConfig string
	talosConfig := d.Get("talos_config").(string)

	if val, ok := d.GetOk("nodes"); ok {
		nodesRaw := val.([]interface{})
		nodes = slices.Map(nodesRaw, func(val interface{}) string {
			return val.(string)
		})
	}

	if val, ok := d.GetOk("endpoints"); ok {
		endpointsRaw := val.([]interface{})
		endpoints = slices.Map(endpointsRaw, func(val interface{}) string {
			return val.(string)
		})
	}

	if err := resource.RetryContext(ctx, d.Timeout(schema.TimeoutCreate)-time.Minute, func() *resource.RetryError {
		if err := talosClientOp(ctx, endpoints, nodes, talosConfig, func(c *client.Client) error {
			kubeConfigBytes, err := c.Kubeconfig(ctx)
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

	return nil
}
