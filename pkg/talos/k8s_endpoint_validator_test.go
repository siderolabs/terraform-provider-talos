// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // exercises the unexported parseK8sEndpoint and k8sEndpointValidator

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestParseK8sEndpoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		value   string
		want    string
		wantErr string
	}{
		// naked ips
		{value: "10.5.0.2", want: "10.5.0.2:6443"},
		{value: "2001:db8::1", want: "[2001:db8::1]:6443"},
		{value: "fe80::1%eth0", want: "[fe80::1%25eth0]:6443"},
		{value: "10.5.0.2:7443", want: "10.5.0.2:7443"},
		// default port added
		{value: "[2001:db8::1]", want: "[2001:db8::1]:6443"},
		{value: "https://myhost", want: "myhost:6443"},
		{value: "myhost/", want: "myhost:6443"},
		// valid
		{value: "https://myhost1:6443", want: "myhost1:6443"},
		{value: "https://myhost2:6443/", want: "myhost2:6443"},
		{value: "myhost:7443", want: "myhost:7443"},
		{value: "https://[2001:db8::1]:7443/", want: "[2001:db8::1]:7443"},
		{value: "https://[fe80::1%25eth0]:7443/", want: "[fe80::1%25eth0]:7443"},
		// invalid
		{value: "http://myhost:6443", wantErr: "only https"},
		{value: "http://myhost", wantErr: "only https"},
		{value: "my host", wantErr: "invalid character"},
		{value: "", wantErr: "host is empty"},
		{value: "https://", wantErr: "host is empty"},
		{value: "/", wantErr: "host is empty"},
		{value: ":6443", wantErr: "host is empty"},
		{value: "https://myhost:", wantErr: "port is empty"},
		{value: "https://myhost:abc", wantErr: "invalid port"},
		{value: "https://myhost:0", wantErr: "invalid port"},
		{value: "https://myhost:65536", wantErr: "invalid port"},
		{value: "https://2001:db8::1", wantErr: "invalid Kubernetes endpoint"},
		{value: "https://myhost:6443//", wantErr: "not allowed"},
		{value: "https://myhost:6443/api", wantErr: "not allowed"},
		{value: "https://user@myhost:6443/", wantErr: "not allowed"},
		{value: "https://myhost:6443/?test=123", wantErr: "not allowed"},
		{value: "https://myhost:6443/#frag123", wantErr: "not allowed"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Parallel()

			got, err := parseK8sEndpoint(tc.value)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseK8sEndpoint(%q) error = %v, want it to contain %q", tc.value, err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseK8sEndpoint(%q) unexpected error: %v", tc.value, err)
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
		{"invalid", types.StringValue("http://10.0.0.1:6443/"), true},
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
