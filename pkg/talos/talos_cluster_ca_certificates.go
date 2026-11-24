// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// talos_cluster exposes the public CA certificates of the cluster, computed from
// an optional machine secrets bundle. Terraform does not let a value derived from
// an ephemeral value flow into a regular argument, so the CA certificates of a
// bundle read from an ephemeral source cannot be used elsewhere (for example to
// configure a Vault Kubernetes auth backend). Passing the bundle to talos_cluster
// as a write-only argument makes the certificates available as ordinary
// attributes, already known at plan time. Only certificates are exposed, never
// keys.

const (
	clusterMachineSecretsAttribute   = "machine_secrets"
	clusterMachineSecretsWOAttribute = "machine_secrets_wo"
)

// clusterCACertificate is a CA certificate talos_cluster exposes.
type clusterCACertificate struct {
	// field is the attribute in the resource model.
	field func(*talosClusterResourceModel) *types.String
	// attribute is the name of the attribute.
	attribute string
	// bundle is the name of the certificate in certs of the machine secrets.
	bundle string
	// name is how the certificate is called in descriptions and errors.
	name string
}

// clusterCACertificates are the CA certificates exposed, all of the bundle's
// certificates. The service account has a key only, so it is not one of them.
var clusterCACertificates = []clusterCACertificate{
	{
		attribute: "kubernetes_ca_certificate", bundle: "k8s", name: "Kubernetes CA certificate",
		field: func(m *talosClusterResourceModel) *types.String { return &m.KubernetesCACertificate },
	},
	{
		attribute: "talos_ca_certificate", bundle: "os", name: "Talos CA certificate",
		field: func(m *talosClusterResourceModel) *types.String { return &m.TalosCACertificate },
	},
	{
		attribute: "etcd_ca_certificate", bundle: "etcd", name: "etcd CA certificate",
		field: func(m *talosClusterResourceModel) *types.String { return &m.EtcdCACertificate },
	},
	{
		attribute: "kubernetes_aggregator_ca_certificate", bundle: "k8s_aggregator", name: "Kubernetes aggregator CA certificate",
		field: func(m *talosClusterResourceModel) *types.String { return &m.KubernetesAggregatorCACertificate },
	},
}

// clusterCAConfigPath is the place of a CA certificate in a machine secrets input.
func clusterCAConfigPath(input string, ca clusterCACertificate) path.Path {
	return path.Root(input).AtName("certs").AtName(ca.bundle).AtName("cert")
}

// clusterMachineSecretsSchemaAttribute describes the machine secrets bundle, the
// same shape as the machine_secrets argument of the talos_machine_configuration
// data source and of the ephemeral resources. writeOnly makes the attribute, and
// every nested attribute, write-only.
func clusterMachineSecretsSchemaAttribute(writeOnly bool, description string) schema.SingleNestedAttribute {
	str := func(description string, required, sensitive bool) schema.StringAttribute {
		return schema.StringAttribute{
			Required:    required,
			Optional:    !required,
			Sensitive:   sensitive,
			WriteOnly:   writeOnly,
			Description: description,
		}
	}

	object := func(description string, attributes map[string]schema.Attribute) schema.SingleNestedAttribute {
		return schema.SingleNestedAttribute{
			Required:    true,
			WriteOnly:   writeOnly,
			Description: description,
			Attributes:  attributes,
		}
	}

	certKeyPair := func(description string) schema.SingleNestedAttribute {
		return object(description, map[string]schema.Attribute{
			"cert": str("The certificate, PEM encoded and then base64 encoded.", true, false),
			"key":  str("The key, PEM encoded and then base64 encoded.", true, true),
		})
	}

	return schema.SingleNestedAttribute{
		Optional:    true,
		WriteOnly:   writeOnly,
		Description: description,
		Attributes: map[string]schema.Attribute{
			"cluster": object("The cluster secrets", map[string]schema.Attribute{
				"id":     str("The cluster id", true, false),
				"secret": str("The cluster secret", true, true),
			}),
			"secrets": object("The secrets for the talos kubernetes cluster", map[string]schema.Attribute{
				"bootstrap_token":             str("The bootstrap token for the talos kubernetes cluster", true, true),
				"secretbox_encryption_secret": str("The secretbox encryption secret for the talos kubernetes cluster", true, true),
				"aescbc_encryption_secret":    str("The aescbc encryption secret for the talos kubernetes cluster", false, true),
			}),
			"trustdinfo": object("The trustd info for the talos kubernetes cluster", map[string]schema.Attribute{
				"token": str("The trustd token for the talos kubernetes cluster", true, true),
			}),
			"certs": object("The certs for the talos kubernetes cluster", map[string]schema.Attribute{
				"etcd":           certKeyPair("The etcd CA"),
				"k8s":            certKeyPair("The Kubernetes CA"),
				"k8s_aggregator": certKeyPair("The Kubernetes aggregator CA"),
				"k8s_serviceaccount": object("The Kubernetes service account key", map[string]schema.Attribute{
					"key": str("The key for the k8s service account", true, true),
				}),
				"os": certKeyPair("The Talos OS CA"),
			}),
		},
	}
}

