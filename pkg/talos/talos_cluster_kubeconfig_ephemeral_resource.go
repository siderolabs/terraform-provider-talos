// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	stdlibx509 "crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"golang.org/x/crypto/hkdf"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var _ ephemeral.EphemeralResource = &talosClusterKubeConfigEphemeralResource{}

type talosClusterKubeConfigEphemeralResource struct{}

type talosClusterKubeConfigEphemeralResourceModel struct {
	MachineSecrets                machineSecrets                `tfsdk:"machine_secrets"`
	ClusterName                   types.String                  `tfsdk:"cluster_name"`
	Endpoint                      types.String                  `tfsdk:"endpoint"`
	NotBefore                     types.String                  `tfsdk:"not_before"`
	CrtTTL                        types.String                  `tfsdk:"crt_ttl"`
	KubeConfigRaw                 types.String                  `tfsdk:"kubeconfig_raw"`
	KubernetesClientConfiguration kubernetesClientConfiguration `tfsdk:"kubernetes_client_configuration"`
}

// NewTalosClusterKubeConfigEphemeralResource implements the ephemeral.EphemeralResource interface.
func NewTalosClusterKubeConfigEphemeralResource() ephemeral.EphemeralResource {
	return &talosClusterKubeConfigEphemeralResource{}
}

func (r *talosClusterKubeConfigEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_kubeconfig"
}

func (r *talosClusterKubeConfigEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generate a kubeconfig for a Talos cluster from machine secrets. " +
			"This is an ephemeral resource that does not persist secrets in Terraform state. " +
			"The admin client certificate is generated with pinned timestamps so kubeconfig_raw " +
			"is byte-identical on every open as long as machine_secrets and not_before are unchanged.",
		Attributes: map[string]schema.Attribute{
			"machine_secrets": machineSecretsSchemaAttribute(),
			"cluster_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the cluster; embedded in the kubeconfig context and cluster names",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"endpoint": schema.StringAttribute{
				Required:    true,
				Description: "The Kubernetes API server URL to embed in the kubeconfig (e.g. https://1.2.3.4:6443)",
			},
			"not_before": schema.StringAttribute{
				Optional: true,
				Description: "RFC3339 timestamp to use as the NotBefore field of the generated admin client certificate. " +
					"When set, the certificate validity starts at this time and ends at not_before + crt_ttl. " +
					"Persist this value in a terraform_data resource so it is stable across plans and the " +
					"generated kubeconfig_raw is byte-identical on every open. " +
					"When omitted, the certificate uses the K8s CA's own NotBefore/NotAfter timestamps.",
				Validators: []validator.String{
					rfc3339Valid(),
				},
			},
			"crt_ttl": schema.StringAttribute{
				Optional: true,
				Description: "The lifetime of the generated admin client certificate as a Go duration string " +
					"(e.g. \"8760h\" for 1 year, \"87600h\" for 10 years). Defaults to \"87600h\" (10 years). " +
					"Only used when not_before is set; when not_before is omitted the cert uses the K8s CA's NotAfter directly.",
				Validators: []validator.String{
					goDurationValid(),
				},
			},
			"kubeconfig_raw": schema.StringAttribute{
				Computed:    true,
				Description: "The raw kubeconfig",
				Sensitive:   true,
			},
			"kubernetes_client_configuration": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"host": schema.StringAttribute{
						Computed:    true,
						Description: "The kubernetes host",
					},
					"ca_certificate": schema.StringAttribute{
						Computed:    true,
						Description: "The kubernetes CA certificate",
					},
					"client_certificate": schema.StringAttribute{
						Computed:    true,
						Description: "The kubernetes client certificate",
					},
					"client_key": schema.StringAttribute{
						Computed:    true,
						Sensitive:   true,
						Description: "The kubernetes client key",
					},
				},
				Computed:    true,
				Description: "The kubernetes client configuration",
			},
		},
	}
}

func (r *talosClusterKubeConfigEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var config talosClusterKubeConfigEphemeralResourceModel

	diags = obj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	secretsBundle, err := machineSecretsToSecretsBundle(talosMachineSecretsResourceModelV1{
		MachineSecrets: config.MachineSecrets,
	})
	if err != nil {
		resp.Diagnostics.AddError("failed to convert machine secrets to secrets bundle", err.Error())

		return
	}

	notBefore, notAfter, tsErr := resolveClientConfigTimestamps(config.NotBefore.ValueString(), config.CrtTTL.ValueString(), secretsBundle.Certs.K8s.Crt)
	if tsErr != nil {
		resp.Diagnostics.AddError(tsErr.summary, tsErr.detail)

		return
	}

	kc, err := GenerateKubeconfig(secretsBundle, config.ClusterName.ValueString(), config.Endpoint.ValueString(), notBefore, notAfter)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate kubeconfig", err.Error())

		return
	}

	result := talosClusterKubeConfigEphemeralResourceModel{
		MachineSecrets: config.MachineSecrets,
		ClusterName:    config.ClusterName,
		Endpoint:       config.Endpoint,
		NotBefore:      config.NotBefore,
		CrtTTL:         config.CrtTTL,
		KubeConfigRaw:  basetypes.NewStringValue(kc.Raw),
		KubernetesClientConfiguration: kubernetesClientConfiguration{
			Host:              basetypes.NewStringValue(config.Endpoint.ValueString()),
			CACertificate:     basetypes.NewStringValue(bytesToBase64(secretsBundle.Certs.K8s.Crt)),
			ClientCertificate: basetypes.NewStringValue(bytesToBase64(kc.ClientCertPEM)),
			ClientKey:         basetypes.NewStringValue(bytesToBase64(kc.ClientKeyPEM)),
		},
	}

	diags = resp.Result.Set(ctx, &result)
	resp.Diagnostics.Append(diags...)
}

