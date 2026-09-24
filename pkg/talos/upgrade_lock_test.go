// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // tests the internal upgrade lock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpgradeLockSerializes(t *testing.T) {
	t.Parallel()

	locks := newUpgradeLock()

	var (
		active atomic.Int32
		wg     sync.WaitGroup
	)

	start := make(chan struct{})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for range 32 {
		wg.Go(func() {
			<-start

			release, err := locks.acquire(ctx, true)
			if err != nil {
				t.Error(err)

				return
			}
			defer release(nil)

			if active.Add(1) != 1 {
				t.Error("overlapping upgrades")
			}

			active.Add(-1)
		})
	}

	close(start)
	wg.Wait()
	require.Zero(t, active.Load())
}

func TestUpgradeLockScopeAndCancellation(t *testing.T) {
	t.Parallel()

	locks := newUpgradeLock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	release, err := locks.acquire(ctx, true)
	require.NoError(t, err)

	// Opted-out operations do not wait for the held lock.
	unlock, acquireErr := locks.acquire(ctx, false)
	require.NoError(t, acquireErr)
	unlock(nil)

	waiting, cancelWait := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancelWait()

	_, err = locks.acquire(waiting, true)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	release(nil)
	// A timed-out waiter must not retain the lock.
	release, err = locks.acquire(ctx, true)
	require.NoError(t, err)
	release(nil)

	canceled, cancelNow := context.WithCancel(ctx)
	cancelNow()

	_, err = locks.acquire(canceled, true)
	require.ErrorIs(t, err, context.Canceled)
}

func TestUpgradeLockStopsAfterFailure(t *testing.T) {
	t.Parallel()

	locks := newUpgradeLock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	release, err := locks.acquire(ctx, true)
	require.NoError(t, err)

	// An already waiting upgrade must observe the failure before entering.
	result := make(chan error, 1)

	go func() {
		unlock, acquireErr := locks.acquire(ctx, true)
		if acquireErr == nil {
			unlock(nil)
		}

		result <- acquireErr
	}()

	failed := errors.New("reboot failed")
	release(failed)
	require.ErrorIs(t, <-result, failed)

	// The stop persists for later callers, but does not affect opted-out operations.
	_, err = locks.acquire(ctx, true)
	require.ErrorContains(t, err, "stopped after an earlier failure")
	release, err = locks.acquire(ctx, false)
	require.NoError(t, err)
	release(nil)

	// A fresh provider process starts with a fresh lock.
	restarted := newUpgradeLock()

	release, err = restarted.acquire(ctx, true)
	require.NoError(t, err)
	release(nil)
}

func TestUpgradeLockPreparationFailure(t *testing.T) {
	t.Parallel()

	lock := newUpgradeLock()
	release, err := lock.acquire(t.Context(), true)
	require.NoError(t, err)

	// Another resource can fail image preparation while this upgrade is active.
	failed := errors.New("image preparation failed")
	lock.stop(failed)
	release(nil)

	_, err = lock.acquire(t.Context(), true)
	require.ErrorIs(t, err, failed, "successful completion must not clear an earlier failure")
}

func TestUpgradeLockAdmittedCancellation(t *testing.T) {
	t.Parallel()

	lock := newUpgradeLock()
	release, err := lock.acquire(t.Context(), true)
	require.NoError(t, err)
	release(context.Canceled)

	_, err = lock.acquire(t.Context(), true)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorContains(t, err, "stopped after an earlier failure")
}

func TestUpgradeLockCancelBlockedWaiter(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		lock := newUpgradeLock()
		release, err := lock.acquire(t.Context(), true)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		result := make(chan error, 1)

		go func() {
			unlock, acquireErr := lock.acquire(ctx, true)
			if acquireErr == nil {
				unlock(nil)
			}

			result <- acquireErr
		}()

		// Wait until the goroutine is blocked inside acquire, then cancel it.
		synctest.Wait()
		cancel()
		require.ErrorIs(t, <-result, context.Canceled)
		require.False(t, lock.semaphore.TryAcquire(1), "canceling a waiter must not release the active upgrade")
		require.Nil(t, lock.failed.Load(), "canceling only a waiter must not stop other upgrades")

		release(nil)

		unlock, err := lock.acquire(t.Context(), true)
		require.NoError(t, err, "an independent upgrade can proceed after the active one finishes")
		unlock(nil)
	})
}
