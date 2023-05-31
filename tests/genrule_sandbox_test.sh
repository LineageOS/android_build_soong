#!/bin/bash

# Copyright (C) 2023 The Android Open Source Project
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# Build the given genrule modules with GENRULE_SANDBOXING enabled and disabled,
# then compare the output of the modules and report result.

function die() { format=$1; shift; printf >&2 "$format\n" $@; exit 1; }

function usage() {
  die "usage: ${0##*/} <-t lunch_target> [module]..."
}

if [ ! -e "build/make/core/Makefile" ]; then
  die "$0 must be run from the top of the Android source tree."
fi

declare TARGET=
while getopts "t:" opt; do
  case $opt in
    t)
      TARGET=$OPTARG ;;
    *) usage ;;
  esac
done

shift $((OPTIND-1))
MODULES="$@"

source build/envsetup.sh

if [[ -n $TARGET ]]; then
  lunch $TARGET
fi

if [[ -z ${OUT_DIR+x} ]]; then
  OUT_DIR="out"
fi

OUTPUT_DIR="$(mktemp -d tmp.XXXXXX)"
PASS=true

function cleanup {
  if [ $PASS = true ]; then
    rm -rf "${OUTPUT_DIR}"
  fi
}
trap cleanup EXIT

declare -A GEN_PATH_MAP

function find_gen_paths() {
  for module in $MODULES; do
    module_path=$(pathmod "$module")
    package_path=${module_path#$ANDROID_BUILD_TOP}
    gen_path=$OUT_DIR/soong/.intermediates$package_path/$module
    GEN_PATH_MAP[$module]=$gen_path
  done
}

function store_outputs() {
  local dir=$1; shift

  for module in $MODULES; do
    dest_dir=$dir/${module}
    mkdir -p $dest_dir
    gen_path=${GEN_PATH_MAP[$module]}
    cp -r $gen_path $dest_dir
  done
}

function cmp_outputs() {
  local dir1=$1; shift
  local dir2=$1; shift

  for module in $MODULES; do
    if ! diff -rq --exclude=genrule.sbox.textproto $dir1/$module $dir2/$module; then
      PASS=false
      echo "$module differ"
    fi
  done
  if [ $PASS = true ]; then
    echo "Test passed"
  fi
}

if [ ! -f "$ANDROID_PRODUCT_OUT/module-info.json" ]; then
  refreshmod
fi

find_gen_paths
m --skip-soong-tests GENRULE_SANDBOXING=true "${MODULES[@]}"
store_outputs "$OUTPUT_DIR/sandbox"
m --skip-soong-tests GENRULE_SANDBOXING=false "${MODULES[@]}"
store_outputs "$OUTPUT_DIR/non_sandbox"

cmp_outputs "$OUTPUT_DIR/non_sandbox" "$OUTPUT_DIR/sandbox"
