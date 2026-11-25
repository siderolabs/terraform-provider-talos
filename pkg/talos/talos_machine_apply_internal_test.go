// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/state"
	"github.com/cosi-project/runtime/pkg/state/impl/inmem"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	runtimeres "github.com/siderolabs/talos/pkg/machinery/resources/runtime"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

type applyMode = machineapi.ApplyConfigurationRequest_Mode

const (
	tryAuto     = machineapi.ApplyConfigurationRequest_AUTO
	tryTry      = machineapi.ApplyConfigurationRequest_TRY
	tryNoReboot = machineapi.ApplyConfigurationRequest_NO_REBOOT
	tryReboot   = machineapi.ApplyConfigurationRequest_REBOOT //nolint:staticcheck // Talos before 1.14 still reports it
)

// tryTestTiming confirms after 50ms and starts the last confirmation by
// RollbackAfter - 2*AttemptTimeout = 300ms.
var tryTestTiming = talos.TryApplyTiming{
	RollbackAfter:  400 * time.Millisecond,
	CommitAfter:    50 * time.Millisecond,
	AttemptTimeout: 50 * time.Millisecond,
	RetryInterval:  10 * time.Millisecond,
}

type tryTestCall struct {
	at         time.Time
	mode       applyMode
	tryTimeout time.Duration
}

// tryTestMachine fakes ApplyConfiguration. Talos answers with the requested
// mode, AUTO as NO_REBOOT; reply can change that answer or fail call n (0-based).
type tryTestMachine struct {
	machineapi.MachineServiceClient

	reply func(n int, mode applyMode) (applyMode, error)
	calls []tryTestCall
}

func (m *tryTestMachine) ApplyConfiguration(_ context.Context, req *machineapi.ApplyConfigurationRequest, _ ...grpc.CallOption) (*machineapi.ApplyConfigurationResponse, error) {
	n := len(m.calls)
	m.calls = append(m.calls, tryTestCall{at: time.Now(), mode: req.GetMode(), tryTimeout: req.GetTryModeTimeout().AsDuration()})

	mode := req.GetMode()
	if mode == tryAuto {
		mode = tryNoReboot
	}

	if m.reply != nil {
		var err error

		if mode, err = m.reply(n, mode); err != nil {
			return nil, err
		}
	}

	return &machineapi.ApplyConfigurationResponse{Messages: []*machineapi.ApplyConfiguration{{Mode: mode}}}, nil
}

func (m *tryTestMachine) modes() []applyMode {
	modes := make([]applyMode, 0, len(m.calls))
	for _, c := range m.calls {
		modes = append(modes, c.mode)
	}

	return modes
}

// failFirst fails the first k calls with err and answers the rest normally.
func failFirst(k int, err error) func(int, applyMode) (applyMode, error) {
	return func(n int, mode applyMode) (applyMode, error) {
		if n < k {
			return 0, err
		}

		return mode, nil
	}
}

// tryTestOp returns an op for a node in stage with the fake machine API.
func tryTestOp(t *testing.T, stage runtimeres.MachineStage, m *tryTestMachine) talos.ClientOpFunc {
	t.Helper()

	st := state.WrapCore(inmem.NewState(runtimeres.NamespaceName))
	machine := runtimeres.NewMachineStatus()
	machine.TypedSpec().Stage = stage
	require.NoError(t, st.Create(t.Context(), machine))

	return cpTestOp(&client.Client{COSI: st, MachineClient: m})
}

func trySend(ctx context.Context, op talos.ClientOpFunc, tryMode bool, diags *diag.Diagnostics) (bool, error) {
	return talos.TalosMachineSendConfig(ctx, "endpoint", cpTestNode, &clientconfig.Config{}, []byte("config"), op, tryMode, tryTestTiming, diags)
}

