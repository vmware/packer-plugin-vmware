// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: MPL-2.0

package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

func TestProfileFor(t *testing.T) {
	noEnv := func(string) string { return "" }

	cases := []struct {
		goos, goarch string
		name         string
		guestOS      string
		adapter      string
	}{
		{"darwin", "arm64", "fusion-arm64", "arm-debian-64", "e1000e"},
		{"darwin", "amd64", "fusion-amd64", "debian-64", "e1000"},
		{"linux", "amd64", "workstation-amd64", "debian-64", "e1000"},
		{"windows", "amd64", "workstation-amd64", "debian-64", "e1000"},
	}
	for _, tc := range cases {
		p, ok := profileFor(tc.goos, tc.goarch, noEnv)
		if !ok {
			t.Fatalf("%s/%s: no profile", tc.goos, tc.goarch)
		}
		if p.Name != tc.name || p.GuestOS != tc.guestOS || p.NetworkAdapter != tc.adapter {
			t.Errorf("%s/%s: got %s %s %s", tc.goos, tc.goarch, p.Name, p.GuestOS, p.NetworkAdapter)
		}
		if p.CdromAdapter != "sata" || p.DiskAdapter != "sata" {
			t.Errorf("%s/%s: adapters %s %s, want sata", tc.goos, tc.goarch, p.CdromAdapter, p.DiskAdapter)
		}
		if p.CPUs != 2 || p.Memory != 2048 || p.DiskSize != 20480 || p.BootWait != "10s" || !p.Headless {
			t.Errorf("%s/%s: defaults not applied: %+v", tc.goos, tc.goarch, p)
		}
		if len(p.ISOURLs) == 0 || !strings.HasPrefix(p.ISOChecksum, "sha256:") || len(p.BootCommand) == 0 {
			t.Errorf("%s/%s: installation media not pinned", tc.goos, tc.goarch)
		}
	}

	for _, unsupported := range [][2]string{{"linux", "arm64"}, {"windows", "arm64"}, {"freebsd", "amd64"}} {
		if _, ok := profileFor(unsupported[0], unsupported[1], noEnv); ok {
			t.Errorf("%s/%s: expected no profile", unsupported[0], unsupported[1])
		}
	}
}

func TestProfileFor_overrides(t *testing.T) {
	env := map[string]string{
		EnvISOURL:      "file:///isos/debian.iso",
		EnvISOChecksum: "sha256:0000",
		EnvHeadless:    "false",
	}
	p, ok := profileFor("darwin", "arm64", func(k string) string { return env[k] })
	if !ok {
		t.Fatal("no profile")
	}
	if len(p.ISOURLs) != 1 || p.ISOURLs[0] != env[EnvISOURL] {
		t.Errorf("ISOURLs = %v", p.ISOURLs)
	}
	if p.ISOChecksum != env[EnvISOChecksum] {
		t.Errorf("ISOChecksum = %s", p.ISOChecksum)
	}
	if p.Headless {
		t.Error("Headless = true, want false")
	}

	// An unparsable value keeps the default.
	env[EnvHeadless] = "maybe"
	if p, _ := profileFor("darwin", "arm64", func(k string) string { return env[k] }); !p.Headless {
		t.Error("Headless = false for an unparsable value, want true")
	}

	// The override must not change the shared profile.
	if fusionARM64.ISOURLs[0] == env[EnvISOURL] {
		t.Error("override changed the shared profile")
	}
}

