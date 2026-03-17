// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/tls"
	stdlibx509 "crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
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
	"github.com/siderolabs/talos/pkg/machinery/role"
	"golang.org/x/crypto/hkdf"
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

	if secretsBundle.Certs.K8sAggregator.Crt != nil {
		model.MachineSecrets.Certs.K8sAggregator.Cert = types.StringValue(bytesToBase64(secretsBundle.Certs.K8sAggregator.Crt))
		model.MachineSecrets.Certs.K8sAggregator.Key = types.StringValue(bytesToBase64(secretsBundle.Certs.K8sAggregator.Key))
	}

	// support for talos < 1.3
	if secretsBundle.Secrets.AESCBCEncryptionSecret != "" {
		model.MachineSecrets.Secrets.AESCBCEncryptionSecret = types.StringValue(secretsBundle.Secrets.AESCBCEncryptionSecret)
	}

	cc, err := generateClientConfigurationWithTTL(secretsBundle, constants.TalosAPIDefaultCertificateValidityDuration)
	if err != nil {
		return model, err
	}

	model.ClientConfiguration = cc

	return model, nil
}

// generateClientConfigurationWithTTL generates a clientConfiguration with a TTL-based cert.
// Used by the managed talos_machine_secrets resource which stores and renews certs in state.
func generateClientConfigurationWithTTL(secretsBundle *secrets.Bundle, ttl time.Duration) (clientConfiguration, error) {
	if secretsBundle.Clock == nil {
		secretsBundle.Clock = secrets.NewFixedClock(time.Now())
	}

	clientcert, err := secretsBundle.GenerateTalosAPIClientCertificateWithTTL(role.MakeSet(role.Admin), ttl)
	if err != nil {
		return clientConfiguration{}, err
	}

	return clientConfiguration{
		CA:   types.StringValue(bytesToBase64(secretsBundle.Certs.OS.Crt)),
		Cert: types.StringValue(bytesToBase64(clientcert.Crt)),
		Key:  types.StringValue(bytesToBase64(clientcert.Key)),
	}, nil
}

// generateClientConfiguration generates a clientConfiguration from a secrets bundle.
//
// The admin client certificate and key are derived deterministically using HKDF
// (RFC 5869) seeded from the OS CA private key and all cert-relevant inputs.
// Same inputs always produce byte-identical output — no crypto/rand is used.
//
// Supports Ed25519 and ECDSA P-256 OS CA keys (Talos uses Ed25519 since v1.x).
// Ed25519 signing is deterministic by design (RFC 8032); ECDSA uses RFC 6979.
func generateClientConfiguration(secretsBundle *secrets.Bundle, clusterName string, notBefore, notAfter time.Time) (clientConfiguration, error) {
	caCertBlock, _ := pem.Decode(secretsBundle.Certs.OS.Crt)
	if caCertBlock == nil {
		return clientConfiguration{}, fmt.Errorf("error decoding OS CA certificate PEM")
	}

	caCert, err := stdlibx509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return clientConfiguration{}, fmt.Errorf("error parsing OS CA certificate: %w", err)
	}

	caKeyBlock, _ := pem.Decode(secretsBundle.Certs.OS.Key)
	if caKeyBlock == nil {
		return clientConfiguration{}, fmt.Errorf("error decoding OS CA private key PEM")
	}

	info := fmt.Sprintf("talos-clientconfig:v1:%s:%d:%d", clusterName, notBefore.Unix(), notAfter.Unix())
	deterministicReader := hkdf.New(sha256.New, secretsBundle.Certs.OS.Key, []byte("talos-clientconfig-v1"), []byte(info))

	serialBytes := make([]byte, 16)
	if _, err = deterministicReader.Read(serialBytes); err != nil {
		return clientConfiguration{}, fmt.Errorf("error deriving serial number: %w", err)
	}

	template := &stdlibx509.Certificate{
		SerialNumber: new(big.Int).SetBytes(serialBytes),
		Subject: pkix.Name{
			Organization: []string{"os:admin"},
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    stdlibx509.KeyUsageDigitalSignature,
		ExtKeyUsage: []stdlibx509.ExtKeyUsage{stdlibx509.ExtKeyUsageClientAuth},
	}

	var (
		certDER      []byte
		clientKeyPEM []byte
	)

	caKeyParsed, parseErr := parseCAPrivateKey(caKeyBlock)
	if parseErr != nil {
		return clientConfiguration{}, parseErr
	}

	switch caKey := caKeyParsed.(type) {
	case ed25519.PrivateKey:
		adminKeyBytes := make([]byte, ed25519.SeedSize)
		if _, err = deterministicReader.Read(adminKeyBytes); err != nil {
			return clientConfiguration{}, fmt.Errorf("error deriving admin key bytes: %w", err)
		}

		adminKey := ed25519.NewKeyFromSeed(adminKeyBytes)

		// Ed25519 signing is deterministic per RFC 8032 — rand reader is unused.
		certDER, err = stdlibx509.CreateCertificate(nil, template, caCert, adminKey.Public(), caKey)
		if err != nil {
			return clientConfiguration{}, fmt.Errorf("error signing admin certificate: %w", err)
		}

		adminKeyDER, marshalErr := stdlibx509.MarshalPKCS8PrivateKey(adminKey)
		if marshalErr != nil {
			return clientConfiguration{}, fmt.Errorf("error marshaling admin private key: %w", marshalErr)
		}

		clientKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: adminKeyDER})

	case *ecdsa.PrivateKey:
		adminKeyBytes := make([]byte, 32)
		if _, err = deterministicReader.Read(adminKeyBytes); err != nil {
			return clientConfiguration{}, fmt.Errorf("error deriving admin key bytes: %w", err)
		}

		adminKey, ecParseErr := ecdsa.ParseRawPrivateKey(elliptic.P256(), adminKeyBytes)
		if ecParseErr != nil {
			return clientConfiguration{}, fmt.Errorf("error parsing derived admin key: %w", ecParseErr)
		}

		caSigner := &deterministicECDSASigner{key: caKey}

		certDER, err = stdlibx509.CreateCertificate(nil, template, caCert, &adminKey.PublicKey, caSigner)
		if err != nil {
			return clientConfiguration{}, fmt.Errorf("error signing admin certificate: %w", err)
		}

		adminKeyDER, marshalErr := stdlibx509.MarshalECPrivateKey(adminKey)
		if marshalErr != nil {
			return clientConfiguration{}, fmt.Errorf("error marshaling admin private key: %w", marshalErr)
		}

		clientKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: adminKeyDER})

	default:
		return clientConfiguration{}, fmt.Errorf("unsupported OS CA private key type: %T", caKeyParsed)
	}

	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	return clientConfiguration{
		CA:   types.StringValue(bytesToBase64(secretsBundle.Certs.OS.Crt)),
		Cert: types.StringValue(bytesToBase64(clientCertPEM)),
		Key:  types.StringValue(bytesToBase64(clientKeyPEM)),
	}, nil
}

