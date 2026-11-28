// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/vmware/packer-plugin-vmware/testing/acceptance"
)

const (
	accVMName       = "packer-acc-iso"
	accSnapshotName = "packer-acc-snapshot"
)

type isoRow struct {
	name  string
	setup func(t *testing.T, h *acceptance.Host, c *acceptance.ISOConfig)
	check func(h *acceptance.Host, ws *acceptance.Workspace, vmxPath string) error
}

func TestAccISOBuilder_Matrix(t *testing.T) {
	rows := []isoRow{
		{
			name: "A_vmx",
			setup: func(_ *testing.T, _ *acceptance.Host, c *acceptance.ISOConfig) {
				c.Format = "vmx"
			},
		},
		{
			name: "B_snapshot",
			setup: func(_ *testing.T, _ *acceptance.Host, c *acceptance.ISOConfig) {
				c.SnapshotName = accSnapshotName
			},
			check: func(h *acceptance.Host, _ *acceptance.Workspace, vmxPath string) error {
				snapshots, err := h.ListSnapshots(vmxPath)
				if err != nil {
					return err
				}
				if !slices.Contains(snapshots, accSnapshotName) {
					return fmt.Errorf("snapshot %q not in %v", accSnapshotName, snapshots)
				}
				return nil
			},
		},
		{
			name: "C_ovf",
			setup: func(t *testing.T, _ *acceptance.Host, c *acceptance.ISOConfig) {
				acceptance.RequireOvfTool(t)
				c.Format = "ovf"
			},
			check: func(_ *acceptance.Host, ws *acceptance.Workspace, _ string) error {
				return acceptance.CheckFileExists(filepath.Join(ws.OutputDir, accVMName+".ovf"))
			},
		},
		{
			name: "D_ova",
			setup: func(t *testing.T, _ *acceptance.Host, c *acceptance.ISOConfig) {
				acceptance.RequireOvfTool(t)
				c.Format = "ova"
			},
			check: func(_ *acceptance.Host, ws *acceptance.Workspace, _ string) error {
				return acceptance.CheckFileExists(filepath.Join(ws.OutputDir, accVMName+".ova"))
			},
		},
		{
			name: "E_tools_upload",
			setup: func(t *testing.T, h *acceptance.Host, c *acceptance.ISOConfig) {
				h.RequireToolsISO(t, "linux")
				c.ToolsMode = "upload"
				c.ToolsUploadFlavor = "linux"
			},
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			acceptance.RequireAcceptance(t)
			h := acceptance.RequireHypervisor(t)

			c := acceptance.ISOConfig{
				Profile: h.Profile,
				VMName:  accVMName,
				Marker:  acceptance.NewMarker(),
			}
			row.setup(t, h, &c)

			ws := acceptance.NewWorkspace(t, h)
			c.OutputDir = ws.OutputDir
			c.MarkerPath = ws.MarkerPath

			template, err := acceptance.ISOTemplate(c)
			if err != nil {
				t.Fatal(err)
			}

			acceptance.Build(t, ws, "vmware-iso_acc_"+row.name, "vmware-iso", template, func() error {
				vmxPath, err := acceptance.FindArtifactVMX(ws.OutputDir)
				if err != nil {
					return err
				}
				if err := acceptance.CheckVMX(vmxPath, acceptance.ExpectProfile(h.Profile)); err != nil {
					return err
				}
				if err := acceptance.CheckDiskExists(vmxPath); err != nil {
					return err
				}
				if err := acceptance.CheckMarker(ws.MarkerPath, c.Marker); err != nil {
					return err
				}
				if row.check != nil {
					return row.check(h, ws, vmxPath)
				}
				return nil
			})
		})
	}
}
