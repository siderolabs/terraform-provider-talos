// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // exercises the unexported supportedDiskImageFormats

import (
	"slices"
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/platforms"
)

// TestSupportedDiskImageFormatsCoversPlatformDefaults checks that every platform default
// appears in the hand-maintained list. A missing one would be rejected at plan time even
// though the factory serves it. Formats that no platform uses as a default are not covered.
func TestSupportedDiskImageFormatsCoversPlatformDefaults(t *testing.T) {
	t.Parallel()

	formats := supportedDiskImageFormats()

	for _, platform := range slices.Concat([]platforms.Platform{platforms.MetalPlatform()}, platforms.CloudPlatforms()) {
		if platform.DiskImageSuffix == "" {
			continue
		}

		if !slices.Contains(formats, platform.DiskImageSuffix) {
			t.Errorf("platform %q declares disk image suffix %q, which supportedDiskImageFormats does not accept", platform.Name, platform.DiskImageSuffix)
		}
	}
}