func TestTalosMachineSendConfigModes(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		want    []applyMode
		stage   runtimeres.MachineStage
		tryMode bool
	}{
		{name: "try mode off", stage: runtimeres.MachineStageRunning, want: []applyMode{tryAuto}},
		{name: "running node: try, then confirm", stage: runtimeres.MachineStageRunning, tryMode: true, want: []applyMode{tryTry, tryNoReboot}},
		{name: "maintenance mode", stage: runtimeres.MachineStageMaintenance, tryMode: true, want: []applyMode{tryAuto}},
		{name: "still booting", stage: runtimeres.MachineStageBooting, tryMode: true, want: []applyMode{tryAuto}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := &tryTestMachine{}

			var diags diag.Diagnostics

			rebooted, err := trySend(t.Context(), tryTestOp(t, tt.stage, m), tt.tryMode, &diags)
			require.NoError(t, err)
			require.False(t, rebooted)
			require.Equal(t, tt.want, m.modes())
			require.Empty(t, diags)

			if tt.want[0] == tryTry {
				require.Equal(t, tryTestTiming.RollbackAfter, m.calls[0].tryTimeout)
				require.GreaterOrEqual(t, m.calls[1].at.Sub(m.calls[0].at), tryTestTiming.CommitAfter, "confirmed before the node had time to drop off")
			}
		})
	}
}

func TestTalosMachineSendConfigReportsReboot(t *testing.T) {
	t.Parallel()

	m := &tryTestMachine{reply: func(int, applyMode) (applyMode, error) { return tryReboot, nil }}

	var diags diag.Diagnostics

	rebooted, err := trySend(t.Context(), tryTestOp(t, runtimeres.MachineStageRunning, m), false, &diags)
	require.NoError(t, err)
	require.True(t, rebooted)
}

func TestTalosMachineSendConfigUnreadableMachineStatus(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		err  error
		name string
	}{
		{name: "permission denied", err: status.Error(codes.PermissionDenied, "denied")},
		{name: "unimplemented", err: status.Error(codes.Unimplemented, "maintenance")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := &tryTestMachine{}
			st := state.WrapCore(inmem.NewState(runtimeres.NamespaceName))
			op := cpTestOp(&client.Client{COSI: bootTestDeniedState{State: st, err: tt.err}, MachineClient: m})

			var diags diag.Diagnostics

			_, err := trySend(t.Context(), op, true, &diags)
			require.NoError(t, err)
			require.Equal(t, []applyMode{tryAuto}, m.modes())
		})
	}
}

func TestTalosMachineSendConfigFallsBackWithoutTry(t *testing.T) {
	t.Parallel()

	// Talos before 1.14 rejects try mode for a change that needs a reboot.
	m := &tryTestMachine{reply: func(_ int, mode applyMode) (applyMode, error) {
		if mode == tryTry {
			return 0, status.Error(codes.InvalidArgument, "configuration change requires a reboot")
		}

		return tryReboot, nil
	}}

	var diags diag.Diagnostics

	rebooted, err := trySend(t.Context(), tryTestOp(t, runtimeres.MachineStageRunning, m), true, &diags)
	require.NoError(t, err)
	require.True(t, rebooted)
	require.Equal(t, []applyMode{tryTry, tryAuto}, m.modes())
	requireWarning(t, diags, "Configuration applied without try mode", "rejected the try-mode apply")
}

func TestTalosMachineSendConfigInvalidConfig(t *testing.T) {
	t.Parallel()

	m := &tryTestMachine{reply: failFirst(math.MaxInt, status.Error(codes.InvalidArgument, "bad config"))}

	var diags diag.Diagnostics

	_, err := trySend(t.Context(), tryTestOp(t, runtimeres.MachineStageRunning, m), true, &diags)
	require.ErrorContains(t, err, "bad config")
	require.Equal(t, []applyMode{tryTry, tryAuto}, m.modes())
	require.Empty(t, diags)
}

