// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/nodedrain"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

// Reservations are deliberately process-local. Provider aliases hosted in a
// different plugin process and concurrent applies do NOT share this coordinator.
var machineUpgradeBudgets = upgradeBudgetCoordinator{groups: make(map[string]map[string]*upgradeBudgetGroup)}

type upgradePolicyModel struct {
	Group          types.String `tfsdk:"group"`
	NodeNames      types.Set    `tfsdk:"node_names"`
	MaxUnavailable types.String `tfsdk:"max_unavailable"`
}

type upgradePolicy struct {
	group   string
	members []string
	limit   int
}

func upgradePolicySchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional: true,
		Description: "Experimental worker OS upgrade budget shared within one provider process. Requires drain_on_upgrade and kubeconfig. " +
			"Does not coordinate separate provider processes or concurrent applies. Applies to image updates of existing nodes only.",
		Attributes: map[string]schema.Attribute{
			"group": schema.StringAttribute{
				Required:    true,
				Description: "Group name, scoped to the Kubernetes cluster. All members must use identical membership and budget settings.",
			},
			"node_names": schema.SetAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "Complete set of Kubernetes worker node names, including unchanged nodes. Groups must not overlap. Nodes must already exist.",
			},
			"max_unavailable": schema.StringAttribute{
				Required: true,
				Description: "Positive integer (for example 1) or percentage (for example 25%). Percentages round down and must permit at least one node. " +
					"Existing NotReady, Unknown, deleting, or cordoned nodes count against the budget.",
			},
		},
	}
}

// unknown is returned separately so validation can defer computed values, while
// execution can reject an unresolved policy before any disruptive operation.
func decodeUpgradePolicy(ctx context.Context, value types.Object) (policy *upgradePolicy, unknown bool, err error) {
	if value.IsNull() {
		return nil, false, nil
	}

	if value.IsUnknown() {
		return nil, true, nil
	}

	var model upgradePolicyModel
	if diags := value.As(ctx, &model, basetypes.ObjectAsOptions{}); diags.HasError() {
		return nil, false, fmt.Errorf("decoding upgrade_policy: %s", diags.Errors())
	}

	if model.Group.IsUnknown() || model.MaxUnavailable.IsUnknown() || model.NodeNames.IsUnknown() {
		return nil, true, nil
	}

	for _, element := range model.NodeNames.Elements() {
		if element.IsUnknown() {
			return nil, true, nil
		}
	}

	if model.Group.IsNull() || strings.TrimSpace(model.Group.ValueString()) == "" {
		return nil, false, errors.New("upgrade_policy.group must not be empty")
	}

	var members []string
	if diags := model.NodeNames.ElementsAs(ctx, &members, false); diags.HasError() {
		return nil, false, errors.New("upgrade_policy.node_names must contain non-null node names")
	}

	if len(members) == 0 {
		return nil, false, errors.New("upgrade_policy.node_names must not be empty")
	}

	for _, name := range members {
		if problems := validation.IsDNS1123Subdomain(name); len(problems) != 0 {
			return nil, false, fmt.Errorf("invalid Kubernetes node name %q", name)
		}
	}

	slices.Sort(members)

	limit, err := upgradeBudgetLimit(model.MaxUnavailable.ValueString(), len(members))
	if err != nil {
		return nil, false, err
	}

	return &upgradePolicy{group: model.Group.ValueString(), members: members, limit: limit}, false, nil
}

func upgradeBudgetLimit(value string, size int) (int, error) {
	raw := strings.TrimSuffix(value, "%")
	if raw == "" || strings.IndexFunc(raw, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, fmt.Errorf("max_unavailable %q must be a positive integer or percentage", value)
	}

	amount, err := strconv.Atoi(raw)
	if err != nil || amount <= 0 {
		return 0, fmt.Errorf("invalid max_unavailable %q", value)
	}

	if strings.HasSuffix(value, "%") {
		if amount > 100 {
			return 0, errors.New("max_unavailable percentage must not exceed 100%")
		}
		// Avoid multiplying the complete group size by the percentage.
		amount = size/100*amount + size%100*amount/100
	}

	if amount < 1 || amount > size {
		return 0, fmt.Errorf("max_unavailable %q resolves to %d for %d nodes; budget must be between 1 and the group size", value, amount, size)
	}

	return amount, nil
}

type upgradeBudgetCoordinator struct {
	groups map[string]map[string]*upgradeBudgetGroup
	mu     sync.Mutex
}

type upgradeBudgetGroup struct {
	stopped  error
	reserved map[string]struct{}
	policy   upgradePolicy
	mu       sync.Mutex
}

func (c *upgradeBudgetCoordinator) group(cluster string, policy upgradePolicy) (*upgradeBudgetGroup, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.groups == nil {
		c.groups = make(map[string]map[string]*upgradeBudgetGroup)
	}

	if c.groups[cluster] == nil {
		c.groups[cluster] = make(map[string]*upgradeBudgetGroup)
	}

	for name, group := range c.groups[cluster] {
		if name == policy.group {
			if group.policy.limit != policy.limit || !slices.Equal(group.policy.members, policy.members) {
				return nil, fmt.Errorf("upgrade group %q has conflicting membership or max_unavailable settings", name)
			}

			return group, nil
		}

		for _, member := range policy.members {
			if slices.Contains(group.policy.members, member) {
				return nil, fmt.Errorf("worker %q belongs to overlapping upgrade groups %q and %q", member, name, policy.group)
			}
		}
	}

	group := &upgradeBudgetGroup{policy: policy, reserved: make(map[string]struct{})}
	c.groups[cluster][policy.group] = group

	return group, nil
}

