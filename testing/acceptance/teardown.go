// © Broadcom. All Rights Reserved.
// The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
// SPDX-License-Identifier: MPL-2.0

package acceptance

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const workspacePrefix = "packer-acc-"

// Workspace is the temporary directory that holds the output of one row.
// Teardown only removes virtual machines and files inside it.
type Workspace struct {
	Base       string
	OutputDir  string
	MarkerPath string

	host *Host
	once sync.Once
	err  error
}

// NewWorkspace creates a workspace and registers its teardown with the test,
// so it runs even when the build is never started.
func NewWorkspace(t *testing.T, h *Host) *Workspace {
	t.Helper()
	base, err := os.MkdirTemp("", workspacePrefix+"*")
	if err != nil {
		t.Fatalf("creating workspace: %s", err)
	}

	// Resolve symbolic links so paths match the output of vmrun list.
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}

	ws := &Workspace{
		Base:       base,
		OutputDir:  filepath.Join(base, "output"),
		MarkerPath: filepath.Join(base, "marker.txt"),
		host:       h,
	}
	t.Cleanup(func() {
		if err := ws.Teardown(); err != nil {
			t.Errorf("teardown: %s", err)
		}
	})
	return ws
}

// Teardown stops and deletes each virtual machine in the output directory
// and then removes the workspace. It is safe to call more than once, and a
// virtual machine or directory that is already gone counts as success.
func (ws *Workspace) Teardown() error {
	ws.once.Do(func() {
		ws.err = ws.teardown()
	})
	return ws.err
}

func (ws *Workspace) teardown() error {
	if err := checkWorkspace(ws.Base); err != nil {
		return err
	}

	vmxPaths, err := findVMX(ws.OutputDir)
	if err != nil {
		return err
	}

	if ws.host != nil && len(vmxPaths) > 0 {
		running := ws.runningVMs()
		for _, vmxPath := range vmxPaths {
			if !within(ws.Base, vmxPath) {
				return fmt.Errorf("refusing to delete %s outside of workspace %s", vmxPath, ws.Base)
			}
			if running[normalizePath(vmxPath)] {
				if _, err := ws.host.Vmrun("stop", vmxPath, "hard"); err != nil {
					log.Printf("[WARN] %s", err)
				}
			}
			if _, err := ws.host.Vmrun("deleteVM", vmxPath); err != nil {
				log.Printf("[WARN] %s", err)
			}
		}
	}

	return removeAll(ws.Base)
}

// runningVMs returns the normalized paths reported by vmrun list.
func (ws *Workspace) runningVMs() map[string]bool {
	running := map[string]bool{}
	out, err := ws.host.Vmrun("list")
	if err != nil {
		log.Printf("[WARN] %s", err)
		return running
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(strings.ToLower(line), ".vmx") {
			running[normalizePath(line)] = true
		}
	}
	return running
}

// checkWorkspace refuses a base directory that was not created by NewWorkspace.
func checkWorkspace(base string) error {
	if base == "" || !filepath.IsAbs(base) {
		return fmt.Errorf("refusing to tear down workspace %q: not an absolute path", base)
	}
	if !strings.HasPrefix(filepath.Base(base), workspacePrefix) {
		return fmt.Errorf("refusing to tear down workspace %q: not a %s* directory", base, workspacePrefix)
	}
	return nil
}

// within reports whether path is inside base.
func within(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// findVMX returns each .vmx under dir, including those in .vmwarevm bundles.
// A missing directory has no virtual machines.
func findVMX(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".vmx") {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

// removeAll removes the directory, retrying while Windows releases file locks.
func removeAll(dir string) error {
	var err error
	for range 5 {
		if err = os.RemoveAll(dir); err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("removing %s: %w", dir, err)
}

// normalizePath cleans a path for comparison, ignoring case on hosts with
// case-insensitive file systems by default.
func normalizePath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
