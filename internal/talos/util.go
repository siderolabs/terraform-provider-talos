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
	"time"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/siderolabs/crypto/x509"
	sideronet "github.com/siderolabs/net"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/bundle"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"github.com/siderolabs/talos/pkg/machinery/config/encoder"
	"github.com/siderolabs/talos/pkg/machinery/config/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
)

type machineConfigGenerateOptions struct { //nolint:govet
	machineType       machine.Type
	clusterName       string
	clusterEndpoint   string
	machineSecrets    *secrets.Bundle
	kubernetesVersion string
	talosVersion      string
	docsEnabled       bool
	examplesEnabled   bool
	configPatches     []string
}

func (m *machineConfigGenerateOptions) generate() (string, error) {
	genOptions := make([]generate.Option, 0)

	// default gen options
	genOptions = append(genOptions,
		generate.WithClusterDiscovery(true),
		generate.WithDNSDomain(constants.DefaultDNSDomain),
		generate.WithInstallDisk("/dev/sda"),
		generate.WithInstallImage(GenerateInstallerImage()),
		generate.WithPersist(true),
		generate.WithSecretsBundle(m.machineSecrets),
	)

	versionContract, err := validateVersionContract(m.talosVersion)
	if err != nil {
		return "", err
	}

	genOptions = append(genOptions, generate.WithVersionContract(versionContract))

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
		bundle.WithVerbose(false),
	}

	addConfigPatch := func(configPatches []string, configOpt func([]configpatcher.Patch) bundle.Option) error {
		var patches []configpatcher.Patch

		patches, err = configpatcher.LoadPatches(configPatches)
		if err != nil {
			return fmt.Errorf("error parsing config patch: %w", err)
		}

		configBundleOpts = append(configBundleOpts, configOpt(patches))

		return nil
	}

	switch m.machineType { //nolint:exhaustive
	case machine.TypeControlPlane:
		if err = addConfigPatch(m.configPatches, bundle.WithPatchControlPlane); err != nil {
			return "", err
		}
	case machine.TypeWorker:
		if err = addConfigPatch(m.configPatches, bundle.WithPatchWorker); err != nil {
			return "", err
		}
	}

	configBundle, err := bundle.NewBundle(configBundleOpts...)
	if err != nil {
		return "", err
	}

	machineConfigBytes, err := configBundle.Serialize(commentsFlags, m.machineType)
	if err != nil {
		return "", err
	}

	return string(machineConfigBytes), nil
}

// GenerateInstallerImage generates the installer image name.
func GenerateInstallerImage() string {
	return fmt.Sprintf("%s/%s/installer:%s", gendata.ImagesRegistry, gendata.ImagesUsername, gendata.VersionTag)
}

func secretsBundleTomachineSecrets(secretsBundle *secrets.Bundle) (talosMachineSecretsResourceModelV1, error) {
	if secretsBundle.Clock == nil {
		secretsBundle.Clock = secrets.NewFixedClock(time.Now())
	}

	model := talosMachineSecretsResourceModelV1{
		ID: types.StringValue("machine_secrets"),
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

	generateInput, err := generate.NewInput("", "", "", generate.WithSecretsBundle(secretsBundle))
	if err != nil {
		return model, err
	}

	talosConfig, err := generateInput.Talosconfig()
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
	nodeCtx := client.WithNode(ctx, node)

	c, err := client.New(ctx, client.WithTLSConfig(&tls.Config{
		InsecureSkipVerify: true,
	}), client.WithEndpoints(endpoint))
	if err != nil {
		return err
	}

	_, err = c.Disks(nodeCtx)
	if err != nil {
		c.Close() //nolint:errcheck

		c, err = client.New(ctx, client.WithConfig(tc), client.WithEndpoints(endpoint))
		if err != nil {
			return err
		}
	}
	defer c.Close() //nolint:errcheck

	return opFunc(nodeCtx, c)
}

type talosVersionValidator struct{}

func talosVersionValid() talosVersionValidator {
	return talosVersionValidator{}
}

func (v talosVersionValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	version := req.ConfigValue.ValueString()

	_, err := validateVersionContract(version)
	if err != nil {
		resp.Diagnostics.AddError("Invalid version", err.Error())
	}
}

func (v talosVersionValidator) Description(_ context.Context) string {
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