// The reader returns true only for Ready, schedulable, non-deleting nodes.
type upgradeNodeAvailability func(context.Context) (map[string]bool, error)

func (g *upgradeBudgetGroup) reserve(ctx context.Context, node string, read upgradeNodeAvailability) (bool, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return false, "", err
	}

	if g.stopped != nil {
		return false, "", fmt.Errorf("upgrade group %q stopped after an earlier failure: %w", g.policy.group, g.stopped)
	}

	if !slices.Contains(g.policy.members, node) {
		return false, "", fmt.Errorf("node %q is not in upgrade_policy.node_names", node)
	}

	if _, exists := g.reserved[node]; exists {
		return false, "", fmt.Errorf("node %q already has an upgrade reservation", node)
	}

	available, err := read(ctx)
	if err != nil {
		return false, "", err
	}

	unavailable := 0

	for _, member := range g.policy.members {
		healthy, exists := available[member]
		if !exists {
			return false, "", fmt.Errorf("upgrade group member %q is missing; refusing to infer pool capacity", member)
		}

		_, reserved := g.reserved[member]
		if !healthy || reserved {
			unavailable++
		}
	}

	if !available[node] || unavailable >= g.policy.limit {
		return false, fmt.Sprintf("%d/%d unavailable or reserved (limit %d); candidate %q ready and schedulable: %t", unavailable, len(g.policy.members), g.policy.limit, node, available[node]), nil
	}

	g.reserved[node] = struct{}{}

	return true, "", nil
}

func (g *upgradeBudgetGroup) run(ctx context.Context, node string, read upgradeNodeAvailability, upgrade func() error, interval time.Duration) error {
	for {
		admitted, reason, err := g.reserve(ctx, node, read)
		if err != nil {
			return err
		}

		if admitted {
			break
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()

			return fmt.Errorf("waiting for upgrade group %q: %s: %w", g.policy.group, reason, ctx.Err())
		case <-timer.C:
		}
	}

	err := ctx.Err()
	if err == nil {
		err = upgrade()
	}

	if err == nil {
		var available map[string]bool

		available, err = read(ctx)
		if err == nil && !available[node] {
			err = fmt.Errorf("upgraded node %q is not Ready and schedulable", node)
		}
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if err != nil {
		// Keep the reservation and close admission for this process. A failed API
		// call or cancellation does not prove that the reboot did not start.
		g.stopped = fmt.Errorf("node %q: %w", node, err)

		return err
	}

	delete(g.reserved, node)

	return nil
}

func workerAvailability(ctx context.Context, cs kubernetes.Interface, members []string) (map[string]bool, error) {
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	nodes, err := cs.CoreV1().Nodes().List(readCtx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("reading upgrade group health: %w", err)
	}

	result := make(map[string]bool, len(members))
	for _, node := range nodes.Items {
		if !slices.Contains(members, node.Name) {
			continue
		}

		for _, label := range []string{"node-role.kubernetes.io/control-plane", "node-role.kubernetes.io/master"} {
			if _, exists := node.Labels[label]; exists {
				return nil, fmt.Errorf("upgrade_policy supports workers only; %q is a control-plane node", node.Name)
			}
		}

		ready := false

		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				ready = condition.Status == corev1.ConditionTrue
			}
		}

		result[node.Name] = ready && !node.Spec.Unschedulable && node.DeletionTimestamp == nil
	}

	return result, nil
}

func talosMachineUpgradeWithBudget(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, state *talosMachineResourceModel) error {
	policy, unknown, err := decodeUpgradePolicy(ctx, state.UpgradePolicy)
	if err != nil {
		return err
	}

	if unknown {
		return errors.New("upgrade_policy must be fully known before upgrading")
	}

	upgrade := func() error { return talosMachineUpgrade(ctx, endpoint, node, talosConfig, state, true) }
	if policy == nil {
		return upgrade()
	}

	if !state.DrainOnUpgrade.ValueBool() {
		return errors.New("upgrade_policy requires drain_on_upgrade = true")
	}

	raw := state.KubeconfigWO.ValueString()
	if raw == "" {
		raw = state.Kubeconfig.ValueString()
	}

	cs, err := kubeclientFromRaw([]byte(raw))
	if err != nil {
		return fmt.Errorf("building upgrade budget Kubernetes client: %w", err)
	}
	// Namespace UID scopes groups by actual cluster identity, not API endpoint
	// spelling or credentials, which can differ across resources.
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	namespace, err := cs.CoreV1().Namespaces().Get(readCtx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("reading cluster identity for upgrade budget: %w", err)
	}

	if namespace.UID == "" {
		return errors.New("kube-system namespace has no UID")
	}

	var name string

	if err = talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		var lookupErr error

		name, lookupErr = nodedrain.GetKubernetesNodeName(nodeCtx, c)

		return lookupErr
	}); err != nil {
		return fmt.Errorf("resolving upgrade group node name: %w", err)
	}

	group, err := machineUpgradeBudgets.group(string(namespace.UID), *policy)
	if err != nil {
		return err
	}

	read := func(ctx context.Context) (map[string]bool, error) { return workerAvailability(ctx, cs, policy.members) }

	return group.run(ctx, name, read, upgrade, 2*time.Second)
}
