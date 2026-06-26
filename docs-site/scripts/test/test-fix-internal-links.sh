#!/usr/bin/env bash
# © Broadcom. All Rights Reserved.
# The term "Broadcom" refers to Broadcom Inc. and/or its subsidiaries.
# SPDX-License-Identifier: MPL-2.0

# Tests for internal link and anchor fixes in staged markdown.

set -euo pipefail

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPTS_DIR="$(cd "${TEST_DIR}/.." && pwd)"
LIB_DIR="${SCRIPTS_DIR}/lib"

# shellcheck source=assertions.sh
source "${TEST_DIR}/assertions.sh"
# shellcheck source=../lib/fix-internal-links.sh
source "${LIB_DIR}/fix-internal-links.sh"

test_same_page_link() {
  local input='[Hardware Configuration](builders/iso.md#hardware-configuration)'
  local output
  output="$(fix_internal_links "$input" "builders/iso.md")"
  assert_contains "$output" '[Hardware Configuration](#hardware-configuration)' "same-page .md link"
  assert_not_contains "$output" 'builders/iso.md' "no absolute same-page path"
}

test_sibling_builder_link() {
  local input='[VMware VMX](builders/vmx.md)'
  local output
  output="$(fix_internal_links "$input" "builders/iso.md")"
  assert_contains "$output" '[VMware VMX](vmx/)' "sibling builder link"
}

test_anchor_passthrough() {
  local input='See [SSH](#ssh) for authentication options.'
  local output
  output="$(fix_internal_links "$input" "builders/iso.md")"
  assert_contains "$output" '#ssh' "anchor is preserved without remapping"
  assert_not_contains "$output" '#location-configuration' "no vsphere-specific anchor remapping"
}

test_home_page_component_link() {
  local input='[VMware ISO](builders/iso.md)'
  local output
  output="$(fix_internal_links "$input" "index.md")"
  assert_contains "$output" '[VMware ISO](builders/iso/)' "home page component link"
  assert_not_contains "$output" '../builders/' "home page link does not escape site root"
}

main() {
  test_same_page_link
  test_sibling_builder_link
  test_anchor_passthrough
  test_home_page_component_link
  echo "All fix-internal-links tests passed."
}

main "$@"
