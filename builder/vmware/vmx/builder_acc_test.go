// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: MPL-2.0

package vmx

import (
	"errors"
	"fmt"
	"testing"

	"github.com/vmware/packer-plugin-vmware/testing/acceptance"
)

const accVMName = "packer-acc-vmx"

type vmxRow struct {
	name string
	// setup applies the row's skips and settings and returns the source .vmx
	// to compare against, if any.
	setup func(t *testing.T, h *acceptance.Host, c *acceptance.VMXConfig) string
	check func(vmxPath string) error
}

func TestAccVMXBuilder_Matrix(t *testing.T) {
	rows := []vmxRow{
		{
			name: "A_full_clone",
			setup: func(t *testing.T, _ *acceptance.Host, c *acceptance.VMXConfig) string {
				c.SourcePath = acceptance.RequireEnv(t, acceptance.EnvSourceVMX)
				c.Format = "vmx"
				return c.SourcePath
			},
		},
		{
			name: "B_linked_clone",
			setup: func(t *testing.T, _ *acceptance.Host, c *acceptance.VMXConfig) string {
				c.SourcePath = acceptance.RequireEnv(t, acceptance.EnvSourceVMX)
				// A linked clone without a snapshot adds one to the source.
				c.AttachSnapshot = acceptance.RequireEnv(t, acceptance.EnvSourceSnapshot)
				c.Linked = true
				return c.SourcePath
			},
			check: checkLinkedClone,
		},
		{
			name: "C_ovf",
			setup: func(t *testing.T, h *acceptance.Host, c *acceptance.VMXConfig) string {
				c.SourcePath = acceptance.RequireEnv(t, acceptance.EnvSourceOVF)
				acceptance.RequireOvfTool(t)
				c.GuestOSType = h.Profile.GuestOS
				return ""
			},
		},
		{
			name: "D_ova",
			setup: func(t *testing.T, h *acceptance.Host, c *acceptance.VMXConfig) string {
				c.SourcePath = acceptance.RequireEnv(t, acceptance.EnvSourceOVA)
				acceptance.RequireOvfTool(t)
				c.GuestOSType = h.Profile.GuestOS
				return ""
			},
		},
		{
			name: "E_attach_snapshot",
			setup: func(t *testing.T, _ *acceptance.Host, c *acceptance.VMXConfig) string {
				c.SourcePath = acceptance.RequireEnv(t, acceptance.EnvSourceVMX)
				c.AttachSnapshot = acceptance.RequireEnv(t, acceptance.EnvSourceSnapshot)
				return c.SourcePath
			},
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			acceptance.RequireAcceptance(t)
			h := acceptance.RequireHypervisor(t)

			c := acceptance.VMXConfig{
				VMName:   accVMName,
				Headless: h.Profile.Headless,
				Marker:   acceptance.NewMarker(),
			}
			sourceVMX := row.setup(t, h, &c)

			// A clone of a .vmx keeps the source hardware. A clone of an
			// .ovf or .ova is remapped by ovftool, so only the guest
			// operating system is checked.
			want := acceptance.Expect{GuestOS: c.GuestOSType}
			var before map[string]string
			if sourceVMX != "" {
				var err error
				if want, err = acceptance.ExpectSource(sourceVMX); err != nil {
					t.Fatalf("reading source: %s", err)
				}
				if before, err = acceptance.Fingerprint(sourceVMX); err != nil {
					t.Fatalf("fingerprinting source: %s", err)
				}
			}

			ws := acceptance.NewWorkspace(t, h)
			c.OutputDir = ws.OutputDir
			c.MarkerPath = ws.MarkerPath

			template, err := acceptance.VMXTemplate(c)
			if err != nil {
				t.Fatal(err)
			}

			acceptance.Build(t, ws, "vmware-vmx_acc_"+row.name, "vmware-vmx", template, func() error {
				vmxPath, err := acceptance.FindArtifactVMX(ws.OutputDir)
				if err != nil {
					return err
				}
				if err := acceptance.CheckVMX(vmxPath, want); err != nil {
					return err
				}
				if err := acceptance.CheckDiskExists(vmxPath); err != nil {
					return err
				}
				if err := acceptance.CheckMarker(ws.MarkerPath, c.Marker); err != nil {
					return err
				}
				if sourceVMX != "" {
					after, err := acceptance.Fingerprint(sourceVMX)
					if err != nil {
						return fmt.Errorf("fingerprinting source: %w", err)
					}
					if err := acceptance.CompareFingerprints(before, after); err != nil {
						return err
					}
				}
				if row.check != nil {
					return row.check(vmxPath)
				}
				return nil
			})
		})
	}
}

// checkLinkedClone returns an error unless every disk of the clone is a delta.
func checkLinkedClone(vmxPath string) error {
	disks, err := acceptance.Disks(vmxPath)
	if err != nil {
		return err
	}
	if len(disks) == 0 {
		return errors.New("linked clone has no disks")
	}
	for _, disk := range disks {
		delta, err := acceptance.IsDeltaDisk(disk)
		if err != nil {
			return err
		}
		if !delta {
			return fmt.Errorf("%s is not a delta disk", disk)
		}
	}
	return nil
}