func TestTalosMachineSendConfigDoesNotResendTry(t *testing.T) {
	t.Parallel()

	// The try request may have reached the node: sending it again would make
	// Talos roll back to the first request's configuration.
	m := &tryTestMachine{reply: failFirst(math.MaxInt, status.Error(codes.Unavailable, "connection reset"))}

	var diags diag.Diagnostics

	_, err := trySend(t.Context(), tryTestOp(t, runtimeres.MachineStageRunning, m), true, &diags)
	require.ErrorContains(t, err, "may have reached the node")
	require.Equal(t, []applyMode{tryTry}, m.modes())
}

func TestTalosMachineSendConfigRetriesTry(t *testing.T) {
	t.Parallel()

	t.Run("another apply in progress", func(t *testing.T) {
		t.Parallel()

		m := &tryTestMachine{reply: failFirst(1, status.Error(codes.FailedPrecondition, "another apply configuration is already in progress"))}

		var diags diag.Diagnostics

		_, err := trySend(t.Context(), tryTestOp(t, runtimeres.MachineStageRunning, m), true, &diags)
		require.NoError(t, err)
		require.Equal(t, []applyMode{tryTry, tryTry, tryNoReboot}, m.modes())
	})

	t.Run("connection failed before sending", func(t *testing.T) {
		t.Parallel()

		m := &tryTestMachine{}
		op := tryTestOp(t, runtimeres.MachineStageRunning, m)
		failed := false
		flaky := func(ctx context.Context, endpoint, node string, cfg *clientconfig.Config, fn func(context.Context, *client.Client) error) error {
			if !failed {
				failed = true

				return status.Error(codes.Unavailable, "dial failed")
			}

			return op(ctx, endpoint, node, cfg, fn)
		}

		var diags diag.Diagnostics

		_, err := trySend(t.Context(), flaky, true, &diags)
		require.NoError(t, err)
		require.Equal(t, []applyMode{tryTry, tryNoReboot}, m.modes())
	})
}

func tryConfirm(ctx context.Context, op talos.ClientOpFunc, sentAt time.Time) error {
	return talos.TalosMachineConfirmTry(ctx, "endpoint", cpTestNode, &clientconfig.Config{}, []byte("config"), op, sentAt, tryTestTiming)
}

func TestTalosMachineConfirmTry(t *testing.T) {
	t.Parallel()

	m := &tryTestMachine{reply: failFirst(2, status.Error(codes.Unavailable, "node unreachable"))}
	sentAt := time.Now()

	require.NoError(t, tryConfirm(t.Context(), tryTestOp(t, runtimeres.MachineStageRunning, m), sentAt))
	require.Equal(t, []applyMode{tryNoReboot, tryNoReboot, tryNoReboot}, m.modes())
	require.GreaterOrEqual(t, m.calls[0].at.Sub(sentAt), tryTestTiming.CommitAfter, "confirmed before the node had time to drop off")
}

func TestTalosMachineConfirmTryGivesUpBeforeRollback(t *testing.T) {
	t.Parallel()

	m := &tryTestMachine{reply: failFirst(math.MaxInt, status.Error(codes.Unavailable, "node unreachable"))}
	sentAt := time.Now()

	require.ErrorContains(t, tryConfirm(t.Context(), tryTestOp(t, runtimeres.MachineStageRunning, m), sentAt), "did not stay reachable")
	require.NotEmpty(t, m.calls)

	// Attempts may start a little after the last planned start because of timer
	// jitter; what must hold is that none can still reach the node after Talos
	// rolled the configuration back.
	rollback := sentAt.Add(tryTestTiming.RollbackAfter)
	for _, c := range m.calls {
		end := c.at.Add(tryTestTiming.AttemptTimeout)
		require.False(t, end.After(rollback), "confirmation attempt can reach the node %s after the rollback", end.Sub(rollback))
	}
}

func TestTalosMachineConfirmTryCanceled(t *testing.T) {
	t.Parallel()

	m := &tryTestMachine{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.ErrorIs(t, tryConfirm(ctx, tryTestOp(t, runtimeres.MachineStageRunning, m), time.Now()), context.Canceled)
	require.Empty(t, m.modes())
}
