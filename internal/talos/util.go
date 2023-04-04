// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/siderolabs/crypto/x509"
	sideronet "github.com/siderolabs/net"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"github.com/siderolabs/talos/pkg/machinery/config/encoder"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/bundle"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"google.golang.org/grpc/status"
)

type machineConfigGenerateOptions struct {
	machineType       machine.Type
	clusterName       string
	clusterEndpoint   string
	machineSecrets    *generate.SecretsBundle
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
		generate.WithInstallImage(GenerateInstallerImage()),
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
			return fmt.Errorf("error parsing config patch: %w", err)
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

	input, err := generate.NewInput(
		options.InputOptions.ClusterName,
		options.InputOptions.Endpoint,
		options.InputOptions.KubeVersion,
		m.machineSecrets,
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

func GenerateInstallerImage() string {
	return fmt.Sprintf("%s/%s/installer:%s", gendata.ImagesRegistry, gendata.ImagesUsername, gendata.VersionTag)
}

func secretsBundleTomachineSecrets(secretsBundle *generate.SecretsBundle) (talosMachineSecretsResourceModelV1, error) {
	if secretsBundle.Clock == nil {
		secretsBundle.Clock = generate.NewClock()
	}

	model := talosMachineSecretsResourceModelV1{
		Id: types.StringValue("machine_secrets"),
		MachineSecrets: machineSecrets{
			Cluster: machineSecretsCluster{
				ID:     types.StringValue(secretsBundle.Cluster.ID),
				Secret: types.StringValue(secretsBundle.Cluster.Secret),
			},
			Secrets: machineSecretsSecrets{
				BootstrapToken:            types.StringValue(secretsBundle.Secrets.BootstrapToken),
				SecretboxEncryptionSecret: types.StringValue(secretsBundle.Secrets.SecretboxEncryptionSecret),
			},
			TrustdInfo: machineSecretsTrustdInfo{
				Token: types.StringValue(secretsBundle.TrustdInfo.Token),
			},
			Certs: machineSecretsCerts{
				Etcd: machineSecretsCertKeyPair{
					Cert: types.StringValue(bytesToBase64(secretsBundle.Certs.Etcd.Crt)),
					Key:  types.StringValue(bytesToBase64(secretsBundle.Certs.Etcd.Key)),
				},
				K8s: machineSecretsCertKeyPair{
					Cert: types.StringValue(bytesToBase64(secretsBundle.Certs.K8s.Crt)),
					Key:  types.StringValue(bytesToBase64(secretsBundle.Certs.K8s.Key)),
				},
				K8sAggregator: machineSecretsCertKeyPair{
					Cert: types.StringValue(bytesToBase64(secretsBundle.Certs.K8sAggregator.Crt)),
					Key:  types.StringValue(bytesToBase64(secretsBundle.Certs.K8sAggregator.Key)),
				},
				K8sServiceAccount: machineSecretsCertsK8sServiceAccount{
					Key: types.StringValue(bytesToBase64(secretsBundle.Certs.K8sServiceAccount.Key)),
				},
				OS: machineSecretsCertKeyPair{
					Cert: types.StringValue(bytesToBase64(secretsBundle.Certs.OS.Crt)),
					Key:  types.StringValue(bytesToBase64(secretsBundle.Certs.OS.Key)),
				},
			},
		},
	}

	// support for talos < 1.3
	if secretsBundle.Secrets.AESCBCEncryptionSecret != "" {
		model.MachineSecrets.Secrets.AESCBCEncryptionSecret = types.StringValue(secretsBundle.Secrets.AESCBCEncryptionSecret)
	}

	generateInput, err := generate.NewInput("", "", "", secretsBundle)
	if err != nil {
		return model, err
	}

	talosConfig, err := generate.Talosconfig(generateInput)
	if err != nil {
		return model, err
	}

	model.ClientConfiguration = clientConfiguration{
		CA:   types.StringValue(talosConfig.Contexts[talosConfig.Context].CA),
		Cert: types.StringValue(talosConfig.Contexts[talosConfig.Context].Crt),
		Key:  types.StringValue(talosConfig.Contexts[talosConfig.Context].Key),
	}

	return model, nil
}

func validateVersionContract(version string) (*config.VersionContract, error) {
	versionContract, err := config.ParseContractFromVersion(version)
	if err != nil {
		return nil, err
	}

	return versionContract, nil
}

func talosClientOp(ctx context.Context, endpoint, node string, tc *clientconfig.Config, opFunc func(ctx context.Context, c *client.Client) error) error {
	opCtx := client.WithNode(ctx, node)

	clientOpts := []client.OptionFunc{
		client.WithConfig(tc),
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
		s, ok := status.FromError(err)
		if !ok {
			return err
		}

		if strings.Contains(s.Message(), "name resilver error") || strings.Contains(s.Message(), "i/o timeout") {
			return err
		}

		if s.Message() == "connection closed before server preface received" || s.Message() == "connection error: desc = \"error reading server preface: remote error: tls: bad certificate\"" {
			c, err = client.New(ctx, clientOpts...)
			if err != nil {
				return err
			}
		}
	}
	defer c.Close() //nolint:errcheck

	if err := opFunc(opCtx, c); err != nil {
		return err
	}

	return nil
}

type talosVersionValidator struct{}

func talosVersionValid() talosVersionValidator {
	return talosVersionValidator{}
}

func (v talosVersionValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	version := req.ConfigValue.ValueString()

	_, err := validateVersionContract(version)
	if err != nil {
		resp.Diagnostics.AddError("Invalid version", err.Error())
	}

}

func (v talosVersionValidator) Description(ctx context.Context) string {
	return "Validates that the talos version is valid"
}

func (v talosVersionValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func validateClusterEndpoint(endpoint string) error {
	// Validate url input to ensure it has https:// scheme before we attempt to gen
	u, err := url.Parse(endpoint)
	if err != nil {
		if !strings.Contains(endpoint, "/") {
			// not a URL, could be just host:port
			u = &url.URL{
				Host: endpoint,
			}
		} else {
			return fmt.Errorf("failed to parse the cluster endpoint URL: %w", err)
		}
	}

	if u.Scheme == "" {
		if u.Port() == "" {
			return fmt.Errorf("no scheme and port specified for the cluster endpoint URL\ntry: %q", fixControlPlaneEndpoint(u))
		}

		return fmt.Errorf("no scheme specified for the cluster endpoint URL\ntry: %q", fixControlPlaneEndpoint(u))
	}

	if u.Scheme != "https" {
		return fmt.Errorf("the control plane endpoint URL should have scheme https://\ntry: %q", fixControlPlaneEndpoint(u))
	}

	if err = sideronet.ValidateEndpointURI(endpoint); err != nil {
		return fmt.Errorf("error validating the cluster endpoint URL: %w", err)
	}

	return nil
}

func fixControlPlaneEndpoint(u *url.URL) *url.URL {
	// handle the case when the hostname/IP is given without the port, it parses as URL Path
	if u.Scheme == "" && u.Host == "" && u.Path != "" {
		u.Host = u.Path
		u.Path = ""
	}

	u.Scheme = "https"

	if u.Port() == "" {
		u.Host = fmt.Sprintf("%s:%d", u.Host, constants.DefaultControlPlanePort)
	}

	return u
}

func bytesToBase64(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}

func base64ToBytes(in string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(in)
}

func talosClientTFConfigToTalosClientConfig(clusterName, ca, cert, key string) (*clientconfig.Config, error) {
	caCert, err := base64ToBytes(ca)
	if err != nil {
		return nil, err
	}

	clientCert, err := base64ToBytes(cert)
	if err != nil {
		return nil, err
	}

	clientKey, err := base64ToBytes(key)
	if err != nil {
		return nil, err
	}

	talosConfig := clientconfig.NewConfig(
		clusterName,
		nil,
		caCert,
		&x509.PEMEncodedCertificateAndKey{
			Crt: clientCert,
			Key: clientKey,
		},
	)

	return talosConfig, nil
}
