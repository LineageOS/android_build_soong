#!/bin/bash -eu

set -o pipefail

# Test that bp2build and Bazel can play nicely together

source "$(dirname "$0")/lib.sh"

readonly GENERATED_BUILD_FILE_NAME="BUILD.bazel"

function test_bp2build_null_build {
  setup
  run_soong bp2build
  local -r output_mtime1=$(stat -c "%y" out/soong/bp2build_workspace_marker)

  run_soong bp2build
  local -r output_mtime2=$(stat -c "%y" out/soong/bp2build_workspace_marker)

  if [[ "$output_mtime1" != "$output_mtime2" ]]; then
    fail "Output bp2build marker file changed on null build"
  fi
}

# Tests that, if bp2build reruns due to a blueprint file changing, that
# BUILD files whose contents are unchanged are not regenerated.
function test_bp2build_unchanged {
  setup

  mkdir -p pkg
  touch pkg/x.txt
  cat > pkg/Android.bp <<'EOF'
filegroup {
    name: "x",
    srcs: ["x.txt"],
    bazel_module: {bp2build_available: true},
  }
EOF

  run_soong bp2build
  local -r buildfile_mtime1=$(stat -c "%y" out/soong/bp2build/pkg/BUILD.bazel)
  local -r marker_mtime1=$(stat -c "%y" out/soong/bp2build_workspace_marker)

  # Force bp2build to rerun by updating the timestamp of a blueprint file.
  touch pkg/Android.bp

  run_soong bp2build
  local -r buildfile_mtime2=$(stat -c "%y" out/soong/bp2build/pkg/BUILD.bazel)
  local -r marker_mtime2=$(stat -c "%y" out/soong/bp2build_workspace_marker)

  if [[ "$marker_mtime1" == "$marker_mtime2" ]]; then
    fail "Expected bp2build marker file to change"
  fi
  if [[ "$buildfile_mtime1" != "$buildfile_mtime2" ]]; then
    fail "BUILD.bazel was updated even though contents are same"
  fi

  # Force bp2build to rerun by updating the timestamp of the constants_exported_to_soong.bzl file.
  touch build/bazel/constants_exported_to_soong.bzl

  run_soong bp2build
  local -r buildfile_mtime3=$(stat -c "%y" out/soong/bp2build/pkg/BUILD.bazel)
  local -r marker_mtime3=$(stat -c "%y" out/soong/bp2build_workspace_marker)

  if [[ "$marker_mtime2" == "$marker_mtime3" ]]; then
    fail "Expected bp2build marker file to change"
  fi
  if [[ "$buildfile_mtime2" != "$buildfile_mtime3" ]]; then
    fail "BUILD.bazel was updated even though contents are same"
  fi
}

# Tests that blueprint files that are deleted are not present when the
# bp2build tree is regenerated.
function test_bp2build_deleted_blueprint {
  setup

  mkdir -p pkg
  touch pkg/x.txt
  cat > pkg/Android.bp <<'EOF'
filegroup {
    name: "x",
    srcs: ["x.txt"],
    bazel_module: {bp2build_available: true},
  }
EOF

  run_soong bp2build
  if [[ ! -e "./out/soong/bp2build/pkg/BUILD.bazel" ]]; then
    fail "Expected pkg/BUILD.bazel to be generated"
  fi

  rm pkg/Android.bp

  run_soong bp2build
  if [[ -e "./out/soong/bp2build/pkg/BUILD.bazel" ]]; then
    fail "Expected pkg/BUILD.bazel to be deleted"
  fi
}

function test_bp2build_null_build_with_globs {
  setup

  mkdir -p foo/bar
  cat > foo/bar/Android.bp <<'EOF'
filegroup {
    name: "globs",
    srcs: ["*.txt"],
  }
EOF
  touch foo/bar/a.txt foo/bar/b.txt

  run_soong bp2build
  local -r output_mtime1=$(stat -c "%y" out/soong/bp2build_workspace_marker)

  run_soong bp2build
  local -r output_mtime2=$(stat -c "%y" out/soong/bp2build_workspace_marker)

  if [[ "$output_mtime1" != "$output_mtime2" ]]; then
    fail "Output bp2build marker file changed on null build"
  fi
}

