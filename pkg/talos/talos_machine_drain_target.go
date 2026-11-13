// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	k8sres "github.com/siderolabs/talos/pkg/machinery/resources/k8s"
)

// kubeletClientCertFile is the kubelet's client certificate. It only exists once the node
// has joined the cluster.
const kubeletClientCertFile = "kubelet-client-current.pem"

// talosMachineKubeletJoined reports whether the node has joined the cluster, by looking for
// the kubelet's client certificate. It is the check Talos itself makes before it drains a
// node ("the node hasn't fully joined yet"). The directory is listed rather than the file
// read, so the private key in the certificate file is not fetched, and the Reader role is
// enough. Anything that cannot be told is an error, never "not joined".
func talosMachineKubeletJoined(ctx context.Context, c *client.Client) (bool, error) {
	joined, err := kubeletClientCertListed(ctx, c)
	if errors.Is(err, errKubeletNotJoined) {
		return false, nil
	}

	return joined, err
}

func kubeletClientCertListed(ctx context.Context, c *client.Client) (bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	stream, err := c.LS(ctx, &machineapi.ListRequest{
		Root:  constants.KubeletPKIDir,
		Types: []machineapi.ListRequest_Type{machineapi.ListRequest_REGULAR, machineapi.ListRequest_SYMLINK},
	})
	if err != nil {
		return false, kubeletPKIListError(err)
	}

	for {
		fi, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return false, nil
		}

		if err != nil {
			return false, kubeletPKIListError(err)
		}

		if msg := fi.GetError(); msg != "" {
			return false, kubeletPKIListError(errors.New(msg))
		}

		if fi.GetRelativeName() == kubeletClientCertFile {
			return true, nil
		}
	}
}

// kubeletPKIListError turns a failed listing of the kubelet's PKI directory into the answer.
// A node that never joined has no such directory, and Talos fails the whole call with
// "lstat <dir>: no such file or directory" rather than reporting it per entry. That is "not
// joined", not an error. Any other failure leaves the question open.
func kubeletPKIListError(err error) error {
	if strings.Contains(err.Error(), constants.KubeletPKIDir+": no such file or directory") {
		return errKubeletNotJoined
	}

	return fmt.Errorf("listing %s: %w", constants.KubeletPKIDir, err)
}

// errKubeletNotJoined is the answer "the kubelet's PKI directory does not exist".
var errKubeletNotJoined = errors.New("the kubelet's PKI directory does not exist")

// talosMachineDrainNodeName resolves the Kubernetes node name used for the drain. A node
// whose machine config sets skipNodeRegistration never gets a Kubernetes node, and Talos
// itself skips the drain for it, so it reports noNode. Every other failure to resolve the
// name, including an empty one, is an error.
func talosMachineDrainNodeName(ctx context.Context, c *client.Client) (name string, noNode bool, err error) {
	nodename, err := safe.StateGetByID[*k8sres.Nodename](ctx, c.COSI, k8sres.NodenameID)
	if err != nil {
		return "", false, fmt.Errorf("resolving k8s node name: %w", err)
	}

	if nodename.TypedSpec().SkipNodeRegistration {
		tflog.Info(ctx, "machine config sets skipNodeRegistration; there is no Kubernetes node to drain")

		return "", true, nil
	}

	name = nodename.TypedSpec().Nodename
	if strings.TrimSpace(name) == "" {
		return "", false, errors.New("resolving k8s node name: Talos reports an empty node name")
	}

	return name, false, nil
}
