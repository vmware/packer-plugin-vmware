# Acceptance Tests

These tests build virtual machines with the `vmware-iso` and `vmware-vmx` builders on a machine
running VMware Fusion Pro or VMware Workstation Pro. The harness lives in [`testing/acceptance`](../testing/acceptance).
Run them with the targets below. `make test`, `task test`, and the GitHub Actions unit-test workflow
don't set `PACKER_ACC`, so they skip this suite.

A row skips when `PACKER_ACC` isn't set, when the host doesn't have a profile in the table below, or
when that hypervisor fails verification.

## Prerequisites

- VMware Fusion Pro on macOS, or VMware Workstation Pro on Linux or Windows.
- [Packer](https://developer.hashicorp.com/packer/install) on the `PATH`. The targets install the
  development plugin before the tests run, so Packer has to be available even on a machine where
  every row skips.
- [VMware OVF Tool](https://developer.broadcom.com/tools/open-virtualization-format-ovf-tool/latest)
  on the `PATH` for the `.ovf` and `.ova` rows. On Apple silicon, the `x86_64` build of OVF Tool
  needs Rosetta 2.
- A network path to `cdimage.debian.org` for the first download, or `VMWARE_ISO_URL` set to a local image.

The templates don't declare `required_plugins`, so Packer uses the newest installed copy of this
plugin. If `packer plugins installed` shows a release newer than the development build, remove that
release first.

## Host Profiles

The host operating system and architecture pick the profile. Any other combination skips.

| Profile              | Host                           | Guest           | Adapters                         | Install Media               |
| ------------------- | ------------------------------ | --------------- | -------------------------------- | --------------------------- |
| `fusion-arm64`      | `darwin/arm64`                 | `arm-debian-64` | SATA CD-ROM, SATA disk, `e1000e` | Debian 13.7.0 arm64 netinst |
| `fusion-amd64`      | `darwin/amd64`                 | `debian-64`     | SATA CD-ROM, SATA disk, `e1000`  | Debian 13.7.0 amd64 netinst |
| `workstation-amd64` | `linux/amd64`, `windows/amd64` | `debian-64`     | SATA CD-ROM, SATA disk, `e1000`  | Debian 13.7.0 amd64 netinst |

Every profile gives the guest 2 CPUs, 2048 MB of memory, and a 20480 MB disk. The boot command comes
from [`example/iso`](../example/iso), and the installer uses
`example/iso/data/debian/preseed.pkrtpl.hcl`. Rows run headless, set `tools_mode` to `disable`
unless a row says otherwise, and wait up to 45 minutes for SSH.

Set `VMWARE_HEADLESS=false` if you want the console open in Fusion or Workstation. Don't type in
that console while the boot command is being sent. The installer image is downloaded once and kept
in `PACKER_CACHE_DIR`. When that variable isn't set, the harness uses
`packer-plugin-vmware/acceptance` in the user cache directory.

## Variables

| Variable                 | Purpose                                                    |
| ------------------------ | ---------------------------------------------------------- |
| `PACKER_ACC`             | Set to `1` by the targets.                                 |
| `VMWARE_ISO_URL`         | Overrides the image URL for the profile.                    |
| `VMWARE_ISO_CHECKSUM`    | Overrides the image checksum for the profile.               |
| `VMWARE_HEADLESS`        | Set to `false` to show the console. The default is `true`. |
| `VMWARE_SOURCE_VMX`      | Source `.vmx` for VMX rows A, B, and E.                    |
| `VMWARE_SOURCE_SNAPSHOT` | Snapshot of that `.vmx`, used by VMX rows B and E.         |
| `VMWARE_SOURCE_OVF`      | Source `.ovf` for VMX row C.                               |
| `VMWARE_SOURCE_OVA`      | Source `.ova` for VMX row D.                               |

## Targets

The same targets are available through `make` and `task`.

| Target                | Runs                                                                                     | Timeout |
| --------------------- | ---------------------------------------------------------------------------------------- | ------- |
| `testacc-builder-iso` | `TESTACC_BUILDER_ISO_RUN` in `./builder/vmware/iso` (default `TestAccISOBuilder_Matrix`) | 180m    |
| `testacc-builder-vmx` | `TESTACC_BUILDER_VMX_RUN` in `./builder/vmware/vmx` (default `TestAccVMXBuilder_Matrix`) | 60m     |
| `testacc-builder`     | The ISO target, then the VMX target                                                      |         |
| `testacc`             | `testacc-builder`                                                                        |         |

To run a single row:

```shell
make testacc-builder-iso TESTACC_BUILDER_ISO_RUN=TestAccISOBuilder_Matrix/A_vmx
```

## Rows

A passing build exits 0. The `.vmx` it writes has the profile's guest operating system, disk and
CD-ROM adapters, network adapter, CPU count, and memory. The VNC port added for the boot command is
removed, the disk file is present, and a shell provisioner has run over SSH.

A `.vmx` clone keeps the hardware of the source, so those rows compare the clone with the source
`.vmx`. `ovftool` rewrites the hardware of an `.ovf` or `.ova`, so those rows check only the guest
operating system.

### ISO

| Row              | Changes                                                              | Skips When                                               |
| ---------------- | -------------------------------------------------------------------- | -------------------------------------------------------- |
| `A_vmx`          | `format = "vmx"`                                                     |                                                          |
| `B_snapshot`     | Sets `snapshot_name`. `vmrun listSnapshots` must list that snapshot. |                                                          |
| `C_ovf`          | `format = "ovf"`. The `.ovf` must exist.                             | `ovftool` is missing                                     |
| `D_ova`          | `format = "ova"`. The `.ova` must exist.                             | `ovftool` is missing                                     |
| `E_tools_upload` | `tools_mode = "upload"` and `tools_upload_flavor = "linux"`           | The Linux VMware Tools ISO isn't in its default location |

Fusion on Apple silicon doesn't ship a Linux VMware Tools ISO, so `E_tools_upload` skips there.

### VMX

| Row                 | Changes                                                                                                       | Skips When                                                |
| ------------------- | ------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| `A_full_clone`      | Full clone. The source `.vmx` and its disks are unchanged.                                                    | `VMWARE_SOURCE_VMX` isn't set                             |
| `B_linked_clone`    | `linked = true`, taken from `VMWARE_SOURCE_SNAPSHOT`. The clone disk is a delta, and the source is unchanged. | `VMWARE_SOURCE_VMX` or `VMWARE_SOURCE_SNAPSHOT` isn't set |
| `C_ovf`             | Clones `VMWARE_SOURCE_OVF` and sets `guest_os_type` from the profile.                                          | The path isn't set, or `ovftool` is missing               |
| `D_ova`             | Clones `VMWARE_SOURCE_OVA`. The check is the same as `C_ovf`.                                                 | The path isn't set, or `ovftool` is missing               |
| `E_attach_snapshot` | Attaches `VMWARE_SOURCE_SNAPSHOT` before powering on.                                                         | `VMWARE_SOURCE_VMX` or `VMWARE_SOURCE_SNAPSHOT` isn't set |

`B_linked_clone` has to name an existing snapshot. Without that name, `vmrun clone` creates a
snapshot on the source virtual machine and changes it.

## Clone Sources

Build the source on the same machine you'll test on. Power it off, then take the snapshot. On macOS,
`vmrun` is at `/Applications/VMware Fusion.app/Contents/Library/vmrun`. On Workstation, pass `-T ws`
and use that host's `pkrvars` file.

```shell
cd example/iso
packer init .
packer build -var headless=true -var output_directory="$HOME/packer-acc/debian" \
  -var-file=pkrvars/debian/fusion-arm64.pkrvars.hcl .

vmrun -T fusion snapshot "$HOME/packer-acc/debian/debian-arm64.vmx" base
ovftool "$HOME/packer-acc/debian/debian-arm64.vmx" "$HOME/packer-acc/debian.ovf"
ovftool "$HOME/packer-acc/debian/debian-arm64.vmx" "$HOME/packer-acc/debian.ova"

export VMWARE_SOURCE_VMX="$HOME/packer-acc/debian/debian-arm64.vmx"
export VMWARE_SOURCE_SNAPSHOT=base
export VMWARE_SOURCE_OVF="$HOME/packer-acc/debian.ovf"
export VMWARE_SOURCE_OVA="$HOME/packer-acc/debian.ova"
make testacc-builder-vmx
```

Each row builds in its own `packer-acc-*` directory under the system temporary directory. When the
row finishes, including after a failed build, the harness runs `vmrun stop` and `vmrun deleteVM`,
then removes that directory. It doesn't delete anything outside the directory, so the clone sources
stay put. A failed build also leaves `packer_log_*.txt` and the generated `vmware-*_acc_*.pkr.hcl`
in the package directory so you can see what Packer ran.
