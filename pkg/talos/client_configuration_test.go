// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos // nolint:testpackage // needs access to internal functions

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// TestGetClientConfigurationValues_Typed tests extraction from a properly-typed ObjectValue
// (created via the clientConfiguration struct).
func TestGetClientConfigurationValues_Typed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create a properly-typed ObjectValue using the clientConfiguration struct
	config := clientConfiguration{
		CA:   types.StringValue("test-ca-cert"),
		Cert: types.StringValue("test-client-cert"),
		Key:  types.StringValue("test-client-key"),
	}

	objValue, diags := types.ObjectValueFrom(ctx, map[string]attr.Type{
		"ca_certificate":     types.StringType,
		"client_certificate": types.StringType,
		"client_key":         types.StringType,
	}, config)

	if diags.HasError() {
		t.Fatalf("Failed to create ObjectValue: %v", diags)
	}

	// Test extraction
	ca, cert, key, errMsg, ok := getClientConfigurationValues(ctx, objValue)

	if !ok {
		t.Fatalf("Expected extraction to succeed, got error: %s", errMsg)
	}

	if ca != "test-ca-cert" {
		t.Errorf("Expected ca='test-ca-cert', got '%s'", ca)
	}

	if cert != "test-client-cert" {
		t.Errorf("Expected cert='test-client-cert', got '%s'", cert)
	}

	if key != "test-client-key" {
		t.Errorf("Expected key='test-client-key', got '%s'", key)
	}
}

// TestGetClientConfigurationValues_Reconstructed tests extraction from a reconstructed ObjectValue
// (plain map with correct keys, as returned from Vault round-trip without full schema metadata).
// This test demonstrates the issue where .As() fails on reconstructed maps.
func TestGetClientConfigurationValues_Reconstructed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Create an ObjectValue from a plain map (simulating Vault reconstruction)
	// This is what happens when you do:
	//   client_configuration = {
	//     ca_certificate     = vault_data["ca"]
	//     client_certificate = vault_data["cert"]
	//     client_key         = vault_data["key"]
	//   }
	objValue, diags := types.ObjectValue(
		map[string]attr.Type{
			"ca_certificate":     types.StringType,
			"client_certificate": types.StringType,
			"client_key":         types.StringType,
		},
		map[string]attr.Value{
			"ca_certificate":     basetypes.NewStringValue("vault-ca-cert"),
			"client_certificate": basetypes.NewStringValue("vault-client-cert"),
			"client_key":         basetypes.NewStringValue("vault-client-key"),
		},
	)

	if diags.HasError() {
		t.Fatalf("Failed to create ObjectValue: %v", diags)
	}

	// Test extraction using the fallback path that handles plain maps (e.g., reconstructed from Vault)
	ca, cert, key, errMsg, ok := getClientConfigurationValues(ctx, objValue)

	if !ok {
		t.Fatalf("Expected extraction to succeed, got error: %s", errMsg)
	}

	if ca != "vault-ca-cert" {
		t.Errorf("Expected ca='vault-ca-cert', got '%s'", ca)
	}

	if cert != "vault-client-cert" {
		t.Errorf("Expected cert='vault-client-cert', got '%s'", cert)
	}

	if key != "vault-client-key" {
		t.Errorf("Expected key='vault-client-key', got '%s'", key)
	}
}

// TestGetClientConfigurationValues_Null tests that null values are handled correctly.
func TestGetClientConfigurationValues_Null(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	objValue := basetypes.NewObjectNull(map[string]attr.Type{
		"ca_certificate":     types.StringType,
		"client_certificate": types.StringType,
		"client_key":         types.StringType,
	})

	ca, cert, key, errMsg, ok := getClientConfigurationValues(ctx, objValue)

	if ok {
		t.Fatal("Expected extraction to fail for null value")
	}

	if errMsg == "" {
		t.Error("Expected error message for null value")
	}

	if ca != "" || cert != "" || key != "" {
		t.Error("Expected empty strings for null value")
	}
}

// TestGetClientConfigurationValues_Unknown tests that unknown values are handled correctly.
func TestGetClientConfigurationValues_Unknown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	objValue := basetypes.NewObjectUnknown(map[string]attr.Type{
		"ca_certificate":     types.StringType,
		"client_certificate": types.StringType,
		"client_key":         types.StringType,
	})

	ca, cert, key, errMsg, ok := getClientConfigurationValues(ctx, objValue)

	if ok {
		t.Fatal("Expected extraction to fail for unknown value")
	}

	if errMsg == "" {
		t.Error("Expected error message for unknown value")
	}

	if ca != "" || cert != "" || key != "" {
		t.Error("Expected empty strings for unknown value")
	}
}
