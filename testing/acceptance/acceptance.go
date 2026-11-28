// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: MPL-2.0

// Package acceptance provides the live acceptance test harness for the
// builders. The tests run on a local VMware Fusion Pro or VMware Workstation
// Pro host and skip unless PACKER_ACC is set.
package acceptance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/acctest"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	vmwcommon "github.com/vmware/packer-plugin-vmware/builder/vmware/common"
)

const (
	// EnvSourceVMX is the .vmx used by the clone rows.
	EnvSourceVMX = "VMWARE_SOURCE_VMX"
	// EnvSourceOVF is the .ovf used by the clone rows.
	EnvSourceOVF = "VMWARE_SOURCE_OVF"
	// EnvSourceOVA is the .ova used by the clone rows.
	EnvSourceOVA = "VMWARE_SOURCE_OVA"
	// EnvSourceSnapshot is the snapshot of the .vmx source used by the clone rows.
	EnvSourceSnapshot = "VMWARE_SOURCE_SNAPSHOT"

	envPackerCacheDir = "PACKER_CACHE_DIR"
)

// Host is a verified hypervisor and the profile selected for it.
type Host struct {
	Profile   Profile
	Driver    vmwcommon.Driver
	VmrunPath string
}

var (
	hostOnce sync.Once
	host     *Host
	hostErr  string
)

// RequireAcceptance skips the test unless PACKER_ACC is set.
func RequireAcceptance(t *testing.T) {
	t.Helper()
	if os.Getenv(acctest.TestEnvVar) == "" {
		t.Skipf("Acceptance tests skipped unless env '%s' set", acctest.TestEnvVar)
	}
}

// RequireHypervisor skips the test unless PACKER_ACC is set, the host has a
// profile, and the hypervisor for that profile passes verification.
func RequireHypervisor(t *testing.T) *Host {
	t.Helper()
	RequireAcceptance(t)

	hostOnce.Do(func() {
		host, hostErr = newHost()
	})
	if host == nil {
		t.Skip(hostErr)
	}
	return host
}

// newHost selects the profile and verifies the driver for the host.
func newHost() (*Host, string) {
	profile, ok := CurrentProfile()
	if !ok {
		return nil, "no acceptance profile for this host"
	}

	dconfig := &vmwcommon.DriverConfig{}
	if errs := dconfig.Prepare(&interpolate.Context{}); len(errs) > 0 {
		return nil, fmt.Sprintf("preparing driver config: %v", errs)
	}

	driver, err := vmwcommon.NewDriver(dconfig, &vmwcommon.SSHConfig{}, "")
	if err != nil {
		return nil, fmt.Sprintf("%s is not available: %s", profile.Hypervisor, err)
	}

	var vmrunPath string
	switch d := driver.(type) {
	case *vmwcommon.FusionDriver:
		vmrunPath = filepath.Join(d.AppPath, "Contents", "Library", "vmrun")
	case *vmwcommon.WorkstationDriver:
		vmrunPath = d.VmrunPath
	default:
		return nil, fmt.Sprintf("unsupported driver %T", driver)
	}

	// Share one download of the installation media across the rows and runs.
	if os.Getenv(envPackerCacheDir) == "" {
		if dir, err := os.UserCacheDir(); err == nil {
			_ = os.Setenv(envPackerCacheDir, filepath.Join(dir, "packer-plugin-vmware", "acceptance"))
		}
	}

	return &Host{Profile: profile, Driver: driver, VmrunPath: vmrunPath}, ""
}

// RequireOvfTool skips the test unless VMware OVF Tool is on the PATH and
// runs on this host. An x86_64 build on Apple silicon without Rosetta 2, for
// example, is found but cannot run.
func RequireOvfTool(t *testing.T) {
	t.Helper()
	ovftool := vmwcommon.GetOvfTool()
	if ovftool == "" {
		t.Skip("ovftool not found on PATH")
	}
	if out, err := exec.Command(ovftool, "--version").CombinedOutput(); err != nil { //nolint:gosec
		t.Skipf("ovftool found on PATH but failed to run: %s %s", err, strings.TrimSpace(string(out)))
	}
}

// RequireEnv skips the test unless the environment variable is set and
// returns its value.
func RequireEnv(t *testing.T, name string) string {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		t.Skipf("%s not set", name)
	}
	return v
}

// RequireToolsISO skips the test unless the hypervisor's VMware Tools ISO
// for the flavor is in its default location and returns its path.
func (h *Host) RequireToolsISO(t *testing.T, flavor string) string {
	t.Helper()
	path := h.Driver.ToolsIsoPath(flavor)
	if path == "" {
		t.Skipf("VMware Tools ISO for %s not found", flavor)
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("VMware Tools ISO for %s not found at %s", flavor, path)
	}
	return path
}

// Vmrun runs vmrun with the host type and returns the combined output.
func (h *Host) Vmrun(args ...string) (string, error) {
	args = append([]string{"-T", h.Profile.VmrunType}, args...)
	out, err := exec.Command(h.VmrunPath, args...).CombinedOutput() //nolint:gosec
	if err != nil {
		return string(out), fmt.Errorf("vmrun %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// Build runs a Packer build of the template and then runs the check against
// the result. The workspace is torn down when the test ends.
func Build(t *testing.T, ws *Workspace, name, builderType, template string, check func() error) {
	t.Helper()
	acctest.TestPlugin(t, &acctest.PluginTestCase{
		Name:     name,
		Type:     builderType,
		Template: template,
		Check: func(buildCommand *exec.Cmd, _ string) error {
			if err := CheckExitCode(buildCommand); err != nil {
				return err
			}
			if check == nil {
				return nil
			}
			return check()
		},
		Teardown: ws.Teardown,
	})
}
