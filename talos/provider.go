// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// Provider -
func Provider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"talos_machine_secrets":                    resourceTalosMachineSecrets(),
			"talos_machine_configuration_controlplane": resourceTalosMachineConfigurationControlPlane(),
			"talos_machine_configuration_worker":       resourceTalosMachineConfigurationWorker(),
		},
		DataSourcesMap: map[string]*schema.Resource{
			"talos_client_configuration": dataSourceTalosClientConfiguration(),
		},
	}
}