// parseCAPrivateKey parses a PEM block into either an ed25519.PrivateKey or *ecdsa.PrivateKey.
func parseCAPrivateKey(block *pem.Block) (any, error) {
	switch block.Type {
	case "EC PRIVATE KEY":
		key, err := stdlibx509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("error parsing OS CA private key: %w", err)
		}

		return key, nil

	case "ED25519 PRIVATE KEY", "PRIVATE KEY":
		parsed, err := stdlibx509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("error parsing OS CA private key: %w", err)
		}

		switch k := parsed.(type) {
		case ed25519.PrivateKey:
			return k, nil
		case *ecdsa.PrivateKey:
			return k, nil
		default:
			return nil, fmt.Errorf("unsupported OS CA private key type in PKCS#8: %T", parsed)
		}

	default:
		return nil, fmt.Errorf("unsupported OS CA private key PEM type: %s", block.Type)
	}
}

type clientConfigTimestampError struct {
	summary string
	detail  string
}

func (e *clientConfigTimestampError) Error() string { return e.detail }

// resolveClientConfigTimestamps resolves notBefore/notAfter for the admin client certificate.
// When notBeforeStr is non-empty it parses the RFC3339 timestamp and adds crtTTLStr (default 87600h).
// When notBeforeStr is empty it reads the timestamps from the OS CA PEM.
func resolveClientConfigTimestamps(notBeforeStr, crtTTLStr string, osCACert []byte) (notBefore, notAfter time.Time, tsErr *clientConfigTimestampError) {
	if notBeforeStr != "" {
		var err error

		notBefore, err = time.Parse(time.RFC3339, notBeforeStr)
		if err != nil {
			return time.Time{}, time.Time{}, &clientConfigTimestampError{
				summary: "invalid not_before",
				detail:  fmt.Sprintf("unable to parse not_before %q as RFC3339: %s", notBeforeStr, err.Error()),
			}
		}

		crtTTL := 87600 * time.Hour

		if crtTTLStr != "" {
			crtTTL, err = time.ParseDuration(crtTTLStr)
			if err != nil {
				return time.Time{}, time.Time{}, &clientConfigTimestampError{
					summary: "invalid crt_ttl",
					detail:  fmt.Sprintf("unable to parse crt_ttl %q: %s", crtTTLStr, err.Error()),
				}
			}
		}

		return notBefore, notBefore.Add(crtTTL), nil
	}

	block, _ := pem.Decode(osCACert)
	if block == nil {
		return time.Time{}, time.Time{}, &clientConfigTimestampError{
			summary: "failed to parse Talos OS CA certificate",
			detail:  "PEM block is nil",
		}
	}

	caCert, err := stdlibx509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}, time.Time{}, &clientConfigTimestampError{
			summary: "failed to parse Talos OS CA certificate",
			detail:  err.Error(),
		}
	}

	return caCert.NotBefore, caCert.NotAfter, nil
}

