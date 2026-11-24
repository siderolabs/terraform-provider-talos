// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"maps"
	"math/big"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// These tests drive the provider through its protocol server, the way Terraform
// does, to check that talos_cluster exposes the CA certificates of an optional
// machine secrets bundle as normal (non-ephemeral) values that are already known
// at plan time.

const (
	caAttrOutput = "kubernetes_ca_certificate"
	caAttrSecret = "machine_secrets"
	caAttrWO     = "machine_secrets_wo"
	caK8sCert    = "certs.k8s.cert"
	caNode       = "10.0.0.1"
)

// caStateAlpha2 is talos_cluster state as written by v0.13.0-alpha.2, before the
// machine secrets inputs and the CA outputs existed.
const caStateAlpha2 = `{
  "id": "10.0.0.1",
  "node": "10.0.0.1",
  "endpoint": "10.0.0.1",
  "control_plane_nodes": ["10.0.0.1"],
  "kubernetes_version": "v1.35.4",
  "client_configuration": {
    "ca_certificate": "Zm9v",
    "client_certificate": "YmFy",
    "client_key": "YmF6"
  },
  "timeouts": null
}`

type caHarness struct {
	server  tfprotov6.ProviderServer
	objType tftypes.Object
	version int64
}

func newCAHarness(t *testing.T) *caHarness {
	t.Helper()

	server, err := providerserver.NewProtocol6WithError(talos.New())()
	if err != nil {
		t.Fatal(err)
	}

	schemaResp, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}

	caFatalOnErrors(t, "GetProviderSchema", schemaResp.Diagnostics)

	resourceSchema := schemaResp.ResourceSchemas["talos_cluster"]

	objType, ok := resourceSchema.ValueType().(tftypes.Object)
	if !ok {
		t.Fatalf("talos_cluster schema type is %T, want tftypes.Object", resourceSchema.ValueType())
	}

	return &caHarness{server: server, objType: objType, version: resourceSchema.Version}
}

// requireAttributes fails the test, instead of panicking later, when the schema
// does not have the attributes the scenario needs.
func (h *caHarness) requireAttributes(t *testing.T, names ...string) {
	t.Helper()

	for _, name := range names {
		if _, ok := h.objType.AttributeTypes[name]; !ok {
			t.Fatalf("talos_cluster has no %q attribute", name)
		}
	}
}

// object builds a talos_cluster value: every attribute is null except the ones in set.
func (h *caHarness) object(set map[string]tftypes.Value) tftypes.Value {
	attrs := make(map[string]tftypes.Value, len(h.objType.AttributeTypes))

	for name, typ := range h.objType.AttributeTypes {
		attrs[name] = tftypes.NewValue(typ, nil)
	}

	maps.Copy(attrs, set)

	return tftypes.NewValue(h.objType, attrs)
}

// bundle builds a machine secrets value. Every string is a placeholder except
// the entries of overrides, keyed by their path inside the bundle.
func (h *caHarness) bundle(t *testing.T, attribute string, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	h.requireAttributes(t, attribute)

	// Every exposed certificate is validated, so the ones a scenario does not
	// care about must be valid certificates too.
	all := make(map[string]tftypes.Value, len(overrides)+len(caOutputs))

	for _, o := range caOutputs {
		all["certs."+o.bundle+".cert"] = caStringValue(caCertificate(t, o.attribute))
	}

	maps.Copy(all, overrides)

	return caFill(t, h.objType.AttributeTypes[attribute], "", all)
}

// clientConfiguration is the client_configuration of caStateAlpha2.
func (h *caHarness) clientConfiguration(t *testing.T) tftypes.Value {
	t.Helper()

	return caFill(t, h.objType.AttributeTypes["client_configuration"], "", map[string]tftypes.Value{
		"ca_certificate":     caStringValue("Zm9v"),
		"client_certificate": caStringValue("YmFy"),
		"client_key":         caStringValue("YmF6"),
	})
}

// baseConfig is the configuration of an existing cluster that does not use the
// machine secrets inputs. Computed attributes are not set, as in a real config.
func (h *caHarness) baseConfig(t *testing.T, set map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	cfg := map[string]tftypes.Value{
		"node":                 tftypes.NewValue(tftypes.String, caNode),
		"kubernetes_version":   tftypes.NewValue(tftypes.String, "v1.35.4"),
		"client_configuration": h.clientConfiguration(t),
	}

	maps.Copy(cfg, set)

	return h.object(cfg)
}

// priorState is the state after an apply of baseConfig: the computed attributes
// are filled in. extra lets a scenario put values into it.
func (h *caHarness) priorState(t *testing.T, extra map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	state := map[string]tftypes.Value{
		"id":       tftypes.NewValue(tftypes.String, caNode),
		"node":     tftypes.NewValue(tftypes.String, caNode),
		"endpoint": tftypes.NewValue(tftypes.String, caNode),
		"control_plane_nodes": tftypes.NewValue(
			h.objType.AttributeTypes["control_plane_nodes"],
			[]tftypes.Value{tftypes.NewValue(tftypes.String, caNode)},
		),
		"kubernetes_version":   tftypes.NewValue(tftypes.String, "v1.35.4"),
		"client_configuration": h.clientConfiguration(t),
	}

	maps.Copy(state, extra)

	return h.object(state)
}

