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
	"math/big"
	"net"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// generateTestCertPEM returns a self-signed certificate and its matching key,
// PEM-encoded. Nothing in talosMachineWaitReachable's preflight check verifies
// a trust chain — it only needs a well-formed, matching cert/key pair.
func generateTestCertPEM(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}

	certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))

	return certPEM, keyPEM
}

// closedPortEndpoint allocates a local port and immediately closes it, so
// nothing will ever be listening and every dial attempt fails fast with a
// real "connection refused" — without needing a real Talos node or any mock
// of talosClientOp's network dial.
func closedPortEndpoint(t *testing.T) string {
	t.Helper()

	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate a local port: %v", err)
	}

	endpoint := ln.Addr().String()

	if err := ln.Close(); err != nil {
		t.Fatalf("failed to close listener: %v", err)
	}

	return endpoint
}

// buildMachineReadState builds a talos_machine prior-state value with just
// enough attributes populated for Read()'s live-refresh path: an endpoint, a
// client_configuration, and the handful of other attributes Read() touches.
func buildMachineReadState(t *testing.T, ctx context.Context, sch schema.Schema, endpoint, certPEM, keyPEM string) tftypes.Value {
	t.Helper()

	schTFType, ok := sch.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema TerraformType is not tftypes.Object")
	}

	nullVals := make(map[string]tftypes.Value, len(schTFType.AttributeTypes))
	for name, typ := range schTFType.AttributeTypes {
		nullVals[name] = tftypes.NewValue(typ, nil)
	}

	ccTFType, ok := schTFType.AttributeTypes["client_configuration"].(tftypes.Object)
	if !ok {
		t.Fatal("client_configuration is not tftypes.Object in schema")
	}

	b64 := base64.StdEncoding.EncodeToString

	ccVal := tftypes.NewValue(ccTFType, map[string]tftypes.Value{
		"ca_certificate":     tftypes.NewValue(tftypes.String, b64([]byte(certPEM))),
		"client_certificate": tftypes.NewValue(tftypes.String, b64([]byte(certPEM))),
		"client_key":         tftypes.NewValue(tftypes.String, b64([]byte(keyPEM))),
	})

	nullVals["id"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	nullVals["node"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	nullVals["endpoint"] = tftypes.NewValue(tftypes.String, endpoint)
	nullVals["client_configuration"] = ccVal
	nullVals["image"] = tftypes.NewValue(tftypes.String, "ghcr.io/siderolabs/installer:v1.12.0")
	nullVals["machine_configuration_hash"] = tftypes.NewValue(tftypes.String, "deadbeef")

	return tftypes.NewValue(tftypes.Object{AttributeTypes: schTFType.AttributeTypes}, nullVals)
}

// TestRead_TransientConnectionFailure_DoesNotRemoveResource guards against
// siderolabs/terraform-provider-talos#408: Read() used to treat any error
// from the liveness probe (a plain Version RPC) as proof the machine was
// gone, and unconditionally called RemoveResource. A closed local port is a
// genuine, unambiguous, transient connection failure — exactly the kind
// produced by a node mid-reboot (netboot, kernel upgrade, brief network
// partition) — not a positive signal that the machine was decommissioned.
// Before the fix, this test failed: Read() removed the resource anyway.
func TestRead_TransientConnectionFailure_DoesNotRemoveResource(t *testing.T) {
	t.Parallel()

	// Deliberately much shorter than talos.ReadLivenessTimeout: connection
	// refused is immediate, so this doesn't need to (and must not, for CI
	// runtime's sake) wait out the full production retry budget to observe
	// the same "still fails, state preserved" outcome.
	const testDeadline = 3 * time.Second
	if testDeadline >= talos.ReadLivenessTimeout {
		t.Fatalf("test deadline (%s) must stay below talos.ReadLivenessTimeout (%s)", testDeadline, talos.ReadLivenessTimeout)
	}

	ctx, cancel := context.WithTimeout(context.Background(), testDeadline)
	defer cancel()

	endpoint := closedPortEndpoint(t)
	certPEM, keyPEM := generateTestCertPEM(t)

	r := talos.NewTalosMachineResource()

	var schemaResp frameworkresource.SchemaResponse

	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)

	stateRaw := buildMachineReadState(t, ctx, schemaResp.Schema, endpoint, certPEM, keyPEM)

	stateObj := tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw}

	req := frameworkresource.ReadRequest{State: stateObj}
	resp := frameworkresource.ReadResponse{State: stateObj}

	r.Read(ctx, req, &resp)

	if resp.State.Raw.IsNull() {
		t.Fatal("Read() removed the resource from state after a transient connection failure " +
			"(closed port) that is indistinguishable from a genuinely decommissioned machine. " +
			"The next terraform plan will show a phantom \"will be created\", and applying it can " +
			"re-run Create() (config re-apply, and — when image is set — a version/schematic check " +
			"that may trigger a real upgrade/drain/reboot cycle on a perfectly healthy node. " +
			"See siderolabs/terraform-provider-talos#408.")
	}

	if resp.Diagnostics.HasError() {
		t.Fatalf("expected no error diagnostics, got: %v", resp.Diagnostics)
	}

	found := false

	for _, d := range resp.Diagnostics.Warnings() {
		if d.Summary() == "could not refresh talos_machine" {
			found = true

			break
		}
	}

	if !found {
		t.Fatalf("expected a warning diagnostic explaining the node could not be reached, got: %v", resp.Diagnostics)
	}
}

// TestRead_InvalidClientCredentials_FailsFastWithoutRemovingResource covers the
// gap the transient-failure fix would otherwise leave: a client_configuration
// that cannot be parsed into a certificate (e.g. state manually edited or
// partially written) can never succeed no matter how long Read() retries.
// It must fail immediately with a clear diagnostic — not spend the whole
// ReadLivenessTimeout window retrying a guaranteed-fail dial and then report
// it as if the node were merely unreachable — and it must not remove the
// resource either, since invalid local credentials say nothing about whether
// the remote machine still exists.
func TestRead_InvalidClientCredentials_FailsFastWithoutRemovingResource(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), talos.ReadLivenessTimeout)
	defer cancel()

	endpoint := closedPortEndpoint(t)

	r := talos.NewTalosMachineResource()

	var schemaResp frameworkresource.SchemaResponse

	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)

	// Not a PEM-encoded certificate or key at all.
	stateRaw := buildMachineReadState(t, ctx, schemaResp.Schema, endpoint, "not a cert", "not a key")

	stateObj := tfsdk.State{Schema: schemaResp.Schema, Raw: stateRaw}

	req := frameworkresource.ReadRequest{State: stateObj}
	resp := frameworkresource.ReadResponse{State: stateObj}

	start := time.Now()

	r.Read(ctx, req, &resp)

	elapsed := time.Since(start)

	const failFastBudget = 5 * time.Second
	if elapsed >= failFastBudget {
		t.Fatalf("Read() took %s to reject invalid credentials; expected it to fail immediately "+
			"without entering the retry loop (budget: %s)", elapsed, failFastBudget)
	}

	if resp.State.Raw.IsNull() {
		t.Fatal("Read() removed the resource from state after an invalid client_configuration; " +
			"invalid local credentials are not a positive signal the remote machine is gone")
	}

	found := false

	for _, d := range resp.Diagnostics.Errors() {
		if d.Summary() == "invalid talos_machine client credentials" {
			found = true

			break
		}
	}

	if !found {
		t.Fatalf("expected an error diagnostic about invalid client credentials, got: %v", resp.Diagnostics)
	}
}
