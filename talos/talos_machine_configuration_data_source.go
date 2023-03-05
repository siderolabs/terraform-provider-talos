// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/siderolabs/crypto/x509"
	"github.com/siderolabs/gen/slices"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/generate"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/terraform-provider-talos/talos/internal/tfutils"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"gopkg.in/yaml.v3"
)

type talosMachineConfigurationDataSource struct {
	clusterName          string
	clusterEndpoint      string
	machineSecrets       generate.SecretsBundle
	machineType          string
	configPatches        []string
	kubernetesVersion    string
	talosVersion         string
	docs                 bool
	examples             bool
	machineConfiguration string

	configPatchesType  []tftypes.Value
	machineSecretsType map[string]tftypes.Value
}

// NewTalosMachineConfigurationDataSource is a helper function to simplify the provider implementation.
func NewTalosMachineConfigurationDataSource() tfprotov6.DataSourceServer {
	return &talosMachineConfigurationDataSource{}
}

func (d *talosMachineConfigurationDataSource) ReadDataSource(ctx context.Context, req *tfprotov6.ReadDataSourceRequest) (*tfprotov6.ReadDataSourceResponse, error) {
	config := req.Config

	resp := &tfprotov6.ReadDataSourceResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}

	if err := d.populateConfigValues(config); err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error unmarshaling config",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	if err := d.generateMachineConfig(ctx); err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error generating machine config",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	state, err := tfprotov6.NewDynamicValue(machineConfigurationDataSourceSchemaObject(), tftypes.NewValue(machineConfigurationDataSourceSchemaObject(), map[string]tftypes.Value{
		"id":                    tftypes.NewValue(tftypes.String, "machine_configuration"),
		"cluster_name":          tftypes.NewValue(tftypes.String, d.clusterName),
		"cluster_endpoint":      tftypes.NewValue(tftypes.String, d.clusterEndpoint),
		"machine_secrets":       tftypes.NewValue(machineSecretsSchemaObject(), d.machineSecretsType),
		"type":                  tftypes.NewValue(tftypes.String, d.machineType),
		"config_patches":        tftypes.NewValue(tftypes.List{ElementType: tftypes.DynamicPseudoType}, d.configPatchesType),
		"kubernetes_version":    tftypes.NewValue(tftypes.String, d.kubernetesVersion),
		"talos_version":         tftypes.NewValue(tftypes.String, d.talosVersion),
		"docs":                  tftypes.NewValue(tftypes.Bool, d.docs),
		"examples":              tftypes.NewValue(tftypes.Bool, d.examples),
		"machine_configuration": tftypes.NewValue(tftypes.String, d.machineConfiguration),
	}))
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error creating state",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	return &tfprotov6.ReadDataSourceResponse{
		State: &state,
	}, nil
}

func (d *talosMachineConfigurationDataSource) ValidateDataResourceConfig(ctx context.Context, req *tfprotov6.ValidateDataResourceConfigRequest) (*tfprotov6.ValidateDataResourceConfigResponse, error) {
	return &tfprotov6.ValidateDataResourceConfigResponse{}, nil
}

