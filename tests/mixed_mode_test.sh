#!/bin/bash

set -o pipefail

# This test exercises mixed builds where Soong and Bazel cooperate in building
# Android.
#
# When the execroot is deleted, the Bazel server process will automatically
# terminate itself.

source "$(dirname "$0")/lib.sh"

function test_bazel_smoke {
  setup

  run_soong bp2build

  run_bazel info --config=bp2build
}

function test_add_irrelevant_file {
  setup

  mkdir -p soong_tests/a/b
  touch soong_tests/a/b/c.txt
  cat > soong_tests/a/b/Android.bp <<'EOF'
filegroup {
  name: "c",
  srcs: ["c.txt"],
  bazel_module: { bp2build_available: true },
}
EOF

  run_soong --bazel-mode-staging nothing

  if [[ ! -e out/soong/bp2build/soong_tests/a/b/BUILD.bazel ]]; then
    fail "BUILD.bazel not created"
  fi

  if [[ ! -e out/soong/build.ninja ]]; then
    fail "build.ninja not created"
  fi

  local mtime_build1=$(stat -c "%y" out/soong/bp2build/soong_tests/a/b/BUILD.bazel)
  local mtime_ninja1=$(stat -c "%y" out/soong/build.ninja)

  touch soong_tests/a/irrelevant.txt

  run_soong --bazel-mode-staging nothing
  local mtime_build2=$(stat -c "%y" out/soong/bp2build/soong_tests/a/b/BUILD.bazel)
  local mtime_ninja2=$(stat -c "%y" out/soong/build.ninja)

  if [[ "$mtime_build1" != "$mtime_build2" ]]; then
    fail "BUILD.bazel was generated"
  fi

  if [[ "$mtime_ninja1" != "$mtime_ninja2" ]]; then
    fail "build.ninja was regenerated"
  fi

  if [[ ! -e out/soong/workspace/soong_tests/a/irrelevant.txt ]]; then
    fail "new file was not symlinked"
  fi
}

function test_force_enabled_modules {
  setup
  # b/273910287 - test force enable modules
  mkdir -p soong_tests/a/b
  cat > soong_tests/a/b/Android.bp <<'EOF'
genrule {
    name: "touch-file",
    out: ["fake-out.txt"],
    cmd: "touch $(out)",
    bazel_module: { bp2build_available: true },
}

genrule {
    name: "unenabled-touch-file",
    out: ["fake-out2.txt"],
    cmd: "touch $(out)",
    bazel_module: { bp2build_available: false },
}
EOF
  run_soong --bazel-mode-staging --bazel-force-enabled-modules=touch-file nothing
  local bazel_contained=`grep out/soong/.intermediates/soong_tests/a/b/touch-file/gen/fake-out.txt out/soong/build.ninja`
  if [[ $bazel_contained == '' ]]; then
    fail "Bazel actions not found for force-enabled module"
  fi

  unused=`run_soong --bazel-force-enabled-modules=unenabled-touch-file --ensure-allowlist-integrity nothing >/dev/null`

  if [[ $? -ne 1 ]]; then
    fail "Expected failure due to force-enabling an unenabled module "
  fi
}


scan_and_run_tests
