// SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"strings"
	"testing"

	ovntest "github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/testing"
	"github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/util"
)

func setFakeExec(t *testing.T, cmds []*ovntest.ExpectedCmd) *ovntest.FakeExec {
	t.Helper()
	fexec := ovntest.NewFakeExec()
	fexec.AddFakeCmds(cmds)
	if err := util.SetExec(fexec); err != nil {
		t.Fatalf("failed to set the fake exec: %v", err)
	}
	t.Cleanup(util.ResetRunner)
	return fexec
}

// TestNorthdReadinessCallTimeout checks that every ovn-northd readiness call uses
// readinessCallTimeout.
func TestNorthdReadinessCallTimeout(t *testing.T) {
	fexec := setFakeExec(t, []*ovntest.ExpectedCmd{
		{Cmd: "ovn-appctl --timeout=12 -t ovn-northd status", Output: "Status: active"},
		{Cmd: "ovn-appctl --timeout=12 -t ovn-northd nb-connection-status", Output: "connected"},
		{Cmd: "ovn-appctl --timeout=12 -t ovn-northd sb-connection-status", Output: "connected"},
	})
	if err := ovnNorthdReadiness("ovn-northd"); err != nil {
		t.Fatalf("ovn-northd readiness failed: %v", err)
	}
	if !fexec.CalledMatchesExpected() {
		t.Fatal(fexec.ErrorDesc())
	}
}

// TestOVNControllerReadinessCallTimeout checks that the two calls to ovn-controller
// itself use readinessCallTimeout. The lflow_run counter is 0, so the check stops
// before it reads the OVS pid files and calls ovsdb-server and ovs-vswitchd.
func TestOVNControllerReadinessCallTimeout(t *testing.T) {
	fexec := setFakeExec(t, []*ovntest.ExpectedCmd{
		{Cmd: "ovs-appctl --timeout=12 -t ovn-controller connection-status", Output: "connected"},
		{Cmd: "ovs-appctl --timeout=12 -t ovn-controller coverage/read-counter lflow_run", Output: "0"},
	})
	err := ovnControllerReadiness("ovn-controller")
	if err == nil || !strings.Contains(err.Error(), "has not completed logical flows processing yet") {
		t.Fatalf("expected the lflow_run check to fail, got: %v", err)
	}
	if !fexec.CalledMatchesExpected() {
		t.Fatal(fexec.ErrorDesc())
	}
}