// proposed mimics Terraform core: the config for everything the user sets, the
// prior value for the computed attributes the user does not set.
func (h *caHarness) proposed(prior, config tftypes.Value) tftypes.Value {
	var priorAttrs, configAttrs map[string]tftypes.Value

	if err := prior.As(&priorAttrs); err != nil || prior.IsNull() {
		return config
	}

	if err := config.As(&configAttrs); err != nil {
		return config
	}

	for _, name := range []string{"id", "endpoint", "control_plane_nodes", caAttrOutput} {
		if configAttrs[name].IsNull() {
			configAttrs[name] = priorAttrs[name]
		}
	}

	return tftypes.NewValue(h.objType, configAttrs)
}

func (h *caHarness) dynamic(t *testing.T, v tftypes.Value) *tfprotov6.DynamicValue {
	t.Helper()

	dv, err := tfprotov6.NewDynamicValue(h.objType, v)
	if err != nil {
		t.Fatal(err)
	}

	return &dv
}

func (h *caHarness) validate(t *testing.T, config tftypes.Value) []*tfprotov6.Diagnostic {
	t.Helper()

	resp, err := h.server.ValidateResourceConfig(context.Background(), &tfprotov6.ValidateResourceConfigRequest{
		TypeName: "talos_cluster",
		Config:   h.dynamic(t, config),
		ClientCapabilities: &tfprotov6.ValidateResourceConfigClientCapabilities{
			WriteOnlyAttributesAllowed: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	return resp.Diagnostics
}

// plan runs PlanResourceChange and returns the planned state.
func (h *caHarness) plan(t *testing.T, prior, config tftypes.Value) tftypes.Value {
	t.Helper()

	resp, err := h.server.PlanResourceChange(context.Background(), &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "talos_cluster",
		PriorState:       h.dynamic(t, prior),
		ProposedNewState: h.dynamic(t, h.proposed(prior, config)),
		Config:           h.dynamic(t, config),
	})
	if err != nil {
		t.Fatal(err)
	}

	caFatalOnErrors(t, "PlanResourceChange", resp.Diagnostics)

	if len(resp.RequiresReplace) > 0 {
		t.Fatalf("RequiresReplace = %v, want none", resp.RequiresReplace)
	}

	planned, err := resp.PlannedState.Unmarshal(h.objType)
	if err != nil {
		t.Fatal(err)
	}

	return planned
}

// apply runs ApplyResourceChange for an update and returns the new state.
func (h *caHarness) apply(t *testing.T, prior, planned, config tftypes.Value) tftypes.Value {
	t.Helper()

	resp, err := h.server.ApplyResourceChange(context.Background(), &tfprotov6.ApplyResourceChangeRequest{
		TypeName:     "talos_cluster",
		PriorState:   h.dynamic(t, prior),
		PlannedState: h.dynamic(t, planned),
		Config:       h.dynamic(t, config),
	})
	if err != nil {
		t.Fatal(err)
	}

	caFatalOnErrors(t, "ApplyResourceChange", resp.Diagnostics)

	newState, err := resp.NewState.Unmarshal(h.objType)
	if err != nil {
		t.Fatal(err)
	}

	return newState
}

func caAttribute(t *testing.T, obj tftypes.Value, name string) tftypes.Value {
	t.Helper()

	var attrs map[string]tftypes.Value
	if err := obj.As(&attrs); err != nil {
		t.Fatal(err)
	}

	v, ok := attrs[name]
	if !ok {
		t.Fatalf("no attribute %q", name)
	}

	return v
}

// caString returns the value of a known, non-null string attribute.
func caString(t *testing.T, obj tftypes.Value, name string) string {
	t.Helper()

	v := caAttribute(t, obj, name)

	if !v.IsKnown() {
		t.Fatalf("%s is unknown, want a value known at plan time", name)
	}

	if v.IsNull() {
		t.Fatalf("%s is null, want a value", name)
	}

	var s string
	if err := v.As(&s); err != nil {
		t.Fatal(err)
	}

	return s
}

func caFill(t *testing.T, typ tftypes.Type, path string, overrides map[string]tftypes.Value) tftypes.Value {
	t.Helper()

	if v, ok := overrides[path]; ok {
		return v
	}

	if obj, ok := typ.(tftypes.Object); ok {
		attrs := make(map[string]tftypes.Value, len(obj.AttributeTypes))

		for name, attrType := range obj.AttributeTypes {
			p := name
			if path != "" {
				p = path + "." + name
			}

			attrs[name] = caFill(t, attrType, p, overrides)
		}

		return tftypes.NewValue(typ, attrs)
	}

	if typ.Is(tftypes.String) {
		return tftypes.NewValue(typ, "dGVzdA==") // base64 of "test"
	}

	t.Fatalf("unsupported type %s at %q", typ, path)

	return tftypes.Value{}
}

func caFatalOnErrors(t *testing.T, call string, diags []*tfprotov6.Diagnostic) {
	t.Helper()

	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("%s: %s: %s", call, d.Summary, d.Detail)
		}
	}
}

// caHasError reports whether diags holds an error with the given summary.
func caHasError(diags []*tfprotov6.Diagnostic, summary string) bool {
	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError && d.Summary == summary {
			return true
		}
	}

	return false
}

