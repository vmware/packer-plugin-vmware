// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: MPL-2.0

package acceptance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	vmwcommon "github.com/vmware/packer-plugin-vmware/builder/vmware/common"
)

// The largest .vmdk read as a descriptor. Descriptors are small text files,
// and sparse extents embed theirs within the first sectors.
const descriptorReadLimit = 64 * 1024

var (
	diskFilenameRe       = regexp.MustCompile(`^(scsi|sata|ide|nvme)[0-9]+:[0-9]+\.filename$`)
	parentFileNameHintRe = regexp.MustCompile(`(?m)^parentFileNameHint="([^"]+)"`)
)

// Expect lists the .vmx values a check requires. Empty fields are not
// checked.
type Expect struct {
	GuestOS        string
	NetworkAdapter string
	CdromAdapter   string
	DiskAdapter    string
	CPUs           string
	Memory         string
}

// ExpectProfile returns the .vmx values for an artifact built from the profile.
func ExpectProfile(p Profile) Expect {
	return Expect{
		GuestOS:        p.GuestOS,
		NetworkAdapter: p.NetworkAdapter,
		CdromAdapter:   p.CdromAdapter,
		DiskAdapter:    p.DiskAdapter,
		CPUs:           strconv.Itoa(p.CPUs),
		Memory:         strconv.Itoa(p.Memory),
	}
}

// ExpectSource returns the .vmx values for a clone of the source .vmx.
func ExpectSource(vmxPath string) (Expect, error) {
	data, err := vmwcommon.ReadVMX(vmxPath)
	if err != nil {
		return Expect{}, err
	}
	return Expect{
		GuestOS:        data["guestos"],
		NetworkAdapter: data["ethernet0.virtualdev"],
		CPUs:           data["numvcpus"],
		Memory:         data["memsize"],
	}, nil
}

// CheckExitCode returns an error unless the build exited with 0.
func CheckExitCode(cmd *exec.Cmd) error {
	if cmd.ProcessState == nil {
		return errors.New("build did not run")
	}
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		return fmt.Errorf("bad exit code: %d", code)
	}
	return nil
}

// FindArtifactVMX returns the single .vmx in the output directory.
func FindArtifactVMX(outputDir string) (string, error) {
	paths, err := findVMX(outputDir)
	if err != nil {
		return "", err
	}
	if len(paths) != 1 {
		return "", fmt.Errorf("expected one .vmx in %s, found %d", outputDir, len(paths))
	}
	return paths[0], nil
}

// CheckVMX checks the artifact .vmx against the expected values and checks
// that the VNC settings added for the boot command are gone.
func CheckVMX(vmxPath string, want Expect) error {
	data, err := vmwcommon.ReadVMX(vmxPath)
	if err != nil {
		return err
	}

	var errs []error
	equal := func(key, value string) {
		if value != "" && !strings.EqualFold(data[key], value) {
			errs = append(errs, fmt.Errorf("%s = %q, want %q", key, data[key], value))
		}
	}
	equal("guestos", want.GuestOS)
	equal("ethernet0.virtualdev", want.NetworkAdapter)
	equal("numvcpus", want.CPUs)
	equal("memsize", want.Memory)

	if want.DiskAdapter != "" {
		key := want.DiskAdapter + "0:0.filename"
		if !strings.EqualFold(filepath.Ext(data[key]), ".vmdk") {
			errs = append(errs, fmt.Errorf("%s = %q, want a .vmdk", key, data[key]))
		}
	}

	if want.CdromAdapter != "" {
		cdromRe := regexp.MustCompile(`^` + regexp.QuoteMeta(want.CdromAdapter) + `[0-9]+:[0-9]+\.devicetype$`)
		found := false
		for k, v := range data {
			if cdromRe.MatchString(k) && strings.HasPrefix(strings.ToLower(v), "cdrom-") {
				found = true
				break
			}
		}
		if !found {
			errs = append(errs, fmt.Errorf("no %s CD-ROM device", want.CdromAdapter))
		}
	}

	for _, key := range []string{"remotedisplay.vnc.port", "remotedisplay.vnc.ip", "remotedisplay.vnc.password"} {
		if _, ok := data[key]; ok {
			errs = append(errs, fmt.Errorf("%s is still set", key))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s: %w", vmxPath, errors.Join(errs...))
	}
	return nil
}

// Disks returns the paths of the .vmdk files attached in the .vmx.
func Disks(vmxPath string) ([]string, error) {
	data, err := vmwcommon.ReadVMX(vmxPath)
	if err != nil {
		return nil, err
	}

	var disks []string
	for k, v := range data {
		if !diskFilenameRe.MatchString(k) || !strings.EqualFold(filepath.Ext(v), ".vmdk") {
			continue
		}
		if present, ok := data[strings.TrimSuffix(k, ".filename")+".present"]; ok && strings.EqualFold(present, "FALSE") {
			continue
		}
		if !filepath.IsAbs(v) {
			v = filepath.Join(filepath.Dir(vmxPath), v)
		}
		disks = append(disks, v)
	}
	slices.Sort(disks)
	return disks, nil
}

// CheckDiskExists returns an error unless the .vmx attaches at least one
// disk and every attached disk exists.
func CheckDiskExists(vmxPath string) error {
	disks, err := Disks(vmxPath)
	if err != nil {
		return err
	}
	if len(disks) == 0 {
		return fmt.Errorf("%s: no disks attached", vmxPath)
	}
	for _, disk := range disks {
		if _, err := os.Stat(disk); err != nil {
			return fmt.Errorf("%s: %w", vmxPath, err)
		}
	}
	return nil
}

// CheckFileExists returns an error unless the file exists.
func CheckFileExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("expected %s: %w", path, err)
	}
	return nil
}