func TestHCLString(t *testing.T) {
	for _, s := range []string{
		`plain`,
		`C:\Users\packer\output`,
		`quote " and backslash \`,
		"tab\tnewline\ncarriage\r",
		`${var.not_interpolated} and %{ if true }directive%{ endif }`,
		`{{ .HTTPIP }}:{{ .HTTPPort }}`,
		`$ and % alone`,
	} {
		expr, diags := hclsyntax.ParseExpression([]byte(hclString(s)), "test.hcl", hcl.InitialPos)
		if diags.HasErrors() {
			t.Fatalf("%q: %s", s, diags)
		}
		v, diags := expr.Value(nil)
		if diags.HasErrors() {
			t.Fatalf("%q: %s", s, diags)
		}
		if got := v.AsString(); got != s {
			t.Errorf("round trip: got %q, want %q", got, s)
		}
	}
}

func TestTemplates_parse(t *testing.T) {
	p, _ := profileFor("windows", "amd64", func(string) string { return "" })
	// Host paths use the native separator, which is a backslash on Windows.
	base := t.TempDir()
	output := filepath.Join(base, "output")
	marker := filepath.Join(base, "marker.txt")
	source := filepath.Join(base, "source", "debian.vmx")

	iso, err := ISOTemplate(ISOConfig{
		Profile:           p,
		VMName:            "acc",
		OutputDir:         output,
		MarkerPath:        marker,
		Marker:            NewMarker(),
		Format:            "ovf",
		SnapshotName:      "acc-snapshot",
		ToolsMode:         "upload",
		ToolsUploadFlavor: "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	vmx, err := VMXTemplate(VMXConfig{
		VMName:         "acc",
		SourcePath:     source,
		OutputDir:      output,
		MarkerPath:     marker,
		Marker:         NewMarker(),
		Headless:       true,
		Linked:         true,
		AttachSnapshot: "base",
		GuestOSType:    "debian-64",
	})
	if err != nil {
		t.Fatal(err)
	}

	for name, src := range map[string]string{"iso.pkr.hcl": iso, "vmx.pkr.hcl": vmx} {
		if _, diags := hclparse.NewParser().ParseHCL([]byte(src), name); diags.HasErrors() {
			t.Fatalf("%s: %s\n%s", name, diags, src)
		}
		if strings.Contains(src, `\`) {
			t.Errorf("%s: contains a backslash from a host path:\n%s", name, src)
		}
		if !strings.Contains(src, hclString(filepath.ToSlash(output))) {
			t.Errorf("%s: output directory not converted to forward slashes:\n%s", name, src)
		}
		if strings.Contains(src, "[[") {
			t.Errorf("%s: unrendered template action:\n%s", name, src)
		}
	}

	for _, want := range []string{
		`iso_checksum         = "sha256:`,
		`headless             = true`,
		`format = "ovf"`,
		`snapshot_name = "acc-snapshot"`,
		`tools_upload_flavor = "linux"`,
		`{{ .HTTPIP }}`,
		`preseed.pkrtpl.hcl`,
	} {
		if !strings.Contains(iso, want) {
			t.Errorf("iso template missing %q:\n%s", want, iso)
		}
	}
	for _, want := range []string{
		`source_path      = ` + hclString(filepath.ToSlash(source)),
		`headless         = true`,
		`linked           = true`,
		`attach_snapshot = "base"`,
		`guest_os_type = "debian-64"`,
	} {
		if !strings.Contains(vmx, want) {
			t.Errorf("vmx template missing %q:\n%s", want, vmx)
		}
	}
}

func TestPreseedPath(t *testing.T) {
	path, err := preseedPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("preseed not found: %s", err)
	}
}

func TestTeardown_missing(t *testing.T) {
	base := filepath.Join(t.TempDir(), workspacePrefix+"missing")
	ws := &Workspace{Base: base, OutputDir: filepath.Join(base, "output")}
	if err := ws.Teardown(); err != nil {
		t.Fatalf("teardown of a missing workspace: %s", err)
	}
	if err := ws.Teardown(); err != nil {
		t.Fatalf("second teardown: %s", err)
	}
}

func TestTeardown_removes(t *testing.T) {
	base := filepath.Join(t.TempDir(), workspacePrefix+"present")
	output := filepath.Join(base, "output", "acc.vmwarevm")
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "acc.vmx"), []byte(`guestos = "debian-64"`), 0o644); err != nil {
		t.Fatal(err)
	}

	ws := &Workspace{Base: base, OutputDir: filepath.Join(base, "output")}
	if err := ws.Teardown(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists: %v", err)
	}
}

func TestTeardown_guard(t *testing.T) {
	keep := t.TempDir()
	for _, base := range []string{"", "relative/" + workspacePrefix + "x", keep} {
		ws := &Workspace{Base: base, OutputDir: filepath.Join(base, "output")}
		if err := ws.Teardown(); err == nil {
			t.Errorf("teardown of %q should be refused", base)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("refused directory was removed: %s", err)
	}
}

func TestWithin(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "tmp", workspacePrefix+"1")
	cases := map[string]bool{
		filepath.Join(base, "output", "acc.vmx"):        true,
		filepath.Join(base, "..", "source", "src.vmx"):  false,
		filepath.Join(base+"-other", "output", "a.vmx"): false,
		filepath.Join(string(filepath.Separator), "vm"): false,
	}
	for path, want := range cases {
		if got := within(base, path); got != want {
			t.Errorf("within(%s, %s) = %t, want %t", base, path, got, want)
		}
	}
}

