// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/siderolabs/gen/slices"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/generate"
	"gopkg.in/yaml.v3"
)

func resourceTalosClientConfiguration() *schema.Resource {
	return &schema.Resource{
		Description:   "Generate client configuration for a Talos cluster",
		CreateContext: resourceTalosClientConfigurationCreate,
		ReadContext:   resourceTalosClientConfigurationRead,
		UpdateContext: resourceTalosClientConfigurationUpdate,
		DeleteContext: resourceTalosClientConfigurationDelete,
		Schema: map[string]*schema.Schema{
			"cluster_name": {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "The name of the cluster in the generated config",
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"machine_secrets": {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "The machine secrets for the cluster",
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"nodes": {
				Type:        schema.TypeList,
				Description: "nodes to set in the generated config",
				Optional:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"endpoints": {
				Type:        schema.TypeList,
				Description: "endpoints to set in the generated config",
				Optional:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"talos_config": {
				Type:        schema.TypeString,
				Description: "The generated talos config",
				Computed:    true,
				Sensitive:   true,
			},
		},
	}
}

func resourceTalosClientConfigurationCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	clusterName := d.Get("cluster_name").(string)
	machineSecrets := d.Get("machine_secrets").(string)
	endpointsRaw := d.Get("endpoints").([]interface{})
	nodesRaw := d.Get("nodes").([]interface{})

	endpoints := slices.Map(endpointsRaw, func(val interface{}) string {
		return val.(string)
	})

	nodes := slices.Map(nodesRaw, func(val interface{}) string {
		return val.(string)
	})

	var secretsBundle *generate.SecretsBundle

	err := yaml.Unmarshal([]byte(machineSecrets), &secretsBundle)
	if err != nil {
		return diag.FromErr(err)
	}

	secretsBundle.Clock = generate.NewClock()

	talosConfig, err := generateTalosClientConfiguration(secretsBundle, clusterName, endpoints, nodes)
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("talos_config", talosConfig)

	d.SetId(clusterName)

	return resourceTalosClientConfigurationRead(ctx, d, meta)
}

func resourceTalosClientConfigurationRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func resourceTalosClientConfigurationUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	talosConfig := d.Get("talos_config").(string)

	cfg, err := clientconfig.FromString(talosConfig)
	if err != nil {
		return diag.FromErr(err)
	}

	if d.HasChange("endpoints") {
		cfg.Contexts[cfg.Context].Endpoints = slices.Map(d.Get("endpoints").([]interface{}), func(val interface{}) string {
			return val.(string)
		})
	}

	if d.HasChange("nodes") {
		cfg.Contexts[cfg.Context].Nodes = slices.Map(d.Get("nodes").([]interface{}), func(val interface{}) string {
			return val.(string)
		})
	}

	tc, err := cfg.Bytes()
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("talos_config", string(tc))

	return resourceTalosClientConfigurationRead(ctx, d, meta)
}

func resourceTalosClientConfigurationDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.SetId("")

	return nil
}