func machineSecretsToSecretsBundle(model talosMachineSecretsResourceModelV1) (*secrets.Bundle, error) {
	secretsBundle := &secrets.Bundle{
		Cluster: &secrets.Cluster{
			ID:     model.MachineSecrets.Cluster.ID.ValueString(),
			Secret: model.MachineSecrets.Cluster.Secret.ValueString(),
		},
		Secrets: &secrets.Secrets{
			BootstrapToken:            model.MachineSecrets.Secrets.BootstrapToken.ValueString(),
			SecretboxEncryptionSecret: model.MachineSecrets.Secrets.SecretboxEncryptionSecret.ValueString(),
		},
		TrustdInfo: &secrets.TrustdInfo{
			Token: model.MachineSecrets.TrustdInfo.Token.ValueString(),
		},
	}

	if model.MachineSecrets.Secrets.AESCBCEncryptionSecret.ValueString() != "" {
		secretsBundle.Secrets.AESCBCEncryptionSecret = model.MachineSecrets.Secrets.AESCBCEncryptionSecret.ValueString()
	}

	etcdCert, err := base64ToBytes(model.MachineSecrets.Certs.Etcd.Cert.ValueString())
	if err != nil {
		return nil, err
	}

	etcdKey, err := base64ToBytes(model.MachineSecrets.Certs.Etcd.Key.ValueString())
	if err != nil {
		return nil, err
	}

	k8sCert, err := base64ToBytes(model.MachineSecrets.Certs.K8s.Cert.ValueString())
	if err != nil {
		return nil, err
	}

	k8sKey, err := base64ToBytes(model.MachineSecrets.Certs.K8s.Key.ValueString())
	if err != nil {
		return nil, err
	}

	k8sAggregatorCert, err := base64ToBytes(model.MachineSecrets.Certs.K8sAggregator.Cert.ValueString())
	if err != nil {
		return nil, err
	}

	k8sAggregatorKey, err := base64ToBytes(model.MachineSecrets.Certs.K8sAggregator.Key.ValueString())
	if err != nil {
		return nil, err
	}

	k8sServiceAccountKey, err := base64ToBytes(model.MachineSecrets.Certs.K8sServiceAccount.Key.ValueString())
	if err != nil {
		return nil, err
	}

	osCert, err := base64ToBytes(model.MachineSecrets.Certs.OS.Cert.ValueString())
	if err != nil {
		return nil, err
	}

	osKey, err := base64ToBytes(model.MachineSecrets.Certs.OS.Key.ValueString())
	if err != nil {
		return nil, err
	}

	secretsBundle.Certs = &secrets.Certs{
		Etcd: &x509.PEMEncodedCertificateAndKey{
			Crt: etcdCert,
			Key: etcdKey,
		},
		K8s: &x509.PEMEncodedCertificateAndKey{
			Crt: k8sCert,
			Key: k8sKey,
		},
		K8sAggregator: &x509.PEMEncodedCertificateAndKey{
			Crt: k8sAggregatorCert,
			Key: k8sAggregatorKey,
		},
		K8sServiceAccount: &x509.PEMEncodedKey{
			Key: k8sServiceAccountKey,
		},
		OS: &x509.PEMEncodedCertificateAndKey{
			Crt: osCert,
			Key: osKey,
		},
	}

	return secretsBundle, nil
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

type goDurationValidator struct{}

func goDurationValid() goDurationValidator {
	return goDurationValidator{}
}

func (v goDurationValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if _, err := time.ParseDuration(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"invalid crt_ttl",
			fmt.Sprintf("unable to parse duration %q: %s", req.ConfigValue.ValueString(), err.Error()),
		)
	}
}

func (v goDurationValidator) Description(_ context.Context) string {
	return "Validates that the value is a valid Go duration string (e.g. \"8760h\", \"87600h\")"
}

func (v goDurationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

type rfc3339Validator struct{}

func rfc3339Valid() rfc3339Validator {
	return rfc3339Validator{}
}

func (v rfc3339Validator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if _, err := time.Parse(time.RFC3339, req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"invalid not_before",
			fmt.Sprintf("unable to parse not_before %q as RFC3339: %s", req.ConfigValue.ValueString(), err.Error()),
		)
	}
}

func (v rfc3339Validator) Description(_ context.Context) string {
	return "must be a valid RFC3339 timestamp (e.g. \"2024-01-01T00:00:00Z\")"
}

func (v rfc3339Validator) MarkdownDescription(ctx context.Context) string {
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
