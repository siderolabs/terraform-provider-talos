// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cosi-project/runtime/pkg/safe"
	cosistate "github.com/cosi-project/runtime/pkg/state"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	runtimeres "github.com/siderolabs/talos/pkg/machinery/resources/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// tryApplyTiming bounds a try-mode apply. Talos rolls the configuration back
// RollbackAfter after accepting it unless the provider confirms it first. The
// provider confirms no earlier than CommitAfter, so a change that cuts the node
// off has time to do so, and starts its last confirmation early enough to reach
// the node before the rollback.
type tryApplyTiming struct {
	RollbackAfter  time.Duration
	CommitAfter    time.Duration
	AttemptTimeout time.Duration
	RetryInterval  time.Duration
}

var defaultTryApplyTiming = tryApplyTiming{
	RollbackAfter:  time.Minute,
	CommitAfter:    10 * time.Second,
	AttemptTimeout: 10 * time.Second,
	RetryInterval:  2 * time.Second,
}

// errTryRejected means Talos refused a try-mode request without applying it.
var errTryRejected = errors.New("try mode rejected")

// talosMachineSendConfig sends the machine configuration and reports whether
// Talos reboots the node to apply it. With tryMode, a running node gets the
// configuration in try mode and keeps it once talosMachineConfirmTry confirmed it.
func talosMachineSendConfig(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, cfgBytes []byte,
	op clientOpFunc, tryMode bool, timing tryApplyTiming, diags *diag.Diagnostics,
) (bool, error) {
	if !tryMode {
		return talosMachineSendAuto(ctx, endpoint, node, talosConfig, cfgBytes, op)
	}

	var running bool

	if err := talosMachineRetryNodeOp(ctx, endpoint, node, talosConfig, op, func(nodeCtx context.Context, c *client.Client) error {
		var err error

		running, err = talosMachineRunning(nodeCtx, c)

		return err
	}); err != nil {
		return false, fmt.Errorf("checking whether the node is running: %w", err)
	}

	if !running {
		tflog.Info(ctx, "node is not running yet; applying configuration without try mode", map[string]any{"node": node})

		return talosMachineSendAuto(ctx, endpoint, node, talosConfig, cfgBytes, op)
	}

	sentAt, err := talosMachineSendTry(ctx, endpoint, node, talosConfig, cfgBytes, op, timing)
	if err == nil {
		return false, talosMachineConfirmTry(ctx, endpoint, node, talosConfig, cfgBytes, op, sentAt, timing)
	}

	if !errors.Is(err, errTryRejected) {
		return false, err
	}

	// Talos before 1.14 rejects try mode for changes that need a reboot. An
	// invalid configuration fails again without try mode and is reported then.
	rebooted, err := talosMachineSendAuto(ctx, endpoint, node, talosConfig, cfgBytes, op)
	if err == nil {
		diags.AddWarning(
			"Configuration applied without try mode",
			fmt.Sprintf("Talos on node %s rejected the try-mode apply, most likely because the change needs a reboot (Talos before 1.14). "+
				"The configuration was applied without try mode instead, without automatic rollback.", node),
		)
	}

	return rebooted, err
}

// talosMachineSendTry sends the configuration in try mode and returns when the
// successful request was sent. A request that may have reached the node is not
// sent again: Talos would then roll back to the first request's configuration.
func talosMachineSendTry(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, cfgBytes []byte,
	op clientOpFunc, timing tryApplyTiming,
) (time.Time, error) {
	var sentAt time.Time

	err := retry.RetryContext(ctx, 10*time.Minute, func() *retry.RetryError {
		sent := false

		err := op(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			sent, sentAt = true, time.Now()

			_, err := c.ApplyConfiguration(nodeCtx, &machineapi.ApplyConfigurationRequest{
				Mode:           machineapi.ApplyConfigurationRequest_TRY,
				Data:           cfgBytes,
				TryModeTimeout: durationpb.New(timing.RollbackAfter),
			})

			return err
		})

		switch code := status.Code(err); {
		case err == nil:
			return nil
		case !sent, code == codes.FailedPrecondition:
			return retry.RetryableError(err)
		case code == codes.InvalidArgument:
			return retry.NonRetryableError(fmt.Errorf("%w: %w", errTryRejected, err))
		default:
			return retry.NonRetryableError(fmt.Errorf(
				"try-mode apply may have reached the node, Talos rolls it back within %s if so: %w", timing.RollbackAfter, err))
		}
	})

	return sentAt, err
}

// talosMachineConfirmTry makes a try-mode configuration permanent by applying it
// again with NO_REBOOT: any mode but TRY cancels the pending rollback, and
// NO_REBOOT also persists the configuration, which TRY never does. It waits until
// timing.CommitAfter after sentAt and gives up early enough that a late
// confirmation cannot re-apply the configuration after Talos rolled it back.
func talosMachineConfirmTry(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, cfgBytes []byte,
	op clientOpFunc, sentAt time.Time, timing tryApplyTiming,
) error {
	// sentAt is taken before the try request left, so the rollback timer started
	// later. No attempt starts after lastStart, give or take timer jitter, which
	// the second AttemptTimeout of margin absorbs: every attempt ends before the
	// rollback.
	lastStart := sentAt.Add(timing.RollbackAfter - 2*timing.AttemptTimeout)

	if err := sleepContext(ctx, time.Until(sentAt.Add(timing.CommitAfter))); err != nil {
		return fmt.Errorf("waiting to confirm try-mode configuration: %w", err)
	}

	for {
		attemptCtx, cancel := context.WithTimeout(ctx, timing.AttemptTimeout)

		err := op(attemptCtx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			_, err := c.ApplyConfiguration(nodeCtx, &machineapi.ApplyConfigurationRequest{
				Mode: machineapi.ApplyConfigurationRequest_NO_REBOOT,
				Data: cfgBytes,
			})

			return err
		})

		cancel()

		if err == nil {
			return nil
		}

		if time.Now().Add(timing.RetryInterval).After(lastStart) {
			return fmt.Errorf("node did not stay reachable after the try-mode apply, Talos rolls the configuration back %s after it was sent: %w",
				timing.RollbackAfter, err)
		}

		if err := sleepContext(ctx, timing.RetryInterval); err != nil {
			return fmt.Errorf("confirming try-mode configuration: %w", err)
		}
	}
}

// talosMachineRunning reports whether the node has finished booting with a
// configuration. Try mode only makes sense there: in maintenance mode the first
// configuration installs Talos.
func talosMachineRunning(ctx context.Context, c *client.Client) (bool, error) {
	machine, err := safe.StateGet[*runtimeres.MachineStatus](ctx, c.COSI, runtimeres.NewMachineStatus().Metadata())
	if err == nil {
		return machine.TypedSpec().Stage == runtimeres.MachineStageRunning, nil
	}

	if cosistate.IsNotFoundError(err) {
		return false, nil
	}

	switch status.Code(err) { //nolint:exhaustive // every other code is a real failure
	case codes.NotFound, codes.PermissionDenied, codes.Unimplemented:
		// Not readable, as in maintenance mode or for a restricted role: apply
		// without try mode and let ApplyConfiguration report missing permissions.
		return false, nil
	default:
		return false, fmt.Errorf("reading machine status: %w", err)
	}
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
