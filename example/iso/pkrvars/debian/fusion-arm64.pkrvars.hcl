# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: MPL-2.0

cdrom_adapter_type   = "sata"
disk_adapter_type    = "sata"
network_adapter_type = "e1000e"
guest_os_type        = "arm-debian-64"
boot_command         = ["<wait><up>e<wait><down><down><down><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><right><wait>install <wait> preseed/url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg <wait>debian-installer=en_US.UTF-8 <wait>auto <wait>locale=en_US.UTF-8 <wait>kbd-chooser/method=us <wait>keyboard-configuration/xkb-keymap=us <wait>netcfg/get_hostname={{ .Name }} <wait>netcfg/get_domain={{ .Name }} <wait>fb=false <wait>debconf/frontend=noninteractive <wait>console-setup/ask_detect=false <wait>console-keymaps-at/keymap=us <wait>grub-installer/bootdev=/dev/sda <wait><f10><wait>"]
vm_name              = "debian-arm64"
iso_urls = [
  "https://cdimage.debian.org/debian-cd/current/arm64/iso-cd/debian-13.7.0-arm64-netinst.iso",
  "https://cdimage.debian.org/cdimage/archive/13.7.0/arm64/iso-cd/debian-13.7.0-arm64-netinst.iso",
]
iso_checksum = "sha256:6e93fa1759bd9d4b0fc11e938987de6967ee7de5297dac1be27c3a75cc17024b"
