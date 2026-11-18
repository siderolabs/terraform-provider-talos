// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
)

var (
	TalosMachineCheckBootReady         = talosMachineCheckBootReady
	TalosMachineWaitForBoot            = talosMachineWaitForBoot
	TalosMachineRebootBeforeBootstrap  = talosMachineRebootBeforeBootstrap
	ValidateMachineConfigurationConfig = validateMachineConfigurationConfig
)

// MachineConfigurationComponents is the model of the talos_machine_configuration `components` attribute.
type MachineConfigurationComponents = machineConfigurationComponents

// GenerateMachineConfiguration generates a machine configuration the way talos_machine_configuration does.
func GenerateMachineConfiguration(
	secretsBundle *secrets.Bundle,
	machineType machine.Type,
	talosVersion string,
	disableKubernetes, disableEtcd bool,
	configPatches ...string,
) (string, error) {
	opts := &machineConfigGenerateOptions{
		machineSecrets:    secretsBundle,
		clusterName:       "test-cluster",
		clusterEndpoint:   "https://127.0.0.1:6443",
		kubernetesVersion: "1.35.0",
		talosVersion:      talosVersion,
		configPatches:     configPatches,
		machineType:       machineType,
		disableKubernetes: disableKubernetes,
		disableEtcd:       disableEtcd,
	}

	return opts.generate()
}
