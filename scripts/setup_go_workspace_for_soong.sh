#!/bin/bash
set -e

# Copyright 2019 Google Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Mounts the components of soong into a directory structure that Go tools
# and editors expect.


#####################################################################
# Print the message to stderr with the prefix ERROR and abort this
# script.
#####################################################################
function log_FATAL() {
  echo "ERROR:" "$*" >&2
  exit 1
}

#####################################################################
# Print the message to stderr with the prefix WARN
#####################################################################
function log_WARN() {
  echo "WARN:" "$*" >&2
}


#####################################################################
# Print the message with the prefix INFO.
#####################################################################
function log_INFO() {
  echo "INFO:" "$*"
}


#####################################################################
# Find the root project directory of this repo. This is done by
# finding the directory of where this script lives and then go up one
# directory to check the ".repo" directory exist. If not, keep going
# up until we find the ".repo" file or we reached to the filesystem
# root. Project root directory is printed to stdout.
#####################################################################
function root_dir() (
  local dir
  if ! dir="$("${readlink}" -e $(dirname "$0"))"; then
    log_FATAL "failed to read the script's current directory."
  fi

  dir=${dir}/../../..
  if ! dir="$("${readlink}" -e "${dir}")"; then
    log_FATAL "Cannot find the root project directory"
  fi

  echo "${dir}"
)


#####################################################################
# executes a shell command by printing out to the screen first and
# then evaluating the command.
#####################################################################
function execute() {
  echo "$@"
  eval "$@"
}


#####################################################################
# Returns the source directory of a passed in path from BIND_PATHS
# array.
#####################################################################
function bind_path_src_dir() (
  local -r bind_path="$1"
  echo "${bind_path/%|*/}"
)


#####################################################################
# Returns the destination directory of a passed in path from
# BIND_PATHS array.
#####################################################################
function bind_path_dst_dir() (
  local -r bind_path="$1"
  echo  "${bind_path/#*|}"
)


#####################################################################
# Executes the bindfs command in linux. Expects $1 to be src
# directory and $2 to be destination directory.
#####################################################################
function linux_bind_dir() (
  execute bindfs "$1" "$2"
)

#####################################################################
# Executes the fusermount -u command in linux. Expects $1 to be the
# destination directory.
#####################################################################
function linux_unbind_dir() (
  execute fusermount -u "$1"
)

#####################################################################
# Executes the bindfs command in darwin. Expects $1 to be src
# directory and $2 to be destination directory.
#####################################################################
function darwin_bind_dir() (
  execute bindfs -o allow_recursion -n "$1" "$2"
)


#####################################################################
# Execute the umount command in darwin to unbind a directory. Expects
# $1 to be the destination directory
#####################################################################
function darwin_unbind_dir() (
  execute umount -f "$1"
)


#####################################################################
# Bind all the paths that are specified in the BIND_PATHS array.
#####################################################################
function bind_all() (
  local src_dir
  local dst_dir

  for path in ${BIND_PATHS[@]}; do
    src_dir=$(bind_path_src_dir "${path}")

    dst_dir=$(bind_path_dst_dir "${path}")
    mkdir -p "${dst_dir}"

    "${bind_dir}" ${src_dir} "${dst_dir}"
  done

  echo
  log_INFO "Created GOPATH-compatible directory structure at ${OUTPUT_PATH}."
)