// KubeconfigResult holds the generated kubeconfig and its components.
type KubeconfigResult struct {
	Raw           string // Full kubeconfig YAML
	ClientCertPEM []byte // PEM-encoded admin client certificate
	ClientKeyPEM  []byte // PEM-encoded admin client private key
}

// GenerateKubeconfig generates a kubeconfig from the provided secrets bundle.
//
// The admin client certificate and key are derived deterministically using HKDF
// (RFC 5869) seeded from the K8s CA private key and all cert-relevant inputs.
// Same inputs always produce byte-identical output — no crypto/rand is used.
func GenerateKubeconfig(bundle *secrets.Bundle, clusterName, endpoint string, notBefore, notAfter time.Time) (*KubeconfigResult, error) {
	// Parse CA certificate and private key from PEM.
	caCertBlock, _ := pem.Decode(bundle.Certs.K8s.Crt)
	if caCertBlock == nil {
		return nil, fmt.Errorf("error decoding K8s CA certificate PEM")
	}

	caCert, err := stdlibx509.ParseCertificate(caCertBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("error parsing K8s CA certificate: %w", err)
	}

	caKeyBlock, _ := pem.Decode(bundle.Certs.K8s.Key)
	if caKeyBlock == nil {
		return nil, fmt.Errorf("error decoding K8s CA private key PEM")
	}

	caKey, err := stdlibx509.ParseECPrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("error parsing K8s CA private key: %w", err)
	}

	// Build a deterministic byte stream via HKDF (RFC 5869). The secret is the
	// CA private key (stable in machine_secrets); the info string binds all
	// inputs that affect the cert so different parameters produce different keys.
	info := fmt.Sprintf("talos-kubeconfig:v1:%s:%s:%d:%d", clusterName, endpoint, notBefore.Unix(), notAfter.Unix())
	deterministicReader := hkdf.New(sha256.New, bundle.Certs.K8s.Key, []byte("talos-kubeconfig-v1"), []byte(info))

	// Derive admin client ECDSA private key deterministically from HKDF output.
	// ecdsa.ParseRawPrivateKey constructs a key from raw bytes without using
	// crypto/rand (unlike ecdsa.GenerateKey which ignores the reader in Go 1.26+).
	adminKeyBytes := make([]byte, 32) // P-256 key size
	if _, err = deterministicReader.Read(adminKeyBytes); err != nil {
		return nil, fmt.Errorf("error deriving admin key bytes: %w", err)
	}

	adminKey, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), adminKeyBytes)
	if err != nil {
		// The HKDF output might produce an invalid scalar (0 or >= n).
		// In practice this is astronomically unlikely for P-256 but handle it.
		return nil, fmt.Errorf("error parsing derived admin key: %w", err)
	}

	// Deterministic serial number from the same HKDF stream.
	serialBytes := make([]byte, 16)
	if _, err = deterministicReader.Read(serialBytes); err != nil {
		return nil, fmt.Errorf("error deriving serial number: %w", err)
	}

	serialNumber := new(big.Int).SetBytes(serialBytes)

	// Build and sign the admin client certificate.
	template := &stdlibx509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "admin",
			Organization: []string{"system:masters"},
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		KeyUsage:  stdlibx509.KeyUsageDigitalSignature | stdlibx509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []stdlibx509.ExtKeyUsage{
			stdlibx509.ExtKeyUsageClientAuth,
		},
	}

	// Use RFC 6979 deterministic signer for the CA signature. Go 1.26+ ignores
	// the io.Reader in ecdsa.Sign and always uses system randomness, so we must
	// implement deterministic ECDSA ourselves via a custom crypto.Signer.
	caSigner := &deterministicECDSASigner{key: caKey}

	// The rand reader is unused: serial number is set explicitly and signing
	// goes through our deterministic crypto.Signer.
	certDER, err := stdlibx509.CreateCertificate(nil, template, caCert, &adminKey.PublicKey, caSigner)
	if err != nil {
		return nil, fmt.Errorf("error signing admin certificate: %w", err)
	}

	// PEM-encode the client cert and key.
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	adminKeyDER, err := stdlibx509.MarshalECPrivateKey(adminKey)
	if err != nil {
		return nil, fmt.Errorf("error marshaling admin private key: %w", err)
	}

	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: adminKeyDER})

	// Assemble kubeconfig.
	cfg := clientcmdapi.Config{
		APIVersion: "v1",
		Kind:       "Config",
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server:                   endpoint,
				CertificateAuthorityData: bundle.Certs.K8s.Crt,
			},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"admin@" + clusterName: {
				ClientCertificateData: clientCertPEM,
				ClientKeyData:         clientKeyPEM,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"admin@" + clusterName: {
				Cluster:   clusterName,
				Namespace: "default",
				AuthInfo:  "admin@" + clusterName,
			},
		},
		CurrentContext: "admin@" + clusterName,
	}

	marshaled, err := clientcmd.Write(cfg)
	if err != nil {
		return nil, fmt.Errorf("error marshaling kubeconfig: %w", err)
	}

	return &KubeconfigResult{
		Raw:           string(marshaled),
		ClientCertPEM: clientCertPEM,
		ClientKeyPEM:  clientKeyPEM,
	}, nil
}