// caCertificate returns a self-signed CA certificate, PEM encoded and then base64
// encoded: the way certificates are stored in a Talos secrets bundle.
func caCertificate(t *testing.T, commonName string) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func caStringValue(s string) tftypes.Value {
	return tftypes.NewValue(tftypes.String, s)
}

// TestTalosClusterKubernetesCAIsKnownAtPlanTime is the point of the feature: the
// CA certificate of a write-only machine secrets bundle is available as a normal
// value during plan, not "known after apply".
func TestTalosClusterKubernetesCAIsKnownAtPlanTime(t *testing.T) {
	t.Parallel()

	for _, attribute := range []string{caAttrWO, caAttrSecret} {
		t.Run(attribute, func(t *testing.T) {
			t.Parallel()

			h := newCAHarness(t)
			ca := caCertificate(t, "kubernetes")

			config := h.baseConfig(t, map[string]tftypes.Value{
				attribute: h.bundle(t, attribute, map[string]tftypes.Value{caK8sCert: caStringValue(ca)}),
			})

			caFatalOnErrors(t, "ValidateResourceConfig", h.validate(t, config))

			planned := h.plan(t, tftypes.NewValue(h.objType, nil), config)

			if got := caString(t, planned, caAttrOutput); got != ca {
				t.Errorf("planned %s = %q, want the bundle's certs.k8s.cert %q", caAttrOutput, got, ca)
			}
		})
	}
}

// TestTalosClusterMachineSecretsWOIsNeverPlannedOrStored guards the secrets: a
// write-only value must not leak into the plan or the state.
func TestTalosClusterMachineSecretsWOIsNeverPlannedOrStored(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO, caAttrOutput)

	prior := h.priorState(t, nil)
	config := h.baseConfig(t, map[string]tftypes.Value{
		caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{caK8sCert: caStringValue(caCertificate(t, "kubernetes"))}),
	})

	planned := h.plan(t, prior, config)

	if v := caAttribute(t, planned, caAttrWO); !v.IsNull() {
		t.Errorf("planned %s = %s, want null: write-only values are never planned", caAttrWO, v)
	}

	newState := h.apply(t, prior, planned, config)

	if v := caAttribute(t, newState, caAttrWO); !v.IsNull() {
		t.Errorf("state %s = %s, want null: write-only values are never stored", caAttrWO, v)
	}

	if got, want := caString(t, newState, caAttrOutput), caString(t, planned, caAttrOutput); got != want {
		t.Errorf("state %s = %q, want the planned %q", caAttrOutput, got, want)
	}
}

func TestTalosClusterMachineSecretsInputsAreMutuallyExclusive(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)

	overrides := map[string]tftypes.Value{caK8sCert: caStringValue(caCertificate(t, "kubernetes"))}

	config := h.baseConfig(t, map[string]tftypes.Value{
		caAttrSecret: h.bundle(t, caAttrSecret, overrides),
		caAttrWO:     h.bundle(t, caAttrWO, overrides),
	})

	diags := h.validate(t, config)

	if !caHasError(diags, "Conflicting machine secrets") {
		t.Errorf("ValidateResourceConfig of machine_secrets and machine_secrets_wo together = %v, want a \"Conflicting machine secrets\" error", diags)
	}
}

func TestTalosClusterMachineSecretsMustHoldAParseableCertificate(t *testing.T) {
	t.Parallel()

	for name, cert := range map[string]string{
		"not base64":     "%%% not base64 %%%",
		"not a PEM":      base64.StdEncoding.EncodeToString([]byte("hello")),
		"not a x509 PEM": base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("junk")})),
		"empty":          "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newCAHarness(t)

			config := h.baseConfig(t, map[string]tftypes.Value{
				caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{caK8sCert: caStringValue(cert)}),
			})

			diags := h.validate(t, config)

			if !caHasError(diags, "Invalid Kubernetes CA certificate") {
				t.Errorf("ValidateResourceConfig of certs.k8s.cert %q = %v, want an \"Invalid Kubernetes CA certificate\" error", cert, diags)
			}
		})
	}
}

