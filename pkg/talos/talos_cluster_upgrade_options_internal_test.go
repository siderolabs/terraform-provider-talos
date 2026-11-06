// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/siderolabs/talos/pkg/machinery/constants"
)

// TestTalosClusterUpgradeOptionsDefaults guards existing state: with the new
// attributes unset, upgrade-k8s must run exactly as before (Talos default image
// repositories, pre-pull enabled).
func TestTalosClusterUpgradeOptionsDefaults(t *testing.T) {
	t.Parallel()

	opts := talosClusterUpgradeOptions("10.0.0.1", &talosClusterResourceModel{
		KubeletImage:           types.StringNull(),
		APIServerImage:         types.StringNull(),
		ControllerManagerImage: types.StringNull(),
		SchedulerImage:         types.StringNull(),
		ProxyImage:             types.StringNull(),
		PrePullImages:          types.BoolNull(),
	})

	for name, tc := range map[string]struct{ got, want string }{
		"kubelet":            {opts.KubeletImage, constants.KubeletImage},
		"apiserver":          {opts.APIServerImage, constants.KubernetesAPIServerImage},
		"controller-manager": {opts.ControllerManagerImage, constants.KubernetesControllerManagerImage},
		"scheduler":          {opts.SchedulerImage, constants.KubernetesSchedulerImage},
		"proxy":              {opts.ProxyImage, constants.KubeProxyImage},
	} {
		if tc.got != tc.want {
			t.Errorf("%s image: got %q, want %q", name, tc.got, tc.want)
		}
	}

	if !opts.PrePullImages {
		t.Error("PrePullImages: got false, want true")
	}

	if opts.ControlPlaneEndpoint != "10.0.0.1" {
		t.Errorf("ControlPlaneEndpoint: got %q", opts.ControlPlaneEndpoint)
	}
}

// TestTalosClusterUpgradeOptionsOverrides is the red-phase test for the
// private-registry case: upgrade-k8s used to hardcode registry.k8s.io and
// ghcr.io repositories and pre-pull, rewriting custom repositories in the
// machine config and failing on nodes without access to public registries.
func TestTalosClusterUpgradeOptionsOverrides(t *testing.T) {
	t.Parallel()

	opts := talosClusterUpgradeOptions("10.0.0.1", &talosClusterResourceModel{
		KubeletImage:           types.StringValue("registry.example.com/siderolabs/kubelet"),
		APIServerImage:         types.StringValue("registry.example.com/kube-apiserver"),
		ControllerManagerImage: types.StringValue("registry.example.com/kube-controller-manager"),
		SchedulerImage:         types.StringValue("registry.example.com/kube-scheduler"),
		ProxyImage:             types.StringValue("registry.example.com:5000/kube-proxy"),
		PrePullImages:          types.BoolValue(false),
	})

	for name, tc := range map[string]struct{ got, want string }{
		"kubelet":            {opts.KubeletImage, "registry.example.com/siderolabs/kubelet"},
		"apiserver":          {opts.APIServerImage, "registry.example.com/kube-apiserver"},
		"controller-manager": {opts.ControllerManagerImage, "registry.example.com/kube-controller-manager"},
		"scheduler":          {opts.SchedulerImage, "registry.example.com/kube-scheduler"},
		"proxy":              {opts.ProxyImage, "registry.example.com:5000/kube-proxy"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s image: got %q, want %q", name, tc.got, tc.want)
		}
	}

	if opts.PrePullImages {
		t.Error("PrePullImages: got true, want false")
	}
}

func TestImageRepositoryValidator(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"repository", types.StringValue("registry.example.com/kube-apiserver"), false},
		{"repository with registry port", types.StringValue("registry.example.com:5000/kube-apiserver"), false},
		{"tag", types.StringValue("registry.example.com/kube-apiserver:v1.35.0"), true},
		{"tag with registry port", types.StringValue("registry.example.com:5000/kube-apiserver:v1.35.0"), true},
		{"digest", types.StringValue("registry.example.com/kube-apiserver@sha256:0123"), true},
		{"empty", types.StringValue(""), true},
		{"null", types.StringNull(), false},
		{"unknown", types.StringUnknown(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := &validator.StringResponse{}
			imageRepositoryValid().ValidateString(context.Background(), validator.StringRequest{
				Path:        path.Root("apiserver_image"),
				ConfigValue: tc.value,
			}, resp)

			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v: %v", got, tc.wantErr, resp.Diagnostics)
			}
		})
	}
}