#####################################################################
# Unbind all the paths that are specified in the BIND_PATHS array.
#####################################################################
function unbind_all() (
  local dst_dir
  local exit_code=0

  # need to go into reverse since several parent directory may have been
  # first before the child one.
  for (( i=${#BIND_PATHS[@]}-1; i>=0; i-- )); do
    dst_dir=$(bind_path_dst_dir "${BIND_PATHS[$i]}")

    # continue to unmount even one of them fails
    if ! "${unbind_dir}" "${dst_dir}"; then
      log_WARN "Failed to umount ${dst_dir}."
      exit_code=1
    fi
  done

  if [[ ${exit_code} -ne 0 ]]; then
    exit ${exit_code}
  fi

  echo
  log_INFO "Unmounted the GOPATH-compatible directory structure at ${OUTPUT_PATH}."
)


#####################################################################
# Asks the user to create the GOPATH-compatible directory structure.
#####################################################################
function confirm() (
  while true; do
    echo "Will create GOPATH-compatible directory structure at ${OUTPUT_PATH}"
    echo -n "Ok [Y/n]?"
    read decision
    if [ "${decision}" == "y" -o "${decision}" == "Y" -o "${decision}" == "" ]; then
      return 0
    else
      if [ "${decision}" == "n" ]; then
        return 1
      else
        log_WARN "Invalid choice ${decision}; choose either 'y' or 'n'"
      fi
    fi
  done
)


#####################################################################
# Help function.
#####################################################################
function help() (
  cat <<EOF
Mounts the components of soong into a directory structure that Go tools
and editors expect.

  --help
    This help

  --bind
    Create the directory structure that Go tools and editors expect by
    binding the one to aosp build directory.

  --unbind
    Reverse operation of bind.

If no flags were specified, the --bind one is selected by default.
EOF
)


#####################################################################
# Parse the arguments passed in to this script.
#####################################################################
function parse_arguments() {
  while [[ -n "$1" ]]; do
    case "$1" in
          --bind)
            ACTION="bind"
            shift
            ;;
          --unbind)
            ACTION="unbind"
            shift
            ;;
          --help )
            help
            shift
            exit 0
            ;;
          *)
            log_WARN "Unknown option: $1"
            help
            exit 1
            ;;
    esac
  done

  if [[ -z "${ACTION}" ]]; then
    ACTION=bind
  fi
}


#####################################################################
# Verifies that a list of required binaries are installed in the
# host in order to run this script.
#####################################################################
function check_exec_existence() (
  function check() {
    if ! hash "$1" &>/dev/null; then
      log_FATAL "missing $1"
    fi
  }

  local bins
  case "${os_type}" in
    Darwin)
      bins=("bindfs" "greadlink")
      ;;
    Linux)
      bins=("bindfs" "fusermount")
      ;;
    *)
      log_FATAL "${os_type} is not a recognized system."
  esac

  for bin in "${bins[@]}"; do
    check "${bin}"
  done
)


function main() {
  parse_arguments "$@"

  check_exec_existence

  if [[ "${ACTION}" == "bind" ]]; then
    if confirm; then
      echo
      bind_all
    else
      echo "skipping due to user request"
      exit 1
    fi
  else
    echo
    unbind_all
  fi
}

readonly os_type="$(uname -s)"
case "${os_type}" in
  Darwin)
    bind_dir=darwin_bind_dir
    unbind_dir=darwin_unbind_dir
    readlink=greadlink
    ;;
  Linux)
    bind_dir=linux_bind_dir
    unbind_dir=linux_unbind_dir
    readlink=readlink
    ;;
    *)
    log_FATAL "${os_type} is not a recognized system."
esac
readonly bind_dir
readonly unbind_dir
readonly readlink


if ! ANDROID_PATH="$(root_dir)"; then
  log_FATAL "failed to find the root of the repo checkout"
fi
readonly ANDROID_PATH

#if GOPATH contains multiple paths, use the first one
if ! OUTPUT_PATH="$(echo ${GOPATH} | sed 's/\:.*//')"; then
  log_FATAL "failed to extract the first GOPATH environment variable"
fi
readonly OUTPUT_PATH
if [ -z "${OUTPUT_PATH}" ]; then
  log_FATAL "Could not determine the desired location at which to create a" \
            "Go-compatible workspace. Please update GOPATH to specify the" \
            "desired destination directory."
fi

# Below are the paths to bind from src to dst. The paths are separated by |
# where the left side is the source and the right side is destination.
readonly BIND_PATHS=(
  "${ANDROID_PATH}/build/blueprint|${OUTPUT_PATH}/src/github.com/google/blueprint"
  "${ANDROID_PATH}/build/soong|${OUTPUT_PATH}/src/android/soong"
  "${ANDROID_PATH}/art/build|${OUTPUT_PATH}/src/android/soong/art"
  "${ANDROID_PATH}/external/golang-protobuf|${OUTPUT_PATH}/src/github.com/golang/protobuf"
  "${ANDROID_PATH}/external/llvm/soong|${OUTPUT_PATH}/src/android/soong/llvm"
  "${ANDROID_PATH}/external/clang/soong|${OUTPUT_PATH}/src/android/soong/clang"
  "${ANDROID_PATH}/external/robolectric-shadows/soong|${OUTPUT_PATH}/src/android/soong/robolectric"
)

main "$@"