// TestTalosClusterNoMachineSecretsNoCA checks that existing configurations are
// untouched: a state written before the feature upgrades cleanly and plans no diff.
func TestTalosClusterNoMachineSecretsNoCA(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrOutput)

	upgradeResp, err := h.server.UpgradeResourceState(context.Background(), &tfprotov6.UpgradeResourceStateRequest{
		TypeName: "talos_cluster",
		Version:  h.version,
		RawState: &tfprotov6.RawState{JSON: []byte(caStateAlpha2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	caFatalOnErrors(t, "UpgradeResourceState", upgradeResp.Diagnostics)

	prior, err := upgradeResp.UpgradedState.Unmarshal(h.objType)
	if err != nil {
		t.Fatal(err)
	}

	if v := caAttribute(t, prior, caAttrOutput); !v.IsNull() {
		t.Errorf("upgraded state %s = %s, want null", caAttrOutput, v)
	}

	config := h.baseConfig(t, nil)

	caFatalOnErrors(t, "ValidateResourceConfig", h.validate(t, config))

	planned := h.plan(t, prior, config)

	diffs, err := prior.Diff(planned)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range diffs {
		t.Errorf("planned state differs from the prior state at %s: %s -> %s", d.Path, d.Value1, d.Value2)
	}
}

// TestTalosClusterCARotationIsAnInPlaceUpdate checks that a changed bundle shows
// up as a normal diff, and that applying it only stores the new certificate: it
// must not start a Kubernetes upgrade. The Kubernetes version is unchanged and
// there is no cluster to talk to, so a successful apply proves nothing was run.
func TestTalosClusterCARotationIsAnInPlaceUpdate(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO, caAttrOutput)

	oldCA := caCertificate(t, "kubernetes old")
	newCA := caCertificate(t, "kubernetes new")

	// The other exposed certificates are the same before and after the rotation.
	overrides := map[string]tftypes.Value{caK8sCert: caStringValue(newCA)}
	extra := map[string]tftypes.Value{caAttrOutput: caStringValue(oldCA)}

	for _, o := range caOutputs[1:] {
		cert := caStringValue(caCertificate(t, o.attribute))
		overrides["certs."+o.bundle+".cert"] = cert
		extra[o.attribute] = cert
	}

	prior := h.priorState(t, extra)
	config := h.baseConfig(t, map[string]tftypes.Value{caAttrWO: h.bundle(t, caAttrWO, overrides)})

	planned := h.plan(t, prior, config)

	if got := caString(t, planned, caAttrOutput); got != newCA {
		t.Fatalf("planned %s = %q, want the rotated certificate %q", caAttrOutput, got, newCA)
	}

	diffs, err := prior.Diff(planned)
	if err != nil {
		t.Fatal(err)
	}

	if len(diffs) != 1 {
		t.Errorf("plan has %d differences, want only %s: %v", len(diffs), caAttrOutput, diffs)
	}

	newState := h.apply(t, prior, planned, config)

	if got := caString(t, newState, caAttrOutput); got != newCA {
		t.Errorf("state %s = %q, want %q", caAttrOutput, got, newCA)
	}

	if got := caString(t, newState, "kubernetes_version"); got != "v1.35.4" {
		t.Errorf("state kubernetes_version = %q, want it unchanged", got)
	}
}

// TestTalosClusterUnknownBundleLeavesTheCAUnknown covers a bundle that is only
// known after apply (for example read from a secret created in the same run).
func TestTalosClusterUnknownBundleLeavesTheCAUnknown(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO, caAttrOutput)

	config := h.baseConfig(t, map[string]tftypes.Value{
		caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{
			caK8sCert: tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}),
	})

	caFatalOnErrors(t, "ValidateResourceConfig", h.validate(t, config))

	planned := h.plan(t, tftypes.NewValue(h.objType, nil), config)

	if v := caAttribute(t, planned, caAttrOutput); v.IsKnown() {
		t.Errorf("planned %s = %s, want unknown while the bundle is unknown", caAttrOutput, v)
	}
}

// TestTalosClusterCAIsResolvedAtApplyWhenTheBundleWasUnknownAtPlan covers the other
// half of the unknown case: once the bundle is known, apply stores its certificate.
func TestTalosClusterCAIsResolvedAtApplyWhenTheBundleWasUnknownAtPlan(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO, caAttrOutput)

	ca := caCertificate(t, "kubernetes")
	prior := h.priorState(t, nil)

	planConfig := h.baseConfig(t, map[string]tftypes.Value{
		caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{
			caK8sCert: tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
		}),
	})

	planned := h.plan(t, prior, planConfig)

	if v := caAttribute(t, planned, caAttrOutput); v.IsKnown() {
		t.Fatalf("planned %s = %s, want unknown while the bundle is unknown", caAttrOutput, v)
	}

	applyConfig := h.baseConfig(t, map[string]tftypes.Value{
		caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{caK8sCert: caStringValue(ca)}),
	})

	newState := h.apply(t, prior, planned, applyConfig)

	if got := caString(t, newState, caAttrOutput); got != ca {
		t.Errorf("state %s = %q, want %q", caAttrOutput, got, ca)
	}
}

// TestTalosClusterWithoutMachineSecretsTheCAStaysNull checks that an unrelated
// change does not turn the CA output into "known after apply" for users who do
// not use the feature: Terraform marks computed attributes unknown on any update.
func TestTalosClusterWithoutMachineSecretsTheCAStaysNull(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrOutput)

	prior := h.priorState(t, nil)
	config := h.baseConfig(t, map[string]tftypes.Value{
		"kubernetes_version": caStringValue("v1.35.5"),
	})

	planned := h.plan(t, prior, config)

	v := caAttribute(t, planned, caAttrOutput)
	if !v.IsKnown() || !v.IsNull() {
		t.Errorf("planned %s = %s, want a known null", caAttrOutput, v)
	}
}

