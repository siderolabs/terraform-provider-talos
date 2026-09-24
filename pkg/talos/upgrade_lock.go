// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"sync/atomic"

	"golang.org/x/sync/semaphore"
)

// Shared by all opted-in resources for the lifetime of the provider process.
var machineUpgradeLock = newUpgradeLock()

type upgradeLock struct {
	semaphore *semaphore.Weighted
	failed    atomic.Pointer[error]
}

func newUpgradeLock() *upgradeLock {
	return &upgradeLock{semaphore: semaphore.NewWeighted(1)}
}

func (l *upgradeLock) acquire(ctx context.Context, enabled bool) (func(error), error) {
	if !enabled {
		return func(error) {}, nil
	}

	if err := l.semaphore.Acquire(ctx, 1); err != nil {
		return nil, fmt.Errorf("waiting for serialized upgrade: %w", err)
	}

	if failed := l.failed.Load(); failed != nil {
		l.semaphore.Release(1)

		return nil, fmt.Errorf("serialized upgrades stopped after an earlier failure: %w", *failed)
	}

	return func(err error) {
		l.stop(err)
		l.semaphore.Release(1)
	}, nil
}

// Preparation can fail while another resource holds the upgrade semaphore.
func (l *upgradeLock) stop(err error) {
	if err != nil {
		l.failed.CompareAndSwap(nil, &err)
	}
}
