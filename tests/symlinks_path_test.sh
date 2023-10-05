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

function check_link_has_mock_top_prefix {
  input_link=$1
  link_target=`readlink $input_link`
  if [[ $link_target != "$MOCK_TOP"* ]]; then
    echo "Symlink for file $input_link -> $link_target doesn't start with $MOCK_TOP"
    exit 1
  fi
}

function test_symlinks_updated_when_top_dir_changed {
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

  g_txt="out2/soong/workspace/a/g.txt"
  check_link_has_mock_top_prefix "$g_txt"

  move_mock_top

  (export OUT_DIR=$MOCK_TOP/$outdir; run_soong bp2build && run_bazel build --config=bp2build --config=ci //a:g)
  check_link_has_mock_top_prefix "$g_txt"
}

scan_and_run_tests