func (d *talosMachineConfigurationDataSource) populateConfigValues(config *tfprotov6.DynamicValue) error {
	// set defaults
	d.docs = true
	d.examples = true
	d.kubernetesVersion = constants.DefaultKubernetesVersion

	val, err := config.Unmarshal(machineConfigurationDataSourceSchemaObject())
	if err != nil {
		return err
	}

	var valMap map[string]tftypes.Value

	if err := val.As(&valMap); err != nil {
		return err
	}

	if !valMap["cluster_name"].IsNull() && valMap["cluster_name"].IsFullyKnown() {
		if err := valMap["cluster_name"].As(&d.clusterName); err != nil {
			return err
		}

		if len(d.clusterName) == 0 {
			return fmt.Errorf("cluster_name cannot be empty")
		}
	}

	if !valMap["cluster_endpoint"].IsNull() && valMap["cluster_endpoint"].IsFullyKnown() {
		if err := valMap["cluster_endpoint"].As(&d.clusterEndpoint); err != nil {
			return err
		}

		if err := validateClusterEndpoint(d.clusterEndpoint); err != nil {
			return err
		}
	}

	var secretsBundle generate.SecretsBundle

	if !valMap["machine_secrets"].IsNull() && valMap["machine_secrets"].IsFullyKnown() {
		msv := map[string]tftypes.Value{}
		err := valMap["machine_secrets"].As(&msv)
		if err != nil {
			return err
		}

		d.machineSecretsType = msv

		for k, v := range msv {
			switch k {
			case "cluster":
				cluster := map[string]tftypes.Value{}
				if err := v.As(&cluster); err != nil {
					return err
				}

				var id string
				var secret string

				if err := cluster["id"].As(&id); err != nil {
					return err
				}

				if err := cluster["secret"].As(&secret); err != nil {
					return err
				}

				secretsBundle.Cluster = &generate.Cluster{
					ID:     id,
					Secret: secret,
				}
			case "secrets":
				secrets := map[string]tftypes.Value{}
				if err := v.As(&secrets); err != nil {
					return err
				}

				var bootstrapToken string
				var secretboxEncryptionSecret string
				var aescbcEncryptionSecret string

				if err := secrets["bootstrap_token"].As(&bootstrapToken); err != nil {
					return err
				}

				if err := secrets["secretbox_encryption_secret"].As(&secretboxEncryptionSecret); err != nil {
					return err
				}

				if !secrets["aescbc_encryption_secret"].IsNull() && secrets["aescbc_encryption_secret"].IsFullyKnown() {
					if err := secrets["aescbc_encryption_secret"].As(&aescbcEncryptionSecret); err != nil {
						return err
					}
				}

				secretsBundle.Secrets = &generate.Secrets{
					BootstrapToken:            bootstrapToken,
					SecretboxEncryptionSecret: secretboxEncryptionSecret,
				}

				if aescbcEncryptionSecret != "" {
					secretsBundle.Secrets.AESCBCEncryptionSecret = aescbcEncryptionSecret
				}
			case "trustdinfo":
				trustdinfo := map[string]tftypes.Value{}
				if err := v.As(&trustdinfo); err != nil {
					return err
				}

				var token string

				if err := trustdinfo["token"].As(&token); err != nil {
					return err
				}

				secretsBundle.TrustdInfo = &generate.TrustdInfo{
					Token: token,
				}
			case "certs":
				certs := map[string]tftypes.Value{}
				if err := v.As(&certs); err != nil {
					return err
				}

				var etcdCertData map[string]tftypes.Value
				var k8sCertData map[string]tftypes.Value
				var k8sAggregatorCertData map[string]tftypes.Value
				var k8sServiceAccountCertData map[string]tftypes.Value
				var osCertData map[string]tftypes.Value

				if err := certs["etcd"].As(&etcdCertData); err != nil {
					return err
				}

				if err := certs["k8s"].As(&k8sCertData); err != nil {
					return err
				}

				if err := certs["k8s_aggregator"].As(&k8sAggregatorCertData); err != nil {
					return err
				}

				if err := certs["k8s_serviceaccount"].As(&k8sServiceAccountCertData); err != nil {
					return err
				}

				if err := certs["os"].As(&osCertData); err != nil {
					return err
				}

				etcdCertDataX509, err := certDataToX509PEMEncodedCertificateAndKey(etcdCertData)
				if err != nil {
					return err
				}

				k8sCertDataX509, err := certDataToX509PEMEncodedCertificateAndKey(k8sCertData)
				if err != nil {
					return err
				}

				k8sAggregatorCertDataX509, err := certDataToX509PEMEncodedCertificateAndKey(k8sAggregatorCertData)
				if err != nil {
					return err
				}

				k8sServiceAccountCertDataX509, err := certDataToX509PEMEncodedKey(k8sServiceAccountCertData)
				if err != nil {
					return err
				}

				osCertDataX509, err := certDataToX509PEMEncodedCertificateAndKey(osCertData)
				if err != nil {
					return err
				}

				secretsBundle.Certs = &generate.Certs{
					Etcd:              etcdCertDataX509,
					K8s:               k8sCertDataX509,
					K8sAggregator:     k8sAggregatorCertDataX509,
					K8sServiceAccount: k8sServiceAccountCertDataX509,
					OS:                osCertDataX509,
				}
			}
		}

		secretsBundle.Clock = generate.NewClock()

		d.machineSecrets = secretsBundle
	}

	if !valMap["type"].IsNull() && valMap["type"].IsFullyKnown() {
		if err := valMap["type"].As(&d.machineType); err != nil {
			return err
		}

		if !slices.Contains([]string{"controlplane", "worker"}, func(s string) bool {
			return s == d.machineType
		}) {
			return fmt.Errorf("type must be either controlplane or worker")
		}
	}

	if !valMap["config_patches"].IsNull() && valMap["config_patches"].IsFullyKnown() {
		var configPatchesType []tftypes.Value
		if err := valMap["config_patches"].As(&configPatchesType); err != nil {
			return err
		}

		d.configPatchesType = configPatchesType

		for _, configPatch := range configPatchesType {
			intf, err := tfutils.TFTypesToInterface(configPatch, tftypes.NewAttributePath())
			if err != nil {
				return err
			}

			patchBytes, err := yaml.Marshal(intf)
			if err != nil {
				return err
			}

			d.configPatches = append(d.configPatches, string(patchBytes))
		}
	}

	if !valMap["kubernetes_version"].IsNull() && valMap["kubernetes_version"].IsFullyKnown() {
		var k8sVersion string
		if err := valMap["kubernetes_version"].As(&k8sVersion); err != nil {
			return err
		}

		d.kubernetesVersion = strings.TrimPrefix(k8sVersion, "v")

		if len(d.kubernetesVersion) == 0 {
			return fmt.Errorf("kubernetes_version cannot be empty")
		}
	}

	if !valMap["talos_version"].IsNull() && valMap["talos_version"].IsFullyKnown() {
		if err := valMap["talos_version"].As(&d.talosVersion); err != nil {
			return err
		}

		if _, err := validateVersionContract(d.talosVersion); err != nil {
			return err
		}
	}

	if !valMap["docs"].IsNull() && valMap["docs"].IsFullyKnown() {
		if err := valMap["docs"].As(&d.docs); err != nil {
			return err
		}
	}

	if !valMap["examples"].IsNull() && valMap["examples"].IsFullyKnown() {
		if err := valMap["examples"].As(&d.examples); err != nil {
			return err
		}
	}

	return nil
}

