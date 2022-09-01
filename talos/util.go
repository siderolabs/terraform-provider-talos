// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"errors"
	"fmt"
	"strings"

	"github.com/talos-systems/talos/pkg/machinery/config"
	"github.com/talos-systems/talos/pkg/machinery/config/configloader"
	"github.com/talos-systems/talos/pkg/machinery/config/configpatcher"
	"github.com/talos-systems/talos/pkg/machinery/config/decoder"
	"github.com/talos-systems/talos/pkg/machinery/config/encoder"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1/bundle"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1/generate"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1/machine"
	"github.com/talos-systems/talos/pkg/machinery/constants"
	"github.com/talos-systems/talos/pkg/machinery/gendata"
	"gopkg.in/yaml.v3"
)

type machineConfigGenerateOptions struct {
	machineType       machine.Type
	clusterName       string
	clusterEndpoint   string
	machineSecrets    string
	kubernetesVersion string
	talosVersion      string
	docsEnabled       bool
	examplesEnabled   bool
	configPatch       string
	configPatches     []string
}

func (m *machineConfigGenerateOptions) generate() (string, error) {
	genOptions := make([]generate.GenOption, 0)

	// default gen options
	genOptions = append(genOptions,
		generate.WithClusterDiscovery(true),
		generate.WithDNSDomain(constants.DefaultDNSDomain),
		generate.WithInstallDisk("/dev/sda"),
		generate.WithInstallImage(generateInstallerImage()),
		generate.WithPersist(true),
	)

	if m.talosVersion != "" {
		versionContract, err := validateVersionContract(m.talosVersion)
		if err != nil {
			return "", err
		}

		genOptions = append(genOptions, generate.WithVersionContract(versionContract))
	}

	commentsFlags := encoder.CommentsDisabled

	if m.docsEnabled {
		commentsFlags |= encoder.CommentsDocs
	}

	if m.examplesEnabled {
		commentsFlags |= encoder.CommentsExamples
	}

	configBundleOpts := []bundle.Option{
		bundle.WithInputOptions(
			&bundle.InputOptions{
				ClusterName: m.clusterName,
				Endpoint:    m.clusterEndpoint,
				KubeVersion: strings.TrimPrefix(m.kubernetesVersion, "v"),
				GenOptions:  genOptions,
			},
		),
	}

	addConfigPatch := func(configPatch string, configOpt func([]configpatcher.Patch) bundle.Option) error {
		patch, err := genPatches([]byte(configPatch))
		if err != nil {
			return err
		}

		configBundleOpts = append(configBundleOpts, configOpt(patch))

		return nil
	}

	if m.configPatch != "" {
		switch m.machineType {
		case machine.TypeControlPlane:
			if err := addConfigPatch(m.configPatch, bundle.WithPatchControlPlane); err != nil {
				return "", err
			}
		case machine.TypeWorker:
			if err := addConfigPatch(m.configPatch, bundle.WithPatchWorker); err != nil {
				return "", err
			}
		}
	}

	for _, p := range m.configPatches {
		switch m.machineType {
		case machine.TypeControlPlane:
			if err := addConfigPatch(p, bundle.WithPatchControlPlane); err != nil {
				return "", err
			}
		case machine.TypeWorker:
			if err := addConfigPatch(p, bundle.WithPatchWorker); err != nil {
				return "", err
			}
		}
	}

	options := bundle.Options{}

	for _, opt := range configBundleOpts {
		if err := opt(&options); err != nil {
			return "", err
		}
	}

	if options.InputOptions == nil {
		return "", fmt.Errorf(("generated input options are nil"))
	}

	var secretsBundle *generate.SecretsBundle

	err := yaml.Unmarshal([]byte(m.machineSecrets), &secretsBundle)
	if err != nil {
		return "", err
	}

	secretsBundle.Clock = generate.NewClock()

	input, err := generate.NewInput(
		options.InputOptions.ClusterName,
		options.InputOptions.Endpoint,
		options.InputOptions.KubeVersion,
		secretsBundle,
		options.InputOptions.GenOptions...,
	)
	if err != nil {
		return "", err
	}

	bundle := &bundle.ConfigBundle{
		InitCfg: &v1alpha1.Config{},
	}

	var (
		generatedConfig *v1alpha1.Config
		machineConfig   string
	)

	switch m.machineType {
	case machine.TypeControlPlane:
		generatedConfig, err = generate.Config(machine.TypeControlPlane, input)
		if err != nil {
			return "", err
		}

		bundle.ControlPlaneCfg = generatedConfig

		if err := bundle.ApplyPatches(options.PatchesControlPlane, true, false); err != nil {
			return "", err
		}

		machineConfig, err = bundle.ControlPlaneCfg.EncodeString(encoder.WithComments(commentsFlags))
		if err != nil {
			return "", err
		}
	case machine.TypeWorker:
		generatedConfig, err = generate.Config(machine.TypeWorker, input)
		if err != nil {
			return "", err
		}

		bundle.WorkerCfg = generatedConfig

		if err := bundle.ApplyPatches(options.PatchesWorker, false, true); err != nil {
			return "", err
		}

		machineConfig, err = bundle.WorkerCfg.EncodeString(encoder.WithComments(commentsFlags))
		if err != nil {
			return "", err
		}
	}

	return machineConfig, nil
}

func generateInstallerImage() string {
	return fmt.Sprintf("%s/%s/installer:%s", gendata.ImagesRegistry, gendata.ImagesUsername, gendata.VersionTag)
}

func genPatches(in []byte) ([]configpatcher.Patch, error) {
	cfg, err := configloader.NewFromBytes(in)
	if err != nil {
		return nil, err
	}

	return []configpatcher.Patch{configpatcher.StrategicMergePatch{Provider: cfg}}, nil
}

func validatePatch(patch string) error {
	dec := decoder.NewDecoder([]byte(patch))

	_, err := dec.Decode()

	return err
}

func validateVersionContract(version string) (*config.VersionContract, error) {
	versionContract, err := config.ParseContractFromVersion(version)
	if err != nil {
		return nil, err
	}

	if !versionContract.Greater(config.TalosVersion1_1) {
		return nil, errors.New("config generation only supported for Talos >= v1.2")
	}

	return versionContract, nil
}
