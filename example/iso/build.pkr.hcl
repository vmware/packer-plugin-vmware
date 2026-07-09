# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: MPL-2.0

packer {
  required_version = ">= 1.7.0"
  required_plugins {
    vmware = {
      version = ">= 2.0.0"
      source  = "github.com/vmware/vmware"
    }
  }
}

build {
  sources = ["source.vmware-iso.debian"]
}

source "vmware-iso" "debian" {
  iso_url = "file:///Users/branson/Downloads/debian-12.14.0-arm64-DVD-1.iso"
  iso_checksum = "sha256:80f3bfccaf88ff6221c7970a7dd26fb1357588619edaeca5351d029f9d8ae4d2"

  guest_os_type = "arm-debian12-64"
  cpus          = 2
  memory        = 2048
  disk_size     = 20000
}
