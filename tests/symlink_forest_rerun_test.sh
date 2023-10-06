#!/bin/bash -eu

set -o pipefail

# Tests that symlink forest will replant if soong_build has changed
# Any change to the build system should trigger a rerun

source "$(dirname "$0")/lib.sh"

function test_symlink_forest_reruns {
  setup

  mkdir -p a
  touch a/g.txt
  cat > a/Android.bp <<'EOF'
filegroup {
    name: "g",
    srcs: ["g.txt"],
  }
EOF

  run_soong g

  mtime=`cat out/soong/workspace/soong_build_mtime`
  # rerun with no changes - ensure that it hasn't changed
  run_soong g
  newmtime=`cat out/soong/workspace/soong_build_mtime`
  if [[ ! "$mtime" == "$mtime" ]]; then
     fail "symlink forest reran when it shouldn't have"
  fi

  # change exit codes to force a soong_build rebuild.
  sed -i 's/os.Exit(1)/os.Exit(2)/g' build/soong/bp2build/symlink_forest.go

  run_soong g
  newmtime=`cat out/soong/workspace/soong_build_mtime`
  if [[ "$mtime" == "$newmtime" ]]; then
     fail "symlink forest did not rerun when it should have"
  fi

}

scan_and_run_tests
