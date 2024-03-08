#!/bin/bash -eu

set -o pipefail

HARDWIRED_MOCK_TOP=
# Uncomment this to be able to view the source tree after a test is run
# HARDWIRED_MOCK_TOP=/tmp/td

REAL_TOP="$(readlink -f "$(dirname "$0")"/../../..)"

function make_mock_top {
  mock=$(mktemp -t -d st.XXXXX)
  echo "$mock"
}

if [[ -n "$HARDWIRED_MOCK_TOP" ]]; then
  MOCK_TOP="$HARDWIRED_MOCK_TOP"
else
  MOCK_TOP=$(make_mock_top)
  trap cleanup_mock_top EXIT
fi

WARMED_UP_MOCK_TOP=$(mktemp -t soong_integration_tests_warmup.XXXXXX.tar.gz)
trap 'rm -f "$WARMED_UP_MOCK_TOP"' EXIT

function warmup_mock_top {
  info "Warming up mock top ..."
  info "Mock top warmup archive: $WARMED_UP_MOCK_TOP"
  cleanup_mock_top
  mkdir -p "$MOCK_TOP"
  cd "$MOCK_TOP"

  create_mock_soong
  run_soong
  tar czf "$WARMED_UP_MOCK_TOP" *
}

function cleanup_mock_top {
  cd /
  rm -fr "$MOCK_TOP"
}

function info {
  echo -e "\e[92;1m[TEST HARNESS INFO]\e[0m" "$*"
}

function fail {
  echo -e "\e[91;1mFAILED:\e[0m" "$*"
  exit 1
}

function copy_directory {
  local dir="$1"
  local -r parent="$(dirname "$dir")"

  mkdir -p "$MOCK_TOP/$parent"
  cp -R "$REAL_TOP/$dir" "$MOCK_TOP/$parent"
}

function delete_directory {
  rm -rf "$MOCK_TOP/$1"
}

function symlink_file {
  local file="$1"

  mkdir -p "$MOCK_TOP/$(dirname "$file")"
  ln -s "$REAL_TOP/$file" "$MOCK_TOP/$file"
}

function symlink_directory {
  local dir="$1"

  mkdir -p "$MOCK_TOP/$dir"
  # We need to symlink the contents of the directory individually instead of
  # using one symlink for the whole directory because finder.go doesn't follow
  # symlinks when looking for Android.bp files
  for i in "$REAL_TOP/$dir"/*; do
    i=$(basename "$i")
    local target="$MOCK_TOP/$dir/$i"
    local source="$REAL_TOP/$dir/$i"

    if [[ -e "$target" ]]; then
      if [[ ! -d "$source" || ! -d "$target" ]]; then
        fail "Trying to symlink $dir twice"
      fi
    else
      ln -s "$REAL_TOP/$dir/$i" "$MOCK_TOP/$dir/$i";
    fi
  done
}

function create_mock_soong {
  create_mock_bazel
  copy_directory build/blueprint
  copy_directory build/soong
  copy_directory build/make

  symlink_directory prebuilts/sdk
  symlink_directory prebuilts/go
  symlink_directory prebuilts/build-tools
  symlink_directory prebuilts/clang/host
  symlink_directory external/compiler-rt
  symlink_directory external/go-cmp
  symlink_directory external/golang-protobuf
  symlink_directory external/licenseclassifier
  symlink_directory external/starlark-go
  symlink_directory external/python
  symlink_directory external/sqlite
  symlink_directory external/spdx-tools
  symlink_directory libcore

  # TODO: b/286872909 - Remove these when the blocking bug is completed
  symlink_directory external/libavc
  symlink_directory external/libaom
  symlink_directory external/libvpx
  symlink_directory frameworks/base/libs/androidfw
  symlink_directory external/libhevc
  symlink_directory external/libexif
  symlink_directory external/libopus
  symlink_directory external/libmpeg2
  symlink_directory external/expat
  symlink_directory external/flac
  symlink_directory system/extras/toolchain-extras

  touch "$MOCK_TOP/Android.bp"
}

function setup {
  cleanup_mock_top
  mkdir -p "$MOCK_TOP"

  echo
  echo ----------------------------------------------------------------------------
  info "Running test case \e[96;1m${FUNCNAME[1]}\e[0m"
  cd "$MOCK_TOP"

  tar xzf "$WARMED_UP_MOCK_TOP" --warning=no-timestamp
}

# shellcheck disable=SC2120
function run_soong {
  USE_RBE=false build/soong/soong_ui.bash --make-mode --skip-ninja --skip-config --soong-only --skip-soong-tests "$@"
}

function create_mock_bazel {
  copy_directory build/bazel
  copy_directory build/bazel_common_rules

  # This requires pulling more tools into the mock top to build partitions
  delete_directory build/bazel/examples/partitions

  symlink_directory packages/modules/common/build
  symlink_directory prebuilts/bazel
  symlink_directory prebuilts/clang
  symlink_directory prebuilts/jdk
  symlink_directory external/bazel-skylib
  symlink_directory external/bazelbuild-rules_android
  symlink_directory external/bazelbuild-rules_go
  symlink_directory external/bazelbuild-rules_license
  symlink_directory external/bazelbuild-kotlin-rules
  symlink_directory external/bazelbuild-rules_cc
  symlink_directory external/bazelbuild-rules_python
  symlink_directory external/bazelbuild-rules_java
  symlink_directory external/bazelbuild-rules_rust
  symlink_directory external/bazelbuild-rules_testing
  symlink_directory external/rust/crates/tinyjson

  symlink_file WORKSPACE
  symlink_file BUILD
}

function run_bazel {
  # Remove the ninja_build output marker file to communicate to buildbot that this is not a regular Ninja build, and its
  # output should not be parsed as such.
  rm -rf out/ninja_build

  build/bazel/bin/bazel "$@"
}

function run_ninja {
  build/soong/soong_ui.bash --make-mode --skip-config --soong-only --skip-soong-tests "$@"
}

info "Starting Soong integration test suite $(basename "$0")"
info "Mock top: $MOCK_TOP"


export ALLOW_MISSING_DEPENDENCIES=true
export ALLOW_BP_UNDER_SYMLINKS=true
warmup_mock_top

function scan_and_run_tests {
  # find all test_ functions
  # NB "declare -F" output is sorted, hence test order is deterministic
  readarray -t test_fns < <(declare -F | sed -n -e 's/^declare -f \(test_.*\)$/\1/p')
  info "Found ${#test_fns[*]} tests"
  if [[ ${#test_fns[*]} -eq 0 ]]; then
    fail "No tests found"
  fi
  for f in ${test_fns[*]}; do
    $f
    info "Completed test case \e[96;1m$f\e[0m"
  done
}

function move_mock_top {
  MOCK_TOP2=$(make_mock_top)
  rm -rf $MOCK_TOP2
  mv $MOCK_TOP $MOCK_TOP2
  MOCK_TOP=$MOCK_TOP2
  trap cleanup_mock_top EXIT
}
