// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // exercises the admission coordinator without live infrastructure

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestUpgradeBudgetLimit(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		value      string
		size, want int
	}{
		{"1", 3, 1},
		{"25%", 4, 1},
		{"34%", 3, 1},
		{"100%", 3, 3},
		{"25%", 3, 0},
		{"0", 4, 0},
		{"0%", 4, 0},
		{"101%", 4, 0},
		{"5", 4, 0},
		{"-1", 4, 0},
		{"1.5", 4, 0},
		{"", 4, 0},
		{" 1", 4, 0},
		{"1", 0, 0},
		{"9999999999999999999999999", 4, 0},
	} {
		t.Run(fmt.Sprintf("%s/%d", test.value, test.size), func(t *testing.T) {
			t.Parallel()

			limit, err := upgradeBudgetLimit(test.value, test.size)
			if test.want == 0 {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.want, limit)
			}
		})
	}
}

func TestUpgradeBudgetPolicyDecoding(t *testing.T) {
	t.Parallel()

	policyType := map[string]attr.Type{"group": types.StringType, "node_names": types.SetType{ElemType: types.StringType}, "max_unavailable": types.StringType}
	makePolicy := func(group string, names types.Set, budget string) types.Object {
		return types.ObjectValueMust(policyType, map[string]attr.Value{
			"group": types.StringValue(group), "node_names": names, "max_unavailable": types.StringValue(budget),
		})
	}
	names := types.SetValueMust(types.StringType, []attr.Value{types.StringValue("w2"), types.StringValue("w1")})
	ctx := context.Background()
	policy, unknown, err := decodeUpgradePolicy(ctx, makePolicy("workers", names, "50%"))
	require.NoError(t, err)
	require.False(t, unknown)
	require.Equal(t, &upgradePolicy{group: "workers", members: []string{"w1", "w2"}, limit: 1}, policy)

	for _, value := range []types.Object{
		makePolicy("", names, "1"),
		makePolicy("workers", types.SetValueMust(types.StringType, nil), "1"),
		makePolicy("workers", types.SetValueMust(types.StringType, []attr.Value{types.StringNull()}), "1"),
		makePolicy("workers", types.SetValueMust(types.StringType, []attr.Value{types.StringValue("bad/name")}), "1"),
		makePolicy("workers", names, "25%"),
	} {
		_, _, err = decodeUpgradePolicy(ctx, value)
		require.Error(t, err)
	}

	for _, value := range []types.Object{
		types.ObjectUnknown(policyType),
		makePolicy("workers", types.SetUnknown(types.StringType), "1"),
		makePolicy("workers", types.SetValueMust(types.StringType, []attr.Value{types.StringUnknown()}), "1"),
	} {
		_, unknown, err = decodeUpgradePolicy(ctx, value)
		require.NoError(t, err)
		require.True(t, unknown)
	}

	policy, unknown, err = decodeUpgradePolicy(ctx, types.ObjectNull(policyType))
	require.NoError(t, err)
	require.False(t, unknown)
	require.Nil(t, policy)
}

func newTestBudget(t *testing.T, names []string, limit int) *upgradeBudgetGroup {
	t.Helper()

	c := &upgradeBudgetCoordinator{}
	group, err := c.group("cluster", upgradePolicy{group: "workers", members: names, limit: limit})
	require.NoError(t, err)

	return group
}

func availableNodes(names ...string) upgradeNodeAvailability {
	return func(context.Context) (map[string]bool, error) {
		result := make(map[string]bool, len(names))
		for _, name := range names {
			result[name] = true
		}

		return result, nil
	}
}

func TestUpgradeBudgetConcurrentReservations(t *testing.T) {
	t.Parallel()

	names := make([]string, 32)
	for i := range names {
		names[i] = fmt.Sprintf("worker-%02d", i)
	}

	group := newTestBudget(t, names, 4)

	var (
		admitted atomic.Int32
		wg       sync.WaitGroup
	)

	start := make(chan struct{})

	for _, name := range names {
		wg.Go(func() {
			<-start

			ok, _, err := group.reserve(context.Background(), name, availableNodes(names...))
			if err != nil {
				t.Error(err)
			}

			if ok {
				admitted.Add(1)
			}
		})
	}

	close(start)
	wg.Wait()
	require.EqualValues(t, 4, admitted.Load(), "live status remains healthy; reservations must still prevent over-admission")
}

