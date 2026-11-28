// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: MPL-2.0

package acceptance

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

const (
	// The credentials set by example/iso/data/debian/preseed.pkrtpl.hcl.
	buildUsername = "packer"
	buildPassword = "packer"

	sshTimeout      = "45m"
	guestMarkerPath = "/tmp/packer-acc-marker"
	toolsModeOff    = "disable"
)

// ISOConfig is the input for a vmware-iso template.
type ISOConfig struct {
	Profile           Profile
	VMName            string
	OutputDir         string
	MarkerPath        string
	Marker            string
	Format            string
	SnapshotName      string
	ToolsMode         string
	ToolsUploadFlavor string
}

// VMXConfig is the input for a vmware-vmx template.
type VMXConfig struct {
	VMName         string
	Headless       bool
	SourcePath     string
	OutputDir      string
	MarkerPath     string
	Marker         string
	Format         string
	Linked         bool
	AttachSnapshot string
	GuestOSType    string
}

// NewMarker returns a unique value for the shell provisioner to write.
func NewMarker() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "packer-acc-" + hex.EncodeToString(b)
}

// ISOTemplate renders an HCL2 template for the vmware-iso builder.
func ISOTemplate(c ISOConfig) (string, error) {
	preseed, err := preseedPath()
	if err != nil {
		return "", err
	}
	if c.ToolsMode == "" {
		c.ToolsMode = toolsModeOff
	}
	return render(isoTemplate, map[string]any{
		"C":        c,
		"P":        c.Profile,
		"Preseed":  filepath.ToSlash(preseed),
		"Username": buildUsername,
		"Password": buildPassword,
		"Timeout":  sshTimeout,
		"Build":    buildBlock(c.Marker, c.MarkerPath),
	})
}

// VMXTemplate renders an HCL2 template for the vmware-vmx builder.
func VMXTemplate(c VMXConfig) (string, error) {
	return render(vmxTemplate, map[string]any{
		"C":        c,
		"Username": buildUsername,
		"Password": buildPassword,
		"Timeout":  sshTimeout,
		"Build":    buildBlock(c.Marker, c.MarkerPath),
	})
}

// preseedPath returns the absolute path of the Debian preseed template in
// example/iso, found relative to this source file.
func preseedPath() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to locate the acceptance package source")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "example", "iso", "data", "debian", "preseed.pkrtpl.hcl"), nil
}

// buildBlock returns the values for the build block shared by the templates.
func buildBlock(marker, markerPath string) map[string]string {
	return map[string]string{
		"Marker":      marker,
		"GuestMarker": guestMarkerPath,
		"HostMarker":  filepath.ToSlash(markerPath),
	}
}

// The templates use [[ ]] delimiters so that Packer's {{ }} pass through.
const isoTemplate = `
source "vmware-iso" "acc" {
  vm_name              = [[ q .C.VMName ]]
  guest_os_type        = [[ q .P.GuestOS ]]
  headless             = [[ .P.Headless ]]
  cpus                 = [[ .P.CPUs ]]
  memory               = [[ .P.Memory ]]
  disk_size            = [[ .P.DiskSize ]]
  network_adapter_type = [[ q .P.NetworkAdapter ]]
  cdrom_adapter_type   = [[ q .P.CdromAdapter ]]
  disk_adapter_type    = [[ q .P.DiskAdapter ]]
  iso_urls             = [[ list .P.ISOURLs ]]
  iso_checksum         = [[ q .P.ISOChecksum ]]
  http_content = {
    "/preseed.cfg" = templatefile([[ q .Preseed ]], {
      build_username       = [[ q .Username ]]
      build_password       = [[ q .Password ]]
      vm_guest_os_language = "en"
      vm_guest_os_keyboard = "us"
      vm_guest_os_timezone = "UTC"
    })
  }
  boot_wait        = [[ q .P.BootWait ]]
  boot_command     = [[ list .P.BootCommand ]]
  ssh_username     = [[ q .Username ]]
  ssh_password     = [[ q .Password ]]
  ssh_timeout      = [[ q .Timeout ]]
  shutdown_command = [[ q (printf "echo '%s' | sudo -S -E shutdown -P now" .Password) ]]
  output_directory = [[ q (slash .C.OutputDir) ]]
  tools_mode       = [[ q .C.ToolsMode ]]
[[- if .C.ToolsUploadFlavor ]]
  tools_upload_flavor = [[ q .C.ToolsUploadFlavor ]]
[[- end ]]
[[- if .C.Format ]]
  format = [[ q .C.Format ]]
[[- end ]]
[[- if .C.SnapshotName ]]
  snapshot_name = [[ q .C.SnapshotName ]]
[[- end ]]
}

build {
  sources = ["source.vmware-iso.acc"]
[[ template "provisioners" .Build ]]
}
`

const vmxTemplate = `
source "vmware-vmx" "acc" {
  vm_name          = [[ q .C.VMName ]]
  source_path      = [[ q (slash .C.SourcePath) ]]
  headless         = [[ .C.Headless ]]
  linked           = [[ .C.Linked ]]
  ssh_username     = [[ q .Username ]]
  ssh_password     = [[ q .Password ]]
  ssh_timeout      = [[ q .Timeout ]]
  shutdown_command = [[ q (printf "echo '%s' | sudo -S -E shutdown -P now" .Password) ]]
  output_directory = [[ q (slash .C.OutputDir) ]]
[[- if .C.AttachSnapshot ]]
  attach_snapshot = [[ q .C.AttachSnapshot ]]
[[- end ]]
[[- if .C.GuestOSType ]]
  guest_os_type = [[ q .C.GuestOSType ]]
[[- end ]]
[[- if .C.Format ]]
  format = [[ q .C.Format ]]
[[- end ]]
}

build {
  sources = ["source.vmware-vmx.acc"]
[[ template "provisioners" .Build ]]
}
`

// The shell provisioner proves SSH works by writing a marker in the guest,
// and the file provisioner downloads it to the host for the check.
const provisionersTemplate = `[[ define "provisioners" ]]
  provisioner "shell" {
    inline = [[ list (marker .Marker .GuestMarker) ]]
  }

  provisioner "file" {
    direction   = "download"
    source      = [[ q .GuestMarker ]]
    destination = [[ q .HostMarker ]]
  }
[[- end ]]`

func render(text string, data any) (string, error) {
	tmpl, err := template.New("acc").
		Delims("[[", "]]").
		Funcs(template.FuncMap{
			"q":     hclString,
			"list":  hclList,
			"slash": filepath.ToSlash,
			"marker": func(marker, path string) []string {
				return []string{fmt.Sprintf("echo '%s' > %s", marker, path)}
			},
		}).
		Parse(text + provisionersTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()) + "\n", nil
}

// hclString quotes a value as an HCL string literal, escaping the template
// sequences so the value is used as is.
func hclString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '"':
			b.WriteString(`\"`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\u%04x`, c)
		case (c == '$' || c == '%') && i+1 < len(s) && s[i+1] == '{':
			b.WriteByte(c)
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// hclList quotes each value and returns an HCL list.
func hclList(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = hclString(v)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