// CheckMarker returns an error unless the marker downloaded from the guest
// holds the value written by the shell provisioner.
func CheckMarker(path, want string) error {
	got, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading marker from the shell provisioner: %w", err)
	}
	if strings.TrimSpace(string(got)) != want {
		return fmt.Errorf("marker = %q, want %q", strings.TrimSpace(string(got)), want)
	}
	return nil
}

// ListSnapshots returns the snapshot names of the virtual machine.
func (h *Host) ListSnapshots(vmxPath string) ([]string, error) {
	out, err := h.Vmrun("listSnapshots", vmxPath)
	if err != nil {
		return nil, err
	}
	return parseSnapshots(out), nil
}

// parseSnapshots parses the output of vmrun listSnapshots.
func parseSnapshots(out string) []string {
	var names []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Total snapshots:") {
			continue
		}
		names = append(names, line)
	}
	return names
}

// Fingerprint records the .vmx and the disk chain in the directory of a
// source virtual machine. The .vmx and disk descriptors are hashed, and every
// .vmdk records its size and modification time. Snapshot metadata, lock, and
// log files are left out, since the hypervisor can touch them during a clone.
func Fingerprint(vmxPath string) (map[string]string, error) {
	dir := filepath.Dir(vmxPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	fp := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		ext := strings.ToLower(filepath.Ext(e.Name()))
		switch {
		case e.Name() == filepath.Base(vmxPath):
			sum, err := hashFile(path)
			if err != nil {
				return nil, err
			}
			fp[e.Name()] = sum
		case ext == ".vmdk":
			info, err := e.Info()
			if err != nil {
				return nil, err
			}
			value := fmt.Sprintf("size=%d mtime=%d", info.Size(), info.ModTime().UnixNano())
			if info.Size() <= descriptorReadLimit {
				sum, err := hashFile(path)
				if err != nil {
					return nil, err
				}
				value += " sha256=" + sum
			}
			fp[e.Name()] = value
		}
	}
	return fp, nil
}

// CompareFingerprints returns an error naming each file that changed.
func CompareFingerprints(before, after map[string]string) error {
	var errs []error
	for name, v := range before {
		if after[name] != v {
			errs = append(errs, fmt.Errorf("%s changed: %q -> %q", name, v, after[name]))
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			errs = append(errs, fmt.Errorf("%s was added", name))
		}
	}
	slices.SortFunc(errs, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })
	if len(errs) > 0 {
		return fmt.Errorf("source changed: %w", errors.Join(errs...))
	}
	return nil
}

// IsDeltaDisk reports whether the .vmdk descriptor names a parent disk.
func IsDeltaDisk(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	buf, err := io.ReadAll(io.LimitReader(f, descriptorReadLimit))
	if err != nil {
		return false, err
	}
	buf = bytes.ReplaceAll(buf, []byte("\r\n"), []byte("\n"))
	return parentFileNameHintRe.Match(buf), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
