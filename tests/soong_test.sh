#!/bin/bash -eu

set -o pipefail

# Tests of Soong functionality

source "$(dirname "$0")/lib.sh"

function test_m_clean_works {
  setup

  # Create a directory with files that cannot be removed
  mkdir -p out/bad_directory_permissions
  touch out/bad_directory_permissions/unremovable_file
  # File permissions are fine but directory permissions are bad
  chmod a+rwx out/bad_directory_permissions/unremovable_file
  chmod a-rwx out/bad_directory_permissions

  run_soong clean
}

test_m_clean_works