// TestTalosClusterWholeUnknownBundleLeavesTheCAUnknown covers the common first
// apply: machine_secrets points at a resource that does not exist yet, so the
// whole bundle is unknown, not only its certificate. The CA must be unknown too,
// not null, or consumers in the same run would read null.
func TestTalosClusterWholeUnknownBundleLeavesTheCAUnknown(t *testing.T) {
	t.Parallel()

	for _, attribute := range []string{caAttrWO, caAttrSecret} {
		t.Run(attribute, func(t *testing.T) {
			t.Parallel()

			h := newCAHarness(t)
			h.requireAttributes(t, attribute, caAttrOutput)

			config := h.baseConfig(t, map[string]tftypes.Value{
				attribute: tftypes.NewValue(h.objType.AttributeTypes[attribute], tftypes.UnknownValue),
			})

			caFatalOnErrors(t, "ValidateResourceConfig", h.validate(t, config))

			planned := h.plan(t, tftypes.NewValue(h.objType, nil), config)

			if v := caAttribute(t, planned, caAttrOutput); v.IsKnown() {
				t.Errorf("planned %s = %s, want unknown while the whole bundle is unknown", caAttrOutput, v)
			}
		})
	}
}

// TestTalosClusterAnUnknownInputIsNotAConflict: an input that is unknown at plan
// time may well be null once known (a conditional expression), so only two known
// inputs conflict.
func TestTalosClusterAnUnknownInputIsNotAConflict(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO, caAttrSecret)

	config := h.baseConfig(t, map[string]tftypes.Value{
		caAttrSecret: h.bundle(t, caAttrSecret, map[string]tftypes.Value{caK8sCert: caStringValue(caCertificate(t, "kubernetes"))}),
		caAttrWO:     tftypes.NewValue(h.objType.AttributeTypes[caAttrWO], tftypes.UnknownValue),
	})

	diags := h.validate(t, config)

	if caHasError(diags, "Conflicting machine secrets") {
		t.Errorf("ValidateResourceConfig = %v, want no conflict while machine_secrets_wo is unknown", diags)
	}
}

// TestTalosClusterApplyErrorPointsAtTheInputThatIsSet checks the attribute of the
// error raised at apply for a bundle that was unknown at plan.
func TestTalosClusterApplyErrorPointsAtTheInputThatIsSet(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrSecret, caAttrOutput)

	prior := h.priorState(t, nil)

	planConfig := h.baseConfig(t, map[string]tftypes.Value{
		caAttrSecret: tftypes.NewValue(h.objType.AttributeTypes[caAttrSecret], tftypes.UnknownValue),
	})
	planned := h.plan(t, prior, planConfig)

	applyConfig := h.baseConfig(t, map[string]tftypes.Value{
		caAttrSecret: h.bundle(t, caAttrSecret, map[string]tftypes.Value{caK8sCert: caStringValue("not a certificate")}),
	})

	resp, err := h.server.ApplyResourceChange(context.Background(), &tfprotov6.ApplyResourceChangeRequest{
		TypeName:     "talos_cluster",
		PriorState:   h.dynamic(t, prior),
		PlannedState: h.dynamic(t, planned),
		Config:       h.dynamic(t, applyConfig),
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError && d.Summary == "Invalid Kubernetes CA certificate" {
			if d.Attribute == nil || !d.Attribute.Equal(tftypes.NewAttributePath().WithAttributeName(caAttrSecret).WithAttributeName("certs").WithAttributeName("k8s").WithAttributeName("cert")) {
				t.Errorf("error attribute = %v, want %s.certs.k8s.cert", d.Attribute, caAttrSecret)
			}

			return
		}
	}

	t.Errorf("ApplyResourceChange diagnostics = %v, want an \"Invalid Kubernetes CA certificate\" error", resp.Diagnostics)
}

// caOutputs are the CA certificates talos_cluster exposes, with the name of the
// certificate in the machine secrets bundle and the summary of the validation
// error for it.
var caOutputs = []struct {
	attribute string
	bundle    string
	summary   string
}{
	{"kubernetes_ca_certificate", "k8s", "Invalid Kubernetes CA certificate"},
	{"talos_ca_certificate", "os", "Invalid Talos CA certificate"},
	{"etcd_ca_certificate", "etcd", "Invalid etcd CA certificate"},
	{"kubernetes_aggregator_ca_certificate", "k8s_aggregator", "Invalid Kubernetes aggregator CA certificate"},
}

func caOutputAttributes() []string {
	names := make([]string, 0, len(caOutputs))

	for _, o := range caOutputs {
		names = append(names, o.attribute)
	}

	return names
}

// caDistinctBundle returns a machine secrets bundle with a different CA
// certificate for each of the exposed ones, and the certificates by output attribute.
func caDistinctBundle(t *testing.T, h *caHarness, attribute string) (tftypes.Value, map[string]string) {
	t.Helper()

	overrides := make(map[string]tftypes.Value, len(caOutputs))
	certs := make(map[string]string, len(caOutputs))

	for _, o := range caOutputs {
		cert := caCertificate(t, o.attribute)
		overrides["certs."+o.bundle+".cert"] = caStringValue(cert)
		certs[o.attribute] = cert
	}

	return h.bundle(t, attribute, overrides), certs
}

// TestTalosClusterEveryCACertificateIsKnownAtPlanTime checks that each exposed
// certificate comes from its own place in the bundle, not from another one's.
func TestTalosClusterEveryCACertificateIsKnownAtPlanTime(t *testing.T) {
	t.Parallel()

	for _, attribute := range []string{caAttrWO, caAttrSecret} {
		t.Run(attribute, func(t *testing.T) {
			t.Parallel()

			h := newCAHarness(t)
			h.requireAttributes(t, caOutputAttributes()...)

			bundle, certs := caDistinctBundle(t, h, attribute)
			config := h.baseConfig(t, map[string]tftypes.Value{attribute: bundle})

			caFatalOnErrors(t, "ValidateResourceConfig", h.validate(t, config))

			planned := h.plan(t, tftypes.NewValue(h.objType, nil), config)

			for _, o := range caOutputs {
				if got := caString(t, planned, o.attribute); got != certs[o.attribute] {
					t.Errorf("planned %s = %q, want certs.%s.cert %q", o.attribute, got, o.bundle, certs[o.attribute])
				}
			}
		})
	}
}

// TestTalosClusterEveryExposedCertificateMustBeParseable: a bundle with a broken
// certificate is rejected, whichever one it is.
func TestTalosClusterEveryExposedCertificateMustBeParseable(t *testing.T) {
	t.Parallel()

	for _, o := range caOutputs {
		t.Run(o.bundle, func(t *testing.T) {
			t.Parallel()

			h := newCAHarness(t)
			h.requireAttributes(t, o.attribute)

			config := h.baseConfig(t, map[string]tftypes.Value{
				caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{"certs." + o.bundle + ".cert": caStringValue("not a certificate")}),
			})

			diags := h.validate(t, config)

			if !caHasError(diags, o.summary) {
				t.Errorf("ValidateResourceConfig of certs.%s.cert = %v, want a %q error", o.bundle, diags, o.summary)
			}
		})
	}
}

// TestTalosClusterRotatingOneCAChangesOnlyThatOutput: the other outputs keep
// their value, so only the consumers of the rotated CA see a change.
func TestTalosClusterRotatingOneCAChangesOnlyThatOutput(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO)
	h.requireAttributes(t, caOutputAttributes()...)

	_, certs := caDistinctBundle(t, h, caAttrWO)

	extra := make(map[string]tftypes.Value, len(certs))
	for attribute, cert := range certs {
		extra[attribute] = caStringValue(cert)
	}

	prior := h.priorState(t, extra)

	rotated := caCertificate(t, "etcd rotated")
	overrides := make(map[string]tftypes.Value, len(caOutputs))

	for _, o := range caOutputs {
		overrides["certs."+o.bundle+".cert"] = caStringValue(certs[o.attribute])
	}

	overrides["certs.etcd.cert"] = caStringValue(rotated)

	config := h.baseConfig(t, map[string]tftypes.Value{caAttrWO: h.bundle(t, caAttrWO, overrides)})
	planned := h.plan(t, prior, config)

	diffs, err := prior.Diff(planned)
	if err != nil {
		t.Fatal(err)
	}

	if len(diffs) != 1 {
		t.Fatalf("plan has %d differences, want only etcd_ca_certificate: %v", len(diffs), diffs)
	}

	if got := caString(t, planned, "etcd_ca_certificate"); got != rotated {
		t.Errorf("planned etcd_ca_certificate = %q, want the rotated certificate %q", got, rotated)
	}
}

// TestTalosClusterWithoutMachineSecretsEveryCAIsNull: existing configurations
// see none of the new attributes, and a state from before the feature upgrades
// cleanly.
func TestTalosClusterWithoutMachineSecretsEveryCAIsNull(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caOutputAttributes()...)

	upgradeResp, err := h.server.UpgradeResourceState(context.Background(), &tfprotov6.UpgradeResourceStateRequest{
		TypeName: "talos_cluster",
		Version:  h.version,
		RawState: &tfprotov6.RawState{JSON: []byte(caStateAlpha2)},
	})
	if err != nil {
		t.Fatal(err)
	}

	caFatalOnErrors(t, "UpgradeResourceState", upgradeResp.Diagnostics)

	prior, err := upgradeResp.UpgradedState.Unmarshal(h.objType)
	if err != nil {
		t.Fatal(err)
	}

	// An unrelated change: Terraform marks every computed attribute unknown.
	planned := h.plan(t, prior, h.baseConfig(t, map[string]tftypes.Value{"kubernetes_version": caStringValue("v1.35.5")}))

	for _, attribute := range caOutputAttributes() {
		if v := caAttribute(t, prior, attribute); !v.IsNull() {
			t.Errorf("upgraded state %s = %s, want null", attribute, v)
		}

		if v := caAttribute(t, planned, attribute); !v.IsKnown() || !v.IsNull() {
			t.Errorf("planned %s = %s, want a known null", attribute, v)
		}
	}
}

// TestTalosClusterUnknownBundleLeavesEveryCAUnknown covers the first apply with
// a bundle from a resource that does not exist yet.
func TestTalosClusterUnknownBundleLeavesEveryCAUnknown(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrSecret)
	h.requireAttributes(t, caOutputAttributes()...)

	config := h.baseConfig(t, map[string]tftypes.Value{
		caAttrSecret: tftypes.NewValue(h.objType.AttributeTypes[caAttrSecret], tftypes.UnknownValue),
	})

	planned := h.plan(t, tftypes.NewValue(h.objType, nil), config)

	for _, attribute := range caOutputAttributes() {
		if v := caAttribute(t, planned, attribute); v.IsKnown() {
			t.Errorf("planned %s = %s, want unknown while the bundle is unknown", attribute, v)
		}
	}
}