func clusterCACertificateAttribute(ca clusterCACertificate) schema.StringAttribute {
	return schema.StringAttribute{
		Computed: true,
		Description: fmt.Sprintf(
			"The %s, PEM encoded and then base64 encoded like the certificates of a secrets bundle. "+
				"It is `certs.%s.cert` of `machine_secrets` or `machine_secrets_wo`, known at plan time, so it can be used where "+
				"a value derived from an ephemeral resource cannot. Null when neither is set. Only the certificate is exposed, never the key.",
			ca.name, ca.bundle,
		),
	}
}

// validateClusterCACertificate checks that a CA certificate field of a machine
// secrets bundle holds PEM encoded CA certificates and nothing else. It can hold
// several, for example the old and the new CA during a rotation, and all of them
// are exposed, so all of them are checked.
func validateClusterCACertificate(encoded string) error {
	pemBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return fmt.Errorf("not base64 encoded: %w", err)
	}

	certificates := 0

	for rest := pemBytes; len(bytes.TrimSpace(rest)) > 0; {
		var block *pem.Block

		block, rest = pem.Decode(rest)
		if block == nil {
			return errors.New("not a PEM encoded certificate, or unexpected data after the certificates")
		}

		if block.Type != "CERTIFICATE" {
			return fmt.Errorf("unexpected PEM block %q, want CERTIFICATE", block.Type)
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return fmt.Errorf("not a valid certificate: %w", err)
		}

		if !cert.IsCA {
			return errors.New("not a CA certificate")
		}

		certificates++
	}

	if certificates == 0 {
		return errors.New("not a PEM encoded certificate")
	}

	return nil
}

// clusterMachineSecretsSelection is the machine secrets input of a config.
type clusterMachineSecretsSelection struct {
	// attribute is the input that is set, or that is not known yet. It is empty
	// when there is none.
	attribute string
	// unknown is true when the input is not known yet: it may still turn out to
	// be null.
	unknown bool
}

// clusterMachineSecretsInput finds the machine secrets input of the config.
// The nested attributes of an unknown object read back as null, so the objects
// themselves have to be checked first.
func clusterMachineSecretsInput(ctx context.Context, config tfsdk.Config) (clusterMachineSecretsSelection, diag.Diagnostics) {
	var diags diag.Diagnostics

	read := func(name string) types.Object {
		var v types.Object

		diags.Append(config.GetAttribute(ctx, path.Root(name), &v)...)

		return v
	}

	fromWO, fromSecrets := read(clusterMachineSecretsWOAttribute), read(clusterMachineSecretsAttribute)

	switch {
	case diags.HasError():
		return clusterMachineSecretsSelection{}, diags
	case fromWO.IsUnknown():
		return clusterMachineSecretsSelection{attribute: clusterMachineSecretsWOAttribute, unknown: true}, diags
	case fromSecrets.IsUnknown():
		return clusterMachineSecretsSelection{attribute: clusterMachineSecretsAttribute, unknown: true}, diags
	case !fromWO.IsNull():
		return clusterMachineSecretsSelection{attribute: clusterMachineSecretsWOAttribute}, diags
	case !fromSecrets.IsNull():
		return clusterMachineSecretsSelection{attribute: clusterMachineSecretsAttribute}, diags
	default:
		return clusterMachineSecretsSelection{}, diags
	}
}

