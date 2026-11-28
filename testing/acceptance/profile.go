// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: MPL-2.0

package acceptance

import (
	"os"
	"runtime"
	"strconv"
)

const (
	// EnvISOURL overrides the pinned installation media URL for the host profile.
	EnvISOURL = "VMWARE_ISO_URL"
	// EnvISOChecksum overrides the pinned installation media checksum for the host profile.
	EnvISOChecksum = "VMWARE_ISO_CHECKSUM"
	// EnvHeadless set to false shows the virtual machines in the hypervisor
	// while the rows run.
	EnvHeadless = "VMWARE_HEADLESS"

	hypervisorFusion      = "VMware Fusion Pro"
	hypervisorWorkstation = "VMware Workstation Pro"

	vmrunTypeFusion      = "fusion"
	vmrunTypeWorkstation = "ws"
)

// Profile describes the guest built on a host operating system and architecture.
type Profile struct {
	Name           string
	Hypervisor     string
	VmrunType      string
	GuestOS        string
	NetworkAdapter string
	CdromAdapter   string
	DiskAdapter    string
	CPUs           int
	Memory         int
	DiskSize       int
	Headless       bool
	BootWait       string
	BootCommand    []string
	ISOURLs        []string
	ISOChecksum    string
}

const (
	debianAMD64Checksum = "sha256:a7ef94ac2fb9a7fec454552abd629b7cc9d5155c886165a45649f5ce6167e355"
	debianARM64Checksum = "sha256:6e93fa1759bd9d4b0fc11e938987de6967ee7de5297dac1be27c3a75cc17024b"
)

var (
	// The Debian netinst images, listed with the archive as a fallback for
	// when the next point release moves them out of current.
	debianAMD64URLs = []string{
		"https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-13.7.0-amd64-netinst.iso",
		"https://cdimage.debian.org/cdimage/archive/13.7.0/amd64/iso-cd/debian-13.7.0-amd64-netinst.iso",
	}
	debianARM64URLs = []string{
		"https://cdimage.debian.org/debian-cd/current/arm64/iso-cd/debian-13.7.0-arm64-netinst.iso",
		"https://cdimage.debian.org/cdimage/archive/13.7.0/arm64/iso-cd/debian-13.7.0-arm64-netinst.iso",
	}

	// The boot commands match example/iso/pkrvars/debian.
	debianBIOSBootCommand = []string{
		"<wait><esc><wait>auto preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg netcfg/get_hostname={{ .Name }}<enter>",
	}
	debianEFIBootCommand = []string{
		"<wait><up>e<wait><down><down><down><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><wait>install <wait> preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg <wait>debian-installer=en_US.UTF-8 <wait>auto <wait>locale=en_US.UTF-8 <wait>kbd-chooser/method=us <wait>keyboard-configuration/xkb-keymap=us <wait>netcfg/get_hostname={{ .Name }} <wait>netcfg/get_domain={{ .Name }} <wait>fb=false <wait>debconf/frontend=noninteractive <wait>console-setup/ask_detect=false <wait>console-keymaps-at/keymap=us <wait>grub-installer/bootdev=/dev/sda <wait><f10><wait>",
	}
)

var (
	fusionARM64 = Profile{
		Name:           "fusion-arm64",
		Hypervisor:     hypervisorFusion,
		VmrunType:      vmrunTypeFusion,
		GuestOS:        "arm-debian-64",
		NetworkAdapter: "e1000e",
		CdromAdapter:   "sata",
		DiskAdapter:    "sata",
		BootCommand:    debianEFIBootCommand,
		ISOURLs:        debianARM64URLs,
		ISOChecksum:    debianARM64Checksum,
	}
	fusionAMD64 = Profile{
		Name:           "fusion-amd64",
		Hypervisor:     hypervisorFusion,
		VmrunType:      vmrunTypeFusion,
		GuestOS:        "debian-64",
		NetworkAdapter: "e1000",
		CdromAdapter:   "sata",
		DiskAdapter:    "sata",
		BootCommand:    debianBIOSBootCommand,
		ISOURLs:        debianAMD64URLs,
		ISOChecksum:    debianAMD64Checksum,
	}
	workstationAMD64 = Profile{
		Name:           "workstation-amd64",
		Hypervisor:     hypervisorWorkstation,
		VmrunType:      vmrunTypeWorkstation,
		GuestOS:        "debian-64",
		NetworkAdapter: "e1000",
		CdromAdapter:   "sata",
		DiskAdapter:    "sata",
		BootCommand:    debianBIOSBootCommand,
		ISOURLs:        debianAMD64URLs,
		ISOChecksum:    debianAMD64Checksum,
	}

	// The profiles keyed by GOOS/GOARCH, following the driver selection in
	// common.NewDriver.
	profiles = map[string]Profile{
		"darwin/arm64":  fusionARM64,
		"darwin/amd64":  fusionAMD64,
		"linux/amd64":   workstationAMD64,
		"windows/amd64": workstationAMD64,
	}
)

// profileFor returns the profile for an operating system and architecture,
// with the defaults and environment overrides applied.
func profileFor(goos, goarch string, getenv func(string) string) (Profile, bool) {
	p, ok := profiles[goos+"/"+goarch]
	if !ok {
		return Profile{}, false
	}

	// The defaults match example/iso/variables.pkr.hcl.
	p.CPUs = 2
	p.Memory = 2048
	p.DiskSize = 20480
	p.BootWait = "10s"
	p.Headless = true
	if v, err := strconv.ParseBool(getenv(EnvHeadless)); err == nil {
		p.Headless = v
	}

	p.BootCommand = append([]string(nil), p.BootCommand...)
	p.ISOURLs = append([]string(nil), p.ISOURLs...)
	if v := getenv(EnvISOURL); v != "" {
		p.ISOURLs = []string{v}
	}
	if v := getenv(EnvISOChecksum); v != "" {
		p.ISOChecksum = v
	}

	return p, true
}

// CurrentProfile returns the profile for the host running the tests.
func CurrentProfile() (Profile, bool) {
	return profileFor(runtime.GOOS, runtime.GOARCH, os.Getenv)
}