function test_different_relative_outdir {
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
  trap "rm -rf $outdir" EXIT
  # Modify OUT_DIR in a subshell so it doesn't affect the top level one.
  (export OUT_DIR=$outdir; run_soong bp2build && run_bazel build --config=bp2build --config=ci //a:g)
}

function test_different_absolute_outdir {
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

  # A directory under /tmp/...
  outdir=$(mktemp -t -d st.XXXXX)
  trap 'rm -rf $outdir' EXIT
  # Modify OUT_DIR in a subshell so it doesn't affect the top level one.
  (export OUT_DIR=$outdir; run_soong bp2build && run_bazel build --config=bp2build --config=ci //a:g)
}

function _bp2build_generates_all_buildfiles {
  setup

  mkdir -p foo/convertible_soong_module
  cat > foo/convertible_soong_module/Android.bp <<'EOF'
genrule {
    name: "the_answer",
    cmd: "echo '42' > $(out)",
    out: [
        "the_answer.txt",
    ],
    bazel_module: {
        bp2build_available: true,
    },
  }
EOF

  mkdir -p foo/unconvertible_soong_module
  cat > foo/unconvertible_soong_module/Android.bp <<'EOF'
genrule {
    name: "not_the_answer",
    cmd: "echo '43' > $(out)",
    out: [
        "not_the_answer.txt",
    ],
    bazel_module: {
        bp2build_available: false,
    },
  }
EOF

  run_soong bp2build

  if [[ ! -f "./out/soong/workspace/foo/convertible_soong_module/${GENERATED_BUILD_FILE_NAME}" ]]; then
    fail "./out/soong/workspace/foo/convertible_soong_module/${GENERATED_BUILD_FILE_NAME} was not generated"
  fi

  if [[ ! -f "./out/soong/workspace/foo/unconvertible_soong_module/${GENERATED_BUILD_FILE_NAME}" ]]; then
    fail "./out/soong/workspace/foo/unconvertible_soong_module/${GENERATED_BUILD_FILE_NAME} was not generated"
  fi

  if ! grep "the_answer" "./out/soong/workspace/foo/convertible_soong_module/${GENERATED_BUILD_FILE_NAME}"; then
    fail "missing BUILD target the_answer in convertible_soong_module/${GENERATED_BUILD_FILE_NAME}"
  fi

  if grep "not_the_answer" "./out/soong/workspace/foo/unconvertible_soong_module/${GENERATED_BUILD_FILE_NAME}"; then
    fail "found unexpected BUILD target not_the_answer in unconvertible_soong_module/${GENERATED_BUILD_FILE_NAME}"
  fi

  if ! grep "filegroup" "./out/soong/workspace/foo/unconvertible_soong_module/${GENERATED_BUILD_FILE_NAME}"; then
    fail "missing filegroup in unconvertible_soong_module/${GENERATED_BUILD_FILE_NAME}"
  fi

  # NOTE: We don't actually use the extra BUILD file for anything here
  run_bazel build --config=android --config=bp2build --config=ci //foo/...

  local -r the_answer_file="$(find -L bazel-out -name the_answer.txt)"
  if [[ ! -f "${the_answer_file}" ]]; then
    fail "Expected the_answer.txt to be generated, but was missing"
  fi
  if ! grep 42 "${the_answer_file}"; then
    fail "Expected to find 42 in '${the_answer_file}'"
  fi
}

function test_bp2build_generates_all_buildfiles {
  _save_trap=$(trap -p EXIT)
  trap '[[ $? -ne 0 ]] && echo Are you running this locally? Try changing --sandbox_tmpfs_path to something other than /tmp/ in build/bazel/linux.bazelrc.' EXIT
  _bp2build_generates_all_buildfiles
  eval "${_save_trap}"
}

function test_build_files_take_precedence {
  _save_trap=$(trap -p EXIT)
  trap '[[ $? -ne 0 ]] && echo Are you running this locally? Try changing --sandbox_tmpfs_path to something other than /tmp/ in build/bazel/linux.bazelrc.' EXIT
  _build_files_take_precedence
  eval "${_save_trap}"
}

function _build_files_take_precedence {
  setup

  # This specific directory is hardcoded in bp2build as being one
  # where the BUILD file should be intentionally kept.
  mkdir -p testpkg/keep_build_file
  cat > testpkg/keep_build_file/Android.bp <<'EOF'
genrule {
    name: "print_origin",
    cmd: "echo 'from_soong' > $(out)",
    out: [
        "origin.txt",
    ],
    bazel_module: {
        bp2build_available: true,
    },
  }
EOF

  run_soong bp2build
  run_bazel build --config=android --config=bp2build --config=ci //testpkg/keep_build_file:print_origin

  local -r output_file="$(find -L bazel-out -name origin.txt)"
  if [[ ! -f "${output_file}" ]]; then
    fail "Expected origin.txt to be generated, but was missing"
  fi
  if ! grep from_soong "${output_file}"; then
    fail "Expected to find 'from_soong' in '${output_file}'"
  fi

  cat > testpkg/keep_build_file/BUILD.bazel <<'EOF'
genrule(
    name = "print_origin",
    outs = ["origin.txt"],
    cmd = "echo 'from_bazel' > $@",
)
EOF

  # Clean the workspace. There is a test infrastructure bug where run_bazel
  # will symlink Android.bp files in the source directory again and thus
  # pollute the workspace.
  # TODO: b/286059878 - Remove this clean after the underlying bug is fixed.
  run_soong clean
  run_soong bp2build
  run_bazel build --config=android --config=bp2build --config=ci //testpkg/keep_build_file:print_origin
  if ! grep from_bazel "${output_file}"; then
    fail "Expected to find 'from_bazel' in '${output_file}'"
  fi
}

function test_bp2build_symlinks_files {
  setup
  mkdir -p foo
  touch foo/BLANK1
  touch foo/BLANK2
  touch foo/F2D
  touch foo/BUILD

  run_soong bp2build

  if [[ -e "./out/soong/workspace/foo/BUILD" ]]; then
    fail "./out/soong/workspace/foo/BUILD should be omitted"
  fi
  for file in BLANK1 BLANK2 F2D
  do
    if [[ ! -L "./out/soong/workspace/foo/$file" ]]; then
      fail "./out/soong/workspace/foo/$file should exist"
    fi
  done
  local -r BLANK1_BEFORE=$(stat -c %y "./out/soong/workspace/foo/BLANK1")

  rm foo/BLANK2
  rm foo/F2D
  mkdir foo/F2D
  touch foo/F2D/BUILD

  run_soong bp2build

  if [[ -e "./out/soong/workspace/foo/BUILD" ]]; then
    fail "./out/soong/workspace/foo/BUILD should be omitted"
  fi
  local -r BLANK1_AFTER=$(stat -c %y "./out/soong/workspace/foo/BLANK1")
  if [[ "$BLANK1_AFTER" != "$BLANK1_BEFORE" ]]; then
    fail "./out/soong/workspace/foo/BLANK1 should be untouched"
  fi
  if [[  -e "./out/soong/workspace/foo/BLANK2" ]]; then
    fail "./out/soong/workspace/foo/BLANK2 should be removed"
  fi
  if [[ -L "./out/soong/workspace/foo/F2D" ]] || [[ ! -d "./out/soong/workspace/foo/F2D" ]]; then
    fail "./out/soong/workspace/foo/F2D should be a dir"
  fi
}

function test_cc_correctness {
  setup

  mkdir -p a
  cat > a/Android.bp <<EOF
cc_object {
  name: "qq",
  srcs: ["qq.cc"],
  bazel_module: {
    bp2build_available: true,
  },
  stl: "none",
  system_shared_libs: [],
}
EOF

  cat > a/qq.cc <<EOF
#include "qq.h"
int qq() {
  return QQ;
}
EOF

  cat > a/qq.h <<EOF
#define QQ 1
EOF

  run_soong bp2build

  run_bazel build --config=android --config=bp2build --config=ci //a:qq
  local -r output_mtime1=$(stat -c "%y" bazel-bin/a/_objs/qq/qq.o)

  run_bazel build --config=android --config=bp2build --config=ci //a:qq
  local -r output_mtime2=$(stat -c "%y" bazel-bin/a/_objs/qq/qq.o)

  if [[ "$output_mtime1" != "$output_mtime2" ]]; then
    fail "output changed on null build"
  fi

  cat > a/qq.h <<EOF
#define QQ 2
EOF

  run_bazel build --config=android --config=bp2build --config=ci //a:qq
  local -r output_mtime3=$(stat -c "%y" bazel-bin/a/_objs/qq/qq.o)

  if [[ "$output_mtime1" == "$output_mtime3" ]]; then
    fail "output not changed when included header changed"
  fi
}

# Regression test for the following failure during symlink forest creation:
#
#   Cannot stat '/tmp/st.rr054/foo/bar/unresolved_symlink': stat /tmp/st.rr054/foo/bar/unresolved_symlink: no such file or directory
#
function test_bp2build_null_build_with_unresolved_symlink_in_source() {
  setup

  mkdir -p foo/bar
  ln -s /tmp/non-existent foo/bar/unresolved_symlink
  cat > foo/bar/Android.bp <<'EOF'
filegroup {
    name: "fg",
    srcs: ["unresolved_symlink/non-existent-file.txt"],
  }
EOF

  run_soong bp2build

  dest=$(readlink -f out/soong/workspace/foo/bar/unresolved_symlink)
  if [[ "$dest" != "/tmp/non-existent" ]]; then
    fail "expected to plant an unresolved symlink out/soong/workspace/foo/bar/unresolved_symlink that resolves to /tmp/non-existent"
  fi
}

function test_bazel_standalone_output_paths_contain_product_name {
  setup
  mkdir -p a
  cat > a/Android.bp <<EOF
cc_object {
  name: "qq",
  srcs: ["qq.cc"],
  bazel_module: {
    bp2build_available: true,
  },
  stl: "none",
  system_shared_libs: [],
}
EOF

  cat > a/qq.cc <<EOF
#include "qq.h"
int qq() {
  return QQ;
}
EOF

  cat > a/qq.h <<EOF
#define QQ 1
EOF

  export TARGET_PRODUCT=aosp_arm; run_soong bp2build
  local -r output=$(run_bazel cquery //a:qq --output=files --config=android --config=bp2build --config=ci)
  if [[ ! $(echo ${output} | grep "bazel-out/aosp_arm") ]]; then
    fail "Did not find the product name '${TARGET_PRODUCT}' in the output path. This can cause " \
      "unnecessary rebuilds when toggling between products as bazel outputs for different products will " \
      "clobber each other. Output paths are: \n${output}"
  fi
}

scan_and_run_tests
