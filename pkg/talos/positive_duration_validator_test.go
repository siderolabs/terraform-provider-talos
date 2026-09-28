// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // exercises the unexported positiveDurationValidator

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestPositiveDurationValidator is the red-phase test for a code-review finding on
// talos_machine's drain_timeout: goDurationValid only checks that the string parses,
// so "-1m" sails through validation and then feeds a context with an already-past
// deadline into nodedrain, and "0s" is indistinguishable from an empty/unparsed value
// and silently falls back to nodedrain's hardcoded 5m default with no error. Both need
// rejecting at plan time with a clear message, not left to fail in confusing ways later.
func TestPositiveDurationValidator(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"positive duration", types.StringValue("5m"), false},
		{"zero seconds", types.StringValue("0s"), true},
		{"zero", types.StringValue("0"), true},
		{"negative duration", types.StringValue("-1m"), true},
		// unparseable strings are goDurationValid's job, not this validator's; it must
		// not pile a second, misleading diagnostic onto an already-invalid value.
		{"unparseable", types.StringValue("not-a-duration"), false},
		// null/unknown are the caller's business, not the validator's
		{"null", types.StringNull(), false},
		{"unknown", types.StringUnknown(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := &validator.StringResponse{}
			positiveDurationValid().ValidateString(
				context.Background(),
				validator.StringRequest{
					Path:        path.Root("drain_timeout"),
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
