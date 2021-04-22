#!/bin/bash -eu

set -o pipefail

HARDWIRED_MOCK_TOP=
# Uncomment this to be able to view the source tree after a test is run
# HARDWIRED_MOCK_TOP=/tmp/td

REAL_TOP="$(readlink -f "$(dirname "$0")"/../../..)"

if [[ ! -z "$HARDWIRED_MOCK_TOP" ]]; then
  MOCK_TOP="$HARDWIRED_MOCK_TOP"
else
  MOCK_TOP=$(mktemp -t -d st.XXXXX)
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
  echo -e "\e[92;1m[TEST HARNESS INFO]\e[0m" $*
}

function fail {
  echo -e "\e[91;1mFAILED:\e[0m" $*
  exit 1
}

function copy_directory() {
  local dir="$1"
  local parent="$(dirname "$dir")"

  mkdir -p "$MOCK_TOP/$parent"
  cp -R "$REAL_TOP/$dir" "$MOCK_TOP/$parent"
}

function symlink_file() {
  local file="$1"

  mkdir -p "$MOCK_TOP/$(dirname "$file")"
  ln -s "$REAL_TOP/$file" "$MOCK_TOP/$file"
}

function symlink_directory() {
  local dir="$1"

  mkdir -p "$MOCK_TOP/$dir"
  # We need to symlink the contents of the directory individually instead of
  # using one symlink for the whole directory because finder.go doesn't follow
  # symlinks when looking for Android.bp files
  for i in $(ls "$REAL_TOP/$dir"); do
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
  copy_directory build/blueprint
  copy_directory build/soong

  symlink_directory prebuilts/go
  symlink_directory prebuilts/build-tools
  symlink_directory external/golang-protobuf

  touch "$MOCK_TOP/Android.bp"
}

function setup() {
  cleanup_mock_top
  mkdir -p "$MOCK_TOP"

  echo
  echo ----------------------------------------------------------------------------
  info "Running test case \e[96;1m${FUNCNAME[1]}\e[0m"
  cd "$MOCK_TOP"

  tar xzf "$WARMED_UP_MOCK_TOP"
}

function run_soong() {
  build/soong/soong_ui.bash --make-mode --skip-ninja --skip-make --skip-soong-tests "$@"
}

function create_mock_bazel() {
  copy_directory build/bazel

  symlink_directory prebuilts/bazel
  symlink_directory prebuilts/jdk

  symlink_file WORKSPACE
  symlink_file tools/bazel
}

run_bazel() {
  tools/bazel "$@"
}

run_bp2build() {
  GENERATE_BAZEL_FILES=true build/soong/soong_ui.bash --make-mode --skip-ninja --skip-make --skip-soong-tests nothing
}

info "Starting Soong integration test suite $(basename $0)"
info "Mock top: $MOCK_TOP"


export ALLOW_MISSING_DEPENDENCIES=true
warmup_mock_top
