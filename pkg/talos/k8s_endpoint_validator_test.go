// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // exercises the unexported parseK8sEndpoint and k8sEndpointValidator

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestParseK8sEndpoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{"IPv4 with port and slash", "https://10.0.0.1:6443/", "10.0.0.1:6443", false},
		{"IPv4 with port", "https://10.0.0.1:6443", "10.0.0.1:6443", false},
		{"hostname with port", "https://k8s.example.com:443/", "k8s.example.com:443", false},
		{"hostname without port", "https://k8s.example.com/", "k8s.example.com:6443", false},
		{"IPv6 with port", "https://[2001:db8::1]:6443/", "[2001:db8::1]:6443", false},
		{"http scheme", "http://10.0.0.1:6443/", "", true},
		{"no scheme", "10.0.0.1:6443", "", true},
		{"path", "https://10.0.0.1:6443/api", "", true},
		{"query", "https://10.0.0.1:6443/?a=b", "", true},
		{"userinfo", "https://user@10.0.0.1:6443/", "", true},
		{"missing host", "https:///", "", true},
		{"empty", "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseK8sEndpoint(tc.value)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseK8sEndpoint(%q) error = %v, wantErr %v", tc.value, err, tc.wantErr)
			}

			if got != tc.want {
				t.Errorf("parseK8sEndpoint(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestK8sEndpointValidator(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"valid", types.StringValue("https://10.0.0.1:6443/"), false},
		{"invalid", types.StringValue("10.0.0.1:6443"), true},
		{"null", types.StringNull(), false},
		{"unknown", types.StringUnknown(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := &validator.StringResponse{}
			k8sEndpointValid().ValidateString(
				context.Background(),
				validator.StringRequest{
					Path:        path.Root("k8s_endpoint"),
					ConfigValue: tc.value,
				},
				resp,
			)

			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}