func (d *talosMachineConfigurationDataSource) generateMachineConfig(ctx context.Context) error {
	var machineType machine.Type

	switch d.machineType {
	case "controlplane":
		machineType = machine.TypeControlPlane
	case "worker":
		machineType = machine.TypeWorker
	}

	genOptions := &machineConfigGenerateOptions{
		machineType:       machineType,
		clusterName:       d.clusterName,
		clusterEndpoint:   d.clusterEndpoint,
		machineSecrets:    &d.machineSecrets,
		configPatches:     d.configPatches,
		kubernetesVersion: d.kubernetesVersion,
		talosVersion:      d.talosVersion,
		docsEnabled:       d.docs,
		examplesEnabled:   d.examples,
	}

	machineConfiguration, err := genOptions.generate()
	if err != nil {
		return err
	}

	d.machineConfiguration = machineConfiguration

	buf, err := json.Marshal(machineConfiguration)
	if err != nil {
		return err
	}

	v := ctyjson.SimpleJSONValue{}
	err = v.UnmarshalJSON(buf)
	if err != nil {
		return fmt.Errorf("could not unmarshal output value: %v", err)
	}

	return nil
}

func certDataToX509PEMEncodedCertificateAndKey(certData map[string]tftypes.Value) (*x509.PEMEncodedCertificateAndKey, error) {
	var key string
	var cert string

	if err := certData["key"].As(&key); err != nil {
		return nil, err
	}

	if err := certData["cert"].As(&cert); err != nil {
		return nil, err
	}

	certBytes, err := base64ToBytes(cert)
	if err != nil {
		return nil, err
	}

	keyBytes, err := base64ToBytes(key)
	if err != nil {
		return nil, err
	}

	return &x509.PEMEncodedCertificateAndKey{
		Key: keyBytes,
		Crt: certBytes,
	}, nil
}

func certDataToX509PEMEncodedKey(certData map[string]tftypes.Value) (*x509.PEMEncodedKey, error) {
	var key string

	if err := certData["key"].As(&key); err != nil {
		return nil, err
	}

	keyBytes, err := base64ToBytes(key)
	if err != nil {
		return nil, err
	}

	return &x509.PEMEncodedKey{
		Key: keyBytes,
	}, nil
}

func certDataWithOptionalCertSchemaTFType(withCert bool) tftypes.Object {
	attrs := map[string]tftypes.Type{
		"key": tftypes.String,
	}
	if withCert {
		attrs["cert"] = tftypes.String
	}

	return tftypes.Object{
		AttributeTypes: attrs,
	}
}

func machineConfigurationDataSourceSchemaObject() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":                    tftypes.String,
			"cluster_name":          tftypes.String,
			"cluster_endpoint":      tftypes.String,
			"machine_secrets":       machineSecretsSchemaObject(),
			"type":                  tftypes.String,
			"config_patches":        tftypes.List{ElementType: tftypes.DynamicPseudoType},
			"kubernetes_version":    tftypes.String,
			"talos_version":         tftypes.String,
			"docs":                  tftypes.Bool,
			"examples":              tftypes.Bool,
			"machine_configuration": tftypes.String,
		},
	}
}

func machineSecretsSchemaObject() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"cluster": tftypes.Object{
				AttributeTypes: map[string]tftypes.Type{
					"id":     tftypes.String,
					"secret": tftypes.String,
				},
			},
			"secrets": tftypes.Object{
				AttributeTypes: map[string]tftypes.Type{
					"bootstrap_token":             tftypes.String,
					"secretbox_encryption_secret": tftypes.String,
					"aescbc_encryption_secret":    tftypes.String,
				},
			},
			"trustdinfo": tftypes.Object{
				AttributeTypes: map[string]tftypes.Type{
					"token": tftypes.String,
				},
			},
			"certs": tftypes.Object{
				AttributeTypes: map[string]tftypes.Type{
					"etcd":               certDataWithOptionalCertSchemaTFType(true),
					"k8s":                certDataWithOptionalCertSchemaTFType(true),
					"k8s_aggregator":     certDataWithOptionalCertSchemaTFType(true),
					"k8s_serviceaccount": certDataWithOptionalCertSchemaTFType(false),
					"os":                 certDataWithOptionalCertSchemaTFType(true),
				},
			},
		},
	}
}
