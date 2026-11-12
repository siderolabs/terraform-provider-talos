// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

type (
	ClientOpFunc           = clientOpFunc
	ControlPlaneWaitTiming = controlPlaneWaitTiming
	TryApplyTiming         = tryApplyTiming
)

var (
	TalosMachineCheckBootReady          = talosMachineCheckBootReady
	TalosMachineWaitForBoot             = talosMachineWaitForBoot
	TalosMachineRebootBeforeBootstrap   = talosMachineRebootBeforeBootstrap
	TalosMachineControlPlaneNodeName    = talosMachineControlPlaneNodeName
	TalosMachineWaitForControlPlane     = talosMachineWaitForControlPlane
	TalosMachineStaticPodsSettle        = talosMachineStaticPodsSettle
	TalosMachinePrepareControlPlaneWait = talosMachinePrepareControlPlaneWait
	TalosMachineRunControlPlaneWait     = talosMachineRunControlPlaneWait
	TalosMachineDefaultTimeout          = talosMachineDefaultTimeout
	TalosMachineSendConfig              = talosMachineSendConfig
	TalosMachineConfirmTry              = talosMachineConfirmTry
)
