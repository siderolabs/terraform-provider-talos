// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
	"github.com/siderolabs/gen/slices"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
)

func resourceTalosMachineConfigurationWorker() *schema.Resource {
	return &schema.Resource{
		Description:   "Generate machine configuration for a Talos worker node.",
		CreateContext: resourceTalosMachineConfigurationWorkerCreate,
		ReadContext:   resourceTalosMachineConfigurationWorkerRead,
		UpdateContext: resourceTalosMachineConfigurationWorkerUpdate,
		DeleteContext: resourceTalosMachineConfigurationWorkerDelete,
		Schema: map[string]*schema.Schema{
			"cluster_name": {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "The name of the cluster in the generated config",
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"cluster_endpoint": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The endpoint of the Talos cluster",
				ValidateDiagFunc: func(v interface{}, p cty.Path) diag.Diagnostics {
					value := v.(string)
					if err := validateClusterEndpoint(value); err != nil {
						return diag.FromErr(err)
					}
					return nil
				},
			},
			"machine_secrets": {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "The machine secrets for the cluster",
				ValidateFunc: validation.StringIsNotEmpty,
			},
			"config_patches": {
				Type:        schema.TypeList,
				Optional:    true,
				Description: "config patches to apply to the generated config",
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
			},
			"kubernetes_version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "desired kubernetes version to run",
				Default:     constants.DefaultKubernetesVersion,
			},
			"talos_version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The version of Talos for which to generate configs. **Note**: This parameter defines the desired Talos schema version of configuration generated for backwards compatibility. To change the version of Talos installed, please use a config patch.",
				ValidateDiagFunc: func(v interface{}, p cty.Path) diag.Diagnostics {
					value := v.(string)
					_, err := validateVersionContract(value)
					if err != nil {
						return diag.FromErr(err)
					}

					return nil
				},
			},
			"config_version": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "the desired machine config version to generate (the `version` field of the generated config, currently `v1alpha1`)",
				Default:     "v1alpha1",
				ValidateDiagFunc: func(i interface{}, p cty.Path) diag.Diagnostics {
					v := i.(string)
					if v != "v1alpha1" {
						return diag.Errorf("invalid config version %q", v)
					}

					return nil
				},
			},
			"docs_enabled": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "whether to render all machine configs adding the documentation for each field",
				Default:     true,
			},
			"examples_enabled": {
				Type:        schema.TypeBool,
				Optional:    true,
				Description: "whether to render all machine configs with the commented examples",
				Default:     true,
			},
			"machine_config": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "the generated worker config",
				Sensitive:   true,
			},
		},
	}
}

func resourceTalosMachineConfigurationWorkerCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	clusterName := d.Get("cluster_name").(string)
	clusterEndpoint := d.Get("cluster_endpoint").(string)
	machineSecrets := d.Get("machine_secrets").(string)
	kubernetesVersion := d.Get("kubernetes_version").(string)
	docsEnabled := d.Get("docs_enabled").(bool)
	examplesEnabled := d.Get("examples_enabled").(bool)

	genOptions := &machineConfigGenerateOptions{
		machineType:       machine.TypeWorker,
		clusterName:       clusterName,
		clusterEndpoint:   clusterEndpoint,
		machineSecrets:    machineSecrets,
		kubernetesVersion: kubernetesVersion,
		docsEnabled:       docsEnabled,
		examplesEnabled:   examplesEnabled,
	}

	if val, ok := d.GetOk("talos_version"); ok {
		talosVersion := val.(string)
		genOptions.talosVersion = talosVersion
	}

	if val, ok := d.GetOk("config_patches"); ok {
		configPatchesRaw := val.([]interface{})

		genOptions.configPatches = slices.Map(configPatchesRaw, func(val interface{}) string {
			return val.(string)
		})
	}

	workerConfig, err := genOptions.generate()
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(clusterName)

	d.Set("machine_config", workerConfig)

	return resourceTalosMachineConfigurationWorkerRead(ctx, d, meta)
}

func resourceTalosMachineConfigurationWorkerRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func resourceTalosMachineConfigurationWorkerUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return resourceTalosMachineConfigurationWorkerCreate(ctx, d, meta)
}

func resourceTalosMachineConfigurationWorkerDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.SetId("")

	return nil
}
