// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1/generate"
	"github.com/talos-systems/talos/pkg/machinery/generic/slices"
	"gopkg.in/yaml.v3"
)

func dataSourceTalosClientConfiguration() *schema.Resource {
	return &schema.Resource{
		Description: "Generate client configuration for a Talos cluster",
		ReadContext: dataSourceTalosClientConfigurationRead,
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
				ForceNew:    true,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"endpoints": {
				Type:        schema.TypeList,
				Description: "endpoints to set in the generated config",
				Optional:    true,
				ForceNew:    true,
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

func dataSourceTalosClientConfigurationRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
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

	return nil
}

func generateTalosClientConfiguration(secretsBundle *generate.SecretsBundle, clusterName string, endpoints, nodes []string) (string, error) {
	generateInput, err := generate.NewInput(clusterName, "", "", secretsBundle)
	if err != nil {
		return "", err
	}

	talosConfig, err := generate.Talosconfig(generateInput)
	if err != nil {
		return "", err
	}

	if len(endpoints) > 0 {
		talosConfig.Contexts[talosConfig.Context].Endpoints = endpoints
	}

	if len(nodes) > 0 {
		talosConfig.Contexts[talosConfig.Context].Nodes = nodes
	}

	talosConfigBytes, err := talosConfig.Bytes()
	if err != nil {
		return "", err
	}

	return string(talosConfigBytes), nil
}
