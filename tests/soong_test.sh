#!/bin/bash -eu

set -o pipefail

# Tests of Soong functionality

source "$(dirname "$0")/lib.sh"

function test_m_clean_works {
  setup

  mkdir -p out/some_directory
  touch out/some_directory/some_file

  run_soong clean
}

scan_and_run_tests