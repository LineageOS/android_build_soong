#!/bin/bash -eu

HARDWIRED_MOCK_TOP=
# Uncomment this to be able to view the source tree after a test is run
# HARDWIRED_MOCK_TOP=/tmp/td

REAL_TOP="$(readlink -f "$(dirname "$0")"/../../..)"

function fail {
  echo ERROR: $1
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

function setup() {
  if [[ ! -z "$HARDWIRED_MOCK_TOP" ]]; then
    MOCK_TOP="$HARDWIRED_MOCK_TOP"
    rm -fr "$MOCK_TOP"
    mkdir -p "$MOCK_TOP"
  else
    MOCK_TOP=$(mktemp -t -d st.XXXXX)
    trap 'cd / && rm -fr "$MOCK_TOP"' EXIT
  fi

  echo "Test case: ${FUNCNAME[1]}, mock top path: $MOCK_TOP"
  cd "$MOCK_TOP"

  copy_directory build/blueprint
  copy_directory build/soong

  symlink_directory prebuilts/go
  symlink_directory prebuilts/build-tools
  symlink_directory external/golang-protobuf

  touch "$MOCK_TOP/Android.bp"

  export ALLOW_MISSING_DEPENDENCIES=true

  mkdir -p out/soong
}

function run_soong() {
  build/soong/soong_ui.bash --make-mode --skip-ninja --skip-make --skip-soong-tests
}