// TestTalosClusterExposesNoKey: only the certificates are exposed. The state of
// an apply holds no private key of the bundle.
func TestTalosClusterExposesNoKey(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO)
	h.requireAttributes(t, caOutputAttributes()...)

	overrides := map[string]tftypes.Value{}
	keys := map[string]string{}

	for _, o := range caOutputs {
		overrides["certs."+o.bundle+".cert"] = caStringValue(caCertificate(t, o.attribute))

		key := "secret-key-of-" + o.bundle
		overrides["certs."+o.bundle+".key"] = caStringValue(key)
		keys[o.bundle] = key
	}

	prior := h.priorState(t, nil)
	config := h.baseConfig(t, map[string]tftypes.Value{caAttrWO: h.bundle(t, caAttrWO, overrides)})
	planned := h.plan(t, prior, config)
	newState := h.apply(t, prior, planned, config)

	for name, state := range map[string]tftypes.Value{"planned": planned, "applied": newState} {
		var attrs map[string]tftypes.Value
		if err := state.As(&attrs); err != nil {
			t.Fatal(err)
		}

		for attribute, v := range attrs {
			if v.IsNull() || !v.IsKnown() || !v.Type().Is(tftypes.String) {
				continue
			}

			var s string
			if err := v.As(&s); err != nil {
				t.Fatal(err)
			}

			for bundle, key := range keys {
				if s == key {
					t.Errorf("%s state: %s holds the key of certs.%s", name, attribute, bundle)
				}
			}
		}
	}
}

// TestTalosClusterEveryBrokenCertificateIsReported: one broken certificate must
// not hide the ones after it.
func TestTalosClusterEveryBrokenCertificateIsReported(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO)

	config := h.baseConfig(t, map[string]tftypes.Value{
		caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{
			"certs.k8s.cert":  caStringValue("not a certificate"),
			"certs.etcd.cert": caStringValue("not a certificate either"),
		}),
	})

	diags := h.validate(t, config)

	for _, summary := range []string{"Invalid Kubernetes CA certificate", "Invalid etcd CA certificate"} {
		if !caHasError(diags, summary) {
			t.Errorf("ValidateResourceConfig = %v, want a %q error", diags, summary)
		}
	}
}

// caNestedType returns the type of an attribute nested in a machine secrets input.
func caNestedType(t *testing.T, h *caHarness, attribute string, names ...string) tftypes.Type {
	t.Helper()

	typ := h.objType.AttributeTypes[attribute]

	for _, name := range names {
		obj, ok := typ.(tftypes.Object)
		if !ok {
			t.Fatalf("%s is not an object", name)
		}

		typ = obj.AttributeTypes[name]
	}

	return typ
}

// TestTalosClusterPartlyUnknownBundleLeavesTheAffectedCAsUnknown: the bundle is
// known but one of its objects is not yet, for example certs from a resource that
// does not exist yet. The certificates under it must be unknown, not null: a
// nested value under an unknown object reads back as null.
func TestTalosClusterPartlyUnknownBundleLeavesTheAffectedCAsUnknown(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		unknownCAs  map[string]bool
		unknownPath string
	}{
		"certs unknown": {
			unknownPath: "certs",
			unknownCAs:  map[string]bool{"kubernetes_ca_certificate": true, "talos_ca_certificate": true, "etcd_ca_certificate": true, "kubernetes_aggregator_ca_certificate": true},
		},
		"certs.etcd unknown": {
			unknownPath: "certs.etcd",
			unknownCAs:  map[string]bool{"etcd_ca_certificate": true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newCAHarness(t)
			h.requireAttributes(t, caAttrWO)
			h.requireAttributes(t, caOutputAttributes()...)

			names := []string{"certs"}
			if tc.unknownPath == "certs.etcd" {
				names = append(names, "etcd")
			}

			config := h.baseConfig(t, map[string]tftypes.Value{
				caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{
					tc.unknownPath: tftypes.NewValue(caNestedType(t, h, caAttrWO, names...), tftypes.UnknownValue),
				}),
			})

			caFatalOnErrors(t, "ValidateResourceConfig", h.validate(t, config))

			planned := h.plan(t, tftypes.NewValue(h.objType, nil), config)

			for _, o := range caOutputs {
				v := caAttribute(t, planned, o.attribute)

				if tc.unknownCAs[o.attribute] && v.IsKnown() {
					t.Errorf("planned %s = %s, want unknown while %s is unknown", o.attribute, v, tc.unknownPath)
				}

				if !tc.unknownCAs[o.attribute] && !v.IsKnown() {
					t.Errorf("planned %s is unknown, want a value: %s is known", o.attribute, o.bundle)
				}
			}
		})
	}
}

