// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1/generate"
	"gopkg.in/yaml.v3"
)

func resourceTalosMachineSecrets() *schema.Resource {
	return &schema.Resource{
		Description:   "Generate machine secrets for a Talos cluster",
		CreateContext: resourceTalosMachineSecretsCreate,
		DeleteContext: resourceTalosMachineSecretsDelete,
		ReadContext:   resourceTalosMachineSecretsRead,
		Schema: map[string]*schema.Schema{
			"talos_version": {
				Type:        schema.TypeString,
				Optional:    true,
				ForceNew:    true,
				Description: "The version of Talos for which to generate secrets",
				ValidateDiagFunc: func(v interface{}, p cty.Path) diag.Diagnostics {
					value := v.(string)

					_, err := validateVersionContract(value)
					if err != nil {
						return diag.FromErr(err)
					}

					return nil
				},
			},
			"machine_secrets": {
				Type:        schema.TypeString,
				Computed:    true,
				Description: "The generated talos cluster secrets",
				Sensitive:   true,
			},
		},
	}
}

func resourceTalosMachineSecretsCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	genOptions := make([]generate.GenOption, 0, 1)

	if val, ok := d.GetOk("talos_version"); ok {
		talosVersion := val.(string)

		versionContract, err := validateVersionContract(talosVersion)
		if err != nil {
			return diag.FromErr(err)
		}

		genOptions = append(genOptions, generate.WithVersionContract(versionContract))
	}

	secretsBundle, err := generate.NewSecretsBundle(generate.NewClock(), genOptions...)
	if err != nil {
		return diag.FromErr(err)
	}

	secrets, err := yaml.Marshal(secretsBundle)
	if err != nil {
		return diag.FromErr(err)
	}

	d.Set("machine_secrets", string(secrets))
	d.SetId(secretsBundle.Cluster.ID)

	return resourceTalosMachineSecretsRead(ctx, d, meta)
}

func resourceTalosMachineSecretsRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	return nil
}

func resourceTalosMachineSecretsDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	d.SetId("")

	return nil
}