func TestUpgradeBudgetCountsUnavailableAndReservationsOnce(t *testing.T) {
	t.Parallel()
	group := newTestBudget(t, []string{"w1", "w2", "w3", "w4"}, 2)
	ok, _, err := group.reserve(context.Background(), "w1", availableNodes("w1", "w2", "w3", "w4"))
	require.NoError(t, err)
	require.True(t, ok)

	read := func(context.Context) (map[string]bool, error) {
		return map[string]bool{"w1": false, "w2": true, "w3": true, "w4": true}, nil
	}
	ok, _, err = group.reserve(context.Background(), "w2", read)
	require.NoError(t, err)
	require.True(t, ok, "w1's reservation and NotReady status consume one slot, not two")
	ok, _, err = group.reserve(context.Background(), "w3", read)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestUpgradeBudgetExistingFailureAndMissingMembers(t *testing.T) {
	t.Parallel()
	group := newTestBudget(t, []string{"w1", "w2"}, 1)
	read := func(context.Context) (map[string]bool, error) { return map[string]bool{"w1": true, "w2": false}, nil }
	ok, reason, err := group.reserve(context.Background(), "w1", read)
	require.NoError(t, err)
	require.False(t, ok)
	require.Contains(t, reason, "1/2 unavailable")

	_, _, err = group.reserve(context.Background(), "w1", availableNodes("w1"))
	require.ErrorContains(t, err, "missing")
	_, _, err = group.reserve(context.Background(), "other", availableNodes("w1", "w2"))
	require.ErrorContains(t, err, "not in")
	_, _, err = group.reserve(context.Background(), "w1", func(context.Context) (map[string]bool, error) { return nil, errors.New("API unavailable") })
	require.ErrorContains(t, err, "API unavailable")
	require.Empty(t, group.reserved)
}

func TestUpgradeBudgetRunReleasesOnlyAfterRecovery(t *testing.T) {
	t.Parallel()
	group := newTestBudget(t, []string{"w1", "w2"}, 1)
	err := group.run(context.Background(), "w1", availableNodes("w1", "w2"), func() error {
		ok, _, err := group.reserve(context.Background(), "w2", availableNodes("w1", "w2"))
		require.NoError(t, err)
		require.False(t, ok)

		return nil
	}, time.Millisecond)
	require.NoError(t, err)
	require.Empty(t, group.reserved)
	ok, _, err := group.reserve(context.Background(), "w2", availableNodes("w1", "w2"))
	require.NoError(t, err)
	require.True(t, ok)
}

func TestUpgradeBudgetFailureStopsGroup(t *testing.T) {
	t.Parallel()

	for _, failsOperation := range []bool{true, false} {
		t.Run(fmt.Sprint(failsOperation), func(t *testing.T) {
			t.Parallel()

			group := newTestBudget(t, []string{"w1", "w2"}, 1)
			returned := false
			read := func(context.Context) (map[string]bool, error) {
				return map[string]bool{"w1": !returned, "w2": true}, nil
			}
			err := group.run(context.Background(), "w1", read, func() error {
				returned = true

				if failsOperation {
					return errors.New("reboot failed")
				}

				return nil
			}, time.Millisecond)
			require.Error(t, err)
			require.Contains(t, group.reserved, "w1")
			_, _, err = group.reserve(context.Background(), "w2", availableNodes("w1", "w2"))
			require.ErrorContains(t, err, "stopped after an earlier failure")
		})
	}
}

func TestUpgradeBudgetWaitCancellation(t *testing.T) {
	t.Parallel()
	group := newTestBudget(t, []string{"w1", "w2"}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	read := func(context.Context) (map[string]bool, error) {
		cancel()

		return map[string]bool{"w1": true, "w2": false}, nil
	}
	err := group.run(ctx, "w1", read, func() error {
		t.Fatal("must not start an upgrade")

		return nil
	}, time.Hour)
	require.ErrorIs(t, err, context.Canceled)
	require.Contains(t, err.Error(), "waiting for upgrade group")
	require.Empty(t, group.reserved)
}

func TestUpgradeBudgetGroupIsolationAndConflicts(t *testing.T) {
	t.Parallel()

	c := &upgradeBudgetCoordinator{}
	policy := upgradePolicy{group: "pool", members: []string{"w1", "w2"}, limit: 1}
	group, err := c.group("cluster-1", policy)
	require.NoError(t, err)
	again, err := c.group("cluster-1", policy)
	require.NoError(t, err)
	require.Same(t, group, again)
	other, err := c.group("cluster-2", policy)
	require.NoError(t, err)
	require.NotSame(t, group, other)
	_, err = c.group("cluster-1", upgradePolicy{group: "pool", members: policy.members, limit: 2})
	require.ErrorContains(t, err, "conflicting")
	_, err = c.group("cluster-1", upgradePolicy{group: "other", members: []string{"w2"}, limit: 1})
	require.ErrorContains(t, err, "overlapping")
	_, err = c.group("cluster-1", upgradePolicy{group: "other", members: []string{"w3"}, limit: 1})
	require.NoError(t, err)
}

func TestUpgradeBudgetWorkerAvailability(t *testing.T) {
	t.Parallel()

	names := []string{"ready", "notready", "unknown", "cordoned", "deleting", "no-condition"}

	objects := make([]runtime.Object, 0, len(names))
	for _, name := range names {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		}}
		switch name {
		case "notready":
			node.Status.Conditions[0].Status = corev1.ConditionFalse
		case "unknown":
			node.Status.Conditions[0].Status = corev1.ConditionUnknown
		case "cordoned":
			node.Spec.Unschedulable = true
		case "deleting":
			now := metav1.Now()
			node.DeletionTimestamp = &now
		case "no-condition":
			node.Status.Conditions = nil
		}

		objects = append(objects, node)
	}

	cs := fake.NewClientset(objects...)
	states, err := workerAvailability(context.Background(), cs, append(names, "missing"))
	require.NoError(t, err)
	require.Len(t, states, len(names))

	for _, name := range names {
		require.Equal(t, name == "ready", states[name], name)
	}

	for _, role := range []string{"control-plane", "master"} {
		cs := fake.NewClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp", Labels: map[string]string{"node-role.kubernetes.io/" + role: ""}}})
		_, err := workerAvailability(context.Background(), cs, []string{"cp"})
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), "workers only"))
	}
}

func TestUpgradeBudgetWaitsForCapacity(t *testing.T) {
	t.Parallel()
	group := newTestBudget(t, []string{"w1", "w2"}, 1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var reads atomic.Int32

	err := group.run(ctx, "w1", func(context.Context) (map[string]bool, error) {
		return map[string]bool{"w1": true, "w2": reads.Add(1) > 1}, nil
	}, func() error {
		require.GreaterOrEqual(t, reads.Load(), int32(2))

		return nil
	}, time.Millisecond)
	require.NoError(t, err)
	require.Empty(t, group.reserved)
}