// TestTalosClusterConflictingInputsAreCaughtAtApply: with one input unknown at
// plan, validation cannot tell they conflict. Apply must not silently pick one.
func TestTalosClusterConflictingInputsAreCaughtAtApply(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO, caAttrSecret)

	prior := h.priorState(t, nil)

	planConfig := h.baseConfig(t, map[string]tftypes.Value{
		caAttrSecret: tftypes.NewValue(h.objType.AttributeTypes[caAttrSecret], tftypes.UnknownValue),
		caAttrWO:     h.bundle(t, caAttrWO, nil),
	})
	planned := h.plan(t, prior, planConfig)

	applyConfig := h.baseConfig(t, map[string]tftypes.Value{
		caAttrSecret: h.bundle(t, caAttrSecret, nil),
		caAttrWO:     h.bundle(t, caAttrWO, nil),
	})

	resp, err := h.server.ApplyResourceChange(context.Background(), &tfprotov6.ApplyResourceChangeRequest{
		TypeName:     "talos_cluster",
		PriorState:   h.dynamic(t, prior),
		PlannedState: h.dynamic(t, planned),
		Config:       h.dynamic(t, applyConfig),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !caHasError(resp.Diagnostics, "Conflicting machine secrets") {
		t.Errorf("ApplyResourceChange diagnostics = %v, want a \"Conflicting machine secrets\" error", resp.Diagnostics)
	}
}

// caLeafCertificate returns a certificate that is not a CA, in the format of the
// bundle.
func caLeafCertificate(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	return base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// TestTalosClusterRejectsACertificateThatIsNotACA: the attributes are CA
// certificates, and a consumer such as Vault trusts them as such.
func TestTalosClusterRejectsACertificateThatIsNotACA(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO)

	config := h.baseConfig(t, map[string]tftypes.Value{
		caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{caK8sCert: caStringValue(caLeafCertificate(t))}),
	})

	diags := h.validate(t, config)

	if !caHasError(diags, "Invalid Kubernetes CA certificate") {
		t.Errorf("ValidateResourceConfig of a leaf certificate = %v, want an \"Invalid Kubernetes CA certificate\" error", diags)
	}
}

// caChain joins the PEM of several certificates, in the format of the bundle.
func caChain(t *testing.T, certs ...string) string {
	t.Helper()

	var pemBytes []byte

	for _, cert := range certs {
		decoded, err := base64.StdEncoding.DecodeString(cert)
		if err != nil {
			t.Fatal(err)
		}

		pemBytes = append(pemBytes, decoded...)
	}

	return base64.StdEncoding.EncodeToString(pemBytes)
}

// TestTalosClusterChecksEveryCertificateOfAChain: a field can hold several
// certificates, for example the old and the new CA during a rotation. All of
// them are exposed, so all of them must be valid.
func TestTalosClusterChecksEveryCertificateOfAChain(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		cert    func(t *testing.T) string
		wantErr bool
	}{
		"two CA certificates": {
			cert: func(t *testing.T) string { return caChain(t, caCertificate(t, "old"), caCertificate(t, "new")) },
		},
		"a leaf certificate after a CA": {
			cert:    func(t *testing.T) string { return caChain(t, caCertificate(t, "ca"), caLeafCertificate(t)) },
			wantErr: true,
		},
		"junk after the certificate": {
			cert: func(t *testing.T) string {
				return base64.StdEncoding.EncodeToString(append(mustDecodeBase64(t, caCertificate(t, "ca")), []byte("junk")...))
			},
			wantErr: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			h := newCAHarness(t)
			h.requireAttributes(t, caAttrWO)

			config := h.baseConfig(t, map[string]tftypes.Value{
				caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{caK8sCert: caStringValue(tc.cert(t))}),
			})

			diags := h.validate(t, config)

			if got := caHasError(diags, "Invalid Kubernetes CA certificate"); got != tc.wantErr {
				t.Errorf("ValidateResourceConfig = %v, want an error: %v", diags, tc.wantErr)
			}
		})
	}
}

func mustDecodeBase64(t *testing.T, s string) []byte {
	t.Helper()

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

// TestTalosClusterCreateRejectsABundleBeforeAnySideEffect: a bundle that was
// unknown at plan time and turns out invalid must fail before the cluster is
// bootstrapped, not after minutes of waiting for it. There is no cluster to talk
// to here, so a create that got as far as bootstrapping would run into the
// deadline and report something else.
func TestTalosClusterCreateRejectsABundleBeforeAnySideEffect(t *testing.T) {
	t.Parallel()

	h := newCAHarness(t)
	h.requireAttributes(t, caAttrWO)

	planConfig := h.baseConfig(t, map[string]tftypes.Value{
		caAttrWO: tftypes.NewValue(h.objType.AttributeTypes[caAttrWO], tftypes.UnknownValue),
	})
	planned := h.plan(t, tftypes.NewValue(h.objType, nil), planConfig)

	applyConfig := h.baseConfig(t, map[string]tftypes.Value{
		caAttrWO: h.bundle(t, caAttrWO, map[string]tftypes.Value{caK8sCert: caStringValue("not a certificate")}),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := h.server.ApplyResourceChange(ctx, &tfprotov6.ApplyResourceChangeRequest{
		TypeName:     "talos_cluster",
		PriorState:   h.dynamic(t, tftypes.NewValue(h.objType, nil)),
		PlannedState: h.dynamic(t, planned),
		Config:       h.dynamic(t, applyConfig),
	})
	if err != nil {
		t.Fatal(err)
	}

	if !caHasError(resp.Diagnostics, "Invalid Kubernetes CA certificate") {
		t.Errorf("ApplyResourceChange diagnostics = %v, want an \"Invalid Kubernetes CA certificate\" error", resp.Diagnostics)
	}
}