// clusterMachineSecretsConflict reports whether both inputs are set. An input
// that is not known yet may still be null, so it is not a conflict.
func clusterMachineSecretsConflict(cfg *talosClusterResourceModel) bool {
	known := func(v basetypes.ObjectValue) bool { return !v.IsNull() && !v.IsUnknown() }

	return known(cfg.MachineSecrets) && known(cfg.MachineSecretsWO)
}

// clusterCACertificateFromInput returns a CA certificate of the machine secrets
// input that is set: null when none is, unknown while the input, one of the
// objects on the way to the certificate, or the certificate is not known yet.
// Only that one value is read here.
func clusterCACertificateFromInput(ctx context.Context, config tfsdk.Config, input clusterMachineSecretsSelection, ca clusterCACertificate) (types.String, diag.Diagnostics) {
	var diags diag.Diagnostics

	switch {
	case input.attribute == "":
		return types.StringNull(), diags
	case input.unknown:
		return types.StringUnknown(), diags
	}

	// A nested value under an unknown object reads back as null, not unknown.
	certs := path.Root(input.attribute).AtName("certs")

	for _, p := range []path.Path{certs, certs.AtName(ca.bundle)} {
		var object types.Object

		diags.Append(config.GetAttribute(ctx, p, &object)...)

		if diags.HasError() {
			return types.StringNull(), diags
		}

		if object.IsUnknown() {
			return types.StringUnknown(), diags
		}
	}

	var v types.String

	diags.Append(config.GetAttribute(ctx, clusterCAConfigPath(input.attribute, ca), &v)...)

	return v, diags
}

// validateClusterMachineSecrets rejects both inputs together and a CA certificate
// that is not a CA certificate.
func validateClusterMachineSecrets(ctx context.Context, cfg *talosClusterResourceModel, config tfsdk.Config) diag.Diagnostics {
	var diags diag.Diagnostics

	if clusterMachineSecretsConflict(cfg) {
		diags.AddError(
			"Conflicting machine secrets",
			"Only one of machine_secrets or machine_secrets_wo can be set, not both.",
		)

		return diags
	}

	input, inputDiags := clusterMachineSecretsInput(ctx, config)
	diags.Append(inputDiags...)

	if inputDiags.HasError() {
		return diags
	}

	for _, ca := range clusterCACertificates {
		cert, certDiags := clusterCACertificateFromInput(ctx, config, input, ca)

		diags.Append(certDiags...)

		if certDiags.HasError() || cert.IsNull() || cert.IsUnknown() {
			continue
		}

		if err := validateClusterCACertificate(cert.ValueString()); err != nil {
			diags.AddAttributeError(clusterCAConfigPath(input.attribute, ca), "Invalid "+ca.name, err.Error())
		}
	}

	return diags
}

// resolveClusterCACertificates fills the planned CA certificates that plan could
// not know (the bundle was unknown then), now that the config is final. A
// conflict that validation could not see then is caught here.
func resolveClusterCACertificates(ctx context.Context, config tfsdk.Config, plan *talosClusterResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	unresolved := false

	for _, ca := range clusterCACertificates {
		unresolved = unresolved || ca.field(plan).IsUnknown()
	}

	if !unresolved {
		return diags
	}

	var cfg talosClusterResourceModel

	diags.Append(config.Get(ctx, &cfg)...)

	if diags.HasError() {
		return diags
	}

	if clusterMachineSecretsConflict(&cfg) {
		diags.AddError(
			"Conflicting machine secrets",
			"Only one of machine_secrets or machine_secrets_wo can be set, not both.",
		)

		return diags
	}

	input, inputDiags := clusterMachineSecretsInput(ctx, config)
	diags.Append(inputDiags...)

	if diags.HasError() {
		return diags
	}

	for _, ca := range clusterCACertificates {
		planned := ca.field(plan)
		if !planned.IsUnknown() {
			continue
		}

		cert, certDiags := clusterCACertificateFromInput(ctx, config, input, ca)

		diags.Append(certDiags...)

		if diags.HasError() {
			return diags
		}

		if cert.IsUnknown() {
			diags.AddError("Unknown "+ca.name, "The machine secrets input must be known at apply time.")

			return diags
		}

		if !cert.IsNull() {
			if err := validateClusterCACertificate(cert.ValueString()); err != nil {
				diags.AddAttributeError(clusterCAConfigPath(input.attribute, ca), "Invalid "+ca.name, err.Error())

				return diags
			}
		}

		*planned = cert
	}

	return diags
}
