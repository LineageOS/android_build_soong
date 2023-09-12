#!/bin/bash -eu

set -o pipefail

# Test that relative symlinks work by recreating the bug in b/259191764
# In some cases, developers prefer to move their checkouts. This causes
# issues in that symlinked files (namely, the bazel wrapper script)
# cannot be found. As such, we implemented relative symlinks so that a
# moved checkout doesn't need a full clean before rebuilding.
# The bazel output base will still need to be removed, as Starlark
# doesn't seem to support relative symlinks yet.

source "$(dirname "$0")/lib.sh"

function test_movable_top_bazel_build {
  setup

  mkdir -p a
  touch a/g.txt
  cat > a/Android.bp <<'EOF'
filegroup {
    name: "g",
    srcs: ["g.txt"],
    bazel_module: {bp2build_available: true},
}
EOF
  # A directory under $MOCK_TOP
  outdir=out2

  # Modify OUT_DIR in a subshell so it doesn't affect the top level one.
  (export OUT_DIR=$MOCK_TOP/$outdir; run_soong bp2build && run_bazel build --config=bp2build --config=ci //a:g)

  move_mock_top

  # remove the bazel output base
  rm -rf $outdir/bazel/output_user_root
  (export OUT_DIR=$MOCK_TOP/$outdir; run_soong bp2build && run_bazel build --config=bp2build --config=ci //a:g)
}

function test_movable_top_soong_build {
  setup

  mkdir -p a
  touch a/g.txt
  cat > a/Android.bp <<'EOF'
filegroup {
    name: "g",
    srcs: ["g.txt"],
}
EOF

  # A directory under $MOCK_TOP
  outdir=out2

  # Modify OUT_DIR in a subshell so it doesn't affect the top level one.
  (export OUT_DIR=$MOCK_TOP/$outdir; run_soong g)

  move_mock_top

  # remove the bazel output base
  rm -rf $outdir/bazel/output
  (export OUT_DIR=$MOCK_TOP/$outdir; run_soong g)
}

function test_remove_output_base_and_ninja_file {
  # If the bazel output base is removed without the ninja file, the build will fail
  # This tests that removing both the bazel output base and ninja file will succeed
  # without a clean
  setup

  mkdir -p a
  touch a/g.txt
  cat > a/Android.bp <<'EOF'
filegroup {
    name: "g",
    srcs: ["g.txt"],
}
EOF
  outdir=out2

  # Modify OUT_DIR in a subshell so it doesn't affect the top level one.
  (export OUT_DIR=$MOCK_TOP/$outdir; run_soong g)
  # remove the bazel output base
  rm -rf $outdir/bazel/output
  rm $outdir/soong/build*ninja

  (export OUT_DIR=$MOCK_TOP/$outdir; run_soong g)
}

scan_and_run_tests