const testArtifactVMX = `
guestos = "arm-debian-64"
numvcpus = "2"
memsize = "2048"
ethernet0.virtualDev = "e1000e"
sata0:0.present = "TRUE"
sata0:0.fileName = "disk.vmdk"
sata0:1.present = "TRUE"
sata0:1.deviceType = "cdrom-raw"
sata0:1.fileName = "auto detect"
remotedisplay.vnc.enabled = "FALSE"
`

func TestCheckVMX(t *testing.T) {
	p, _ := profileFor("darwin", "arm64", func(string) string { return "" })
	dir := t.TempDir()
	vmxPath := filepath.Join(dir, "acc.vmx")

	if err := os.WriteFile(vmxPath, []byte(testArtifactVMX), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckVMX(vmxPath, ExpectProfile(p)); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	if err := CheckDiskExists(vmxPath); err == nil {
		t.Error("expected an error for a missing disk")
	}
	if err := os.WriteFile(filepath.Join(dir, "disk.vmdk"), []byte("# Disk DescriptorFile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckDiskExists(vmxPath); err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	bad := testArtifactVMX + "remotedisplay.vnc.port = \"5900\"\nmemsize = \"1024\"\n"
	if err := os.WriteFile(vmxPath, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	err := CheckVMX(vmxPath, ExpectProfile(p))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"remotedisplay.vnc.port", "memsize"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}

	// Values not set in Expect are not checked.
	if err := CheckVMX(vmxPath, Expect{GuestOS: p.GuestOS}); err == nil || strings.Contains(err.Error(), "memsize") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestIsDeltaDisk(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]bool{
		"delta.vmdk": true,
		"base.vmdk":  false,
	}
	content := map[string]string{
		"delta.vmdk": "# Disk DescriptorFile\r\nversion=1\r\nparentCID=fffffffe\r\nparentFileNameHint=\"/vms/source/disk-000001.vmdk\"\r\n",
		"base.vmdk":  "# Disk DescriptorFile\nversion=1\nparentCID=ffffffff\ncreateType=\"monolithicSparse\"\n",
	}
	for name, want := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content[name]), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := IsDeltaDisk(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("IsDeltaDisk(%s) = %t, want %t", name, got, want)
		}
	}
}

func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	vmxPath := filepath.Join(dir, "source.vmx")
	files := map[string]string{
		"source.vmx":  `guestos = "debian-64"`,
		"disk.vmdk":   "# Disk DescriptorFile\n",
		"source.vmsd": "",
		"vmware.log":  "",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	before, err := Fingerprint(vmxPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 2 {
		t.Fatalf("expected the .vmx and .vmdk only, got %v", before)
	}

	// Snapshot metadata and logs are ignored.
	if err := os.WriteFile(filepath.Join(dir, "source.vmsd"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := Fingerprint(vmxPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := CompareFingerprints(before, after); err != nil {
		t.Fatalf("unexpected change: %s", err)
	}

	if err := os.WriteFile(vmxPath, []byte(`guestos = "other"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "disk-000001.vmdk"), []byte("# Disk DescriptorFile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err = Fingerprint(vmxPath)
	if err != nil {
		t.Fatal(err)
	}
	err = CompareFingerprints(before, after)
	if err == nil {
		t.Fatal("expected a change")
	}
	for _, want := range []string{"source.vmx changed", "disk-000001.vmdk was added"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestParseSnapshots(t *testing.T) {
	got := parseSnapshots("Total snapshots: 2\nbase\nacc-snapshot\n")
	if len(got) != 2 || got[0] != "base" || got[1] != "acc-snapshot" {
		t.Errorf("parseSnapshots = %v", got)
	}
}

func TestRequireAcceptance(t *testing.T) {
	t.Setenv("PACKER_ACC", "")
	ran := false
	t.Run("skip", func(t *testing.T) {
		RequireHypervisor(t)
		ran = true
	})
	if ran {
		t.Error("RequireHypervisor did not skip with PACKER_ACC unset")
	}
}
