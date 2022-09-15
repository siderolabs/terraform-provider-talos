// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/talos-systems/talos/pkg/machinery/client"
	clientconfig "github.com/talos-systems/talos/pkg/machinery/client/config"
	"github.com/talos-systems/talos/pkg/machinery/config"
	"github.com/talos-systems/talos/pkg/machinery/config/configpatcher"
	"github.com/talos-systems/talos/pkg/machinery/config/encoder"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1/bundle"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1/generate"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1/machine"
	"github.com/talos-systems/talos/pkg/machinery/constants"
	"github.com/talos-systems/talos/pkg/machinery/gendata"
	"google.golang.org/grpc/status"
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

	addConfigPatch := func(configPatches []string, configOpt func([]configpatcher.Patch) bundle.Option) error {
		var patches []configpatcher.Patch

		patches, err := configpatcher.LoadPatches(configPatches)
		if err != nil {
			return fmt.Errorf("error parsing config JSON patch: %w", err)
		}

		configBundleOpts = append(configBundleOpts, configOpt(patches))

		return nil
	}

	switch m.machineType {
	case machine.TypeControlPlane:

		if err := addConfigPatch(m.configPatches, bundle.WithPatchControlPlane); err != nil {
			return "", err
		}
	case machine.TypeWorker:
		if err := addConfigPatch(m.configPatches, bundle.WithPatchWorker); err != nil {
			return "", err
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

func validateVersionContract(version string) (*config.VersionContract, error) {
	versionContract, err := config.ParseContractFromVersion(version)
	if err != nil {
		return nil, err
	}

	return versionContract, nil
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

func talosClientOp(ctx context.Context, endpoint, node, tc string, opFunc func(ctx context.Context, c *client.Client) error) error {
	cfg, err := clientconfig.FromString(tc)
	if err != nil {
		return err
	}

	opCtx := client.WithNode(ctx, node)

	clientOpts := []client.OptionFunc{
		client.WithConfig(cfg),
		client.WithEndpoints([]string{endpoint}...),
	}

	c, err := client.New(ctx, append(clientOpts, client.WithTLSConfig(&tls.Config{
		InsecureSkipVerify: true,
	}))...)
	if err != nil {
		return err
	}

	_, err = c.Disks(ctx)
	if err != nil {
		c.Close()
		if s, ok := status.FromError(err); !ok || s.Message() != "connection closed before server preface received" {
			return err
		}

		c, err = client.New(ctx, clientOpts...)
		if err != nil {
			return err
		}
	}
	defer c.Close() //nolint:errcheck

	if err := opFunc(opCtx, c); err != nil {
		return err
	}

	return nil
}
