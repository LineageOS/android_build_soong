#!/bin/bash -eu

set -o pipefail

# Test that bp2build and Bazel can play nicely together

source "$(dirname "$0")/lib.sh"

function test_bp2build_generates_all_buildfiles {
  setup
  create_mock_bazel

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

  run_bp2build

  if [[ ! -f "./out/soong/workspace/foo/convertible_soong_module/BUILD" ]]; then
    fail "./out/soong/workspace/foo/convertible_soong_module/BUILD was not generated"
  fi

  if [[ ! -f "./out/soong/workspace/foo/unconvertible_soong_module/BUILD" ]]; then
    fail "./out/soong/workspace/foo/unconvertible_soong_module/BUILD was not generated"
  fi

  if ! grep "the_answer" "./out/soong/workspace/foo/convertible_soong_module/BUILD"; then
    fail "missing BUILD target the_answer in convertible_soong_module/BUILD"
  fi

  if grep "not_the_answer" "./out/soong/workspace/foo/unconvertible_soong_module/BUILD"; then
    fail "found unexpected BUILD target not_the_answer in unconvertible_soong_module/BUILD"
  fi

  if ! grep "filegroup" "./out/soong/workspace/foo/unconvertible_soong_module/BUILD"; then
    fail "missing filegroup in unconvertible_soong_module/BUILD"
  fi

  # NOTE: We don't actually use the extra BUILD file for anything here
  run_bazel build --package_path=out/soong/workspace //foo/...

  local the_answer_file="bazel-out/k8-fastbuild/bin/foo/convertible_soong_module/the_answer.txt"
  if [[ ! -f "${the_answer_file}" ]]; then
    fail "Expected '${the_answer_file}' to be generated, but was missing"
  fi
  if ! grep 42 "${the_answer_file}"; then
    fail "Expected to find 42 in '${the_answer_file}'"
  fi
}

test_bp2build_generates_all_buildfiles
