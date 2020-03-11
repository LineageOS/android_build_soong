#!/bin/bash

set -eu

# Copyright 2020 Google Inc. All rights reserved.
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

# Tool to evaluate the transitive closure of the ninja dependency graph of the
# files and targets depending on a given target.
#
# i.e. the list of things that could change after changing a target.

readonly me=$(basename "${0}")

readonly usage="usage: ${me} {options} target [target...]

Evaluate the reverse transitive closure of ninja targets depending on one or
more targets.

Options:

  -(no)quiet        Suppresses progress output to stderr and interactive
    alias -(no)q    prompts. By default, when stderr is a tty, progress gets
                    reported to stderr; when both stderr and stdin are tty,
                    the script asks user whether to delete intermediate files.
                    When suppressed or not prompted, script always deletes the
                    temporary / intermediate files.
  -sep=<delim>      Use 'delim' as output field separator between notice
                    checksum and notice filename in notice output.
                    e.g. sep='\t'
                    (Default space)
  -csv              Shorthand for -sep=','

At minimum, before running this script, you must first run:
$ source build/envsetup.sh
$ lunch
$ m nothing
to setup the build environment, choose a target platform, and build the ninja
dependency graph.
"

function die() { echo -e "${*}" >&2; exit 2; }

# Reads one input target per line from stdin; outputs (isnotice target) tuples.
#
# output target is a ninja target that the input target depends on
# isnotice in {0,1} with 1 for output targets believed to be license or notice
#
# only argument is the dependency depth indicator
function getDeps() {
    (tr '\n' '\0' | xargs -0 "${ninja_bin}" -f "${ninja_file}" -t query) \
    | awk -v depth="${1}" '
      BEGIN {
        inoutput = 0
      }
      $0 ~ /^\S\S*:$/ {
        inoutput = 0
      }
      inoutput != 0 {
        print gensub(/^\s*/, "", "g")" "depth
      }
      $1 == "outputs:" {
        inoutput = 1
      }
    '
}


if [ -z "${ANDROID_BUILD_TOP}" ]; then
    die "${me}: Run 'lunch' to configure the build environment"
fi

if [ -z "${TARGET_PRODUCT}" ]; then
    die "${me}: Run 'lunch' to configure the build environment"
fi

ninja_file="${ANDROID_BUILD_TOP}/out/combined-${TARGET_PRODUCT}.ninja"
if [ ! -f "${ninja_file}" ]; then
    die "${me}: Run 'm nothing' to build the dependency graph"
fi

ninja_bin="${ANDROID_BUILD_TOP}/prebuilts/build-tools/linux-x86/bin/ninja"
if [ ! -x "${ninja_bin}" ]; then
    die "${me}: Cannot find ninja executable expected at ${ninja_bin}"
fi


# parse the command-line

declare -a targets # one or more targets to evaluate

quiet=false      # whether to suppress progress

sep=" "          # output separator between depth and target

use_stdin=false  # whether to read targets from stdin i.e. target -

while [ $# -gt 0 ]; do
    case "${1:-}" in
      -)
        use_stdin=true
      ;;
      -*)
        flag=$(expr "${1}" : '^-*\(.*\)$')
        case "${flag:-}" in
          q) ;&
          quiet)
            quiet=true;;
          noq) ;&
          noquiet)
            quiet=false;;
          csv)
            sep=",";;
          sep)
            sep="${2?"${usage}"}"; shift;;
          sep=*)
            sep=$(expr "${flag}" : '^sep=\(.*\)$';;
          *)
            die "Unknown flag ${1}"
          ;;
        esac
      ;;
      *)
        targets+=("${1:-}")
      ;;
    esac
    shift
done

if [ ! -v targets[0] ] && ! ${use_stdin}; then
    die "${usage}\n\nNo target specified."
fi

# showProgress when stderr is a tty
if [ -t 2 ] && ! ${quiet}; then
    showProgress=true
else
    showProgress=false
fi

# interactive when both stderr and stdin are tty
if ${showProgress} && [ -t 0 ]; then
    interactive=true
else
    interactive=false
fi


readonly tmpFiles=$(mktemp -d "${TMPDIR}.tdeps.XXXXXXXXX")
if [ -z "${tmpFiles}" ]; then
    die "${me}: unable to create temporary directory"
fi

# The deps files contain unique (isnotice target) tuples where
# isnotice in {0,1} with 1 when ninja target `target` is a license or notice.
readonly oldDeps="${tmpFiles}/old"
readonly newDeps="${tmpFiles}/new"
readonly allDeps="${tmpFiles}/all"

if ${use_stdin}; then # start deps by reading 1 target per line from stdin
  awk '
    NF > 0 {
      print gensub(/\s*$/, "", "g", gensub(/^\s*/, "", "g"))" "0
    }
  ' >"${newDeps}"
else # start with no deps by clearing file
  : >"${newDeps}"
fi

# extend deps by appending targets from command-line
for idx in "${!targets[*]}"; do
    echo "${targets[${idx}]} 0" >>"${newDeps}"
done

# remove duplicates and start with new, old and all the same
sort -u <"${newDeps}" >"${allDeps}"
cp "${allDeps}" "${newDeps}"
cp "${allDeps}" "${oldDeps}"

# report depth of dependenciens when showProgress
depth=0

while [ $(wc -l < "${newDeps}") -gt 0 ]; do
    if ${showProgress}; then
        echo "depth ${depth} has "$(wc -l < "${newDeps}")" targets" >&2
    fi
    depth=$(expr ${depth} + 1)
    ( # recalculate dependencies by combining unique inputs of new deps w. old
        cut -d\  -f1 "${newDeps}" | getDeps "${depth}"
        cat "${oldDeps}"
    ) | sort -n | awk '
      BEGIN {
        prev = ""
      }
      {
        depth = $NF
        $NF = ""
        gsub(/\s*$/, "")
        if ($0 != prev) {
          print gensub(/\s*$/, "", "g")" "depth
        }
        prev = $0
      }
    ' >"${allDeps}"
    # recalculate new dependencies as net additions to old dependencies
    set +e
    diff "${oldDeps}" "${allDeps}" --old-line-format='' \
      --new-line-format='%L' --unchanged-line-format='' > "${newDeps}"
    set -e
    # recalculate old dependencies for next iteration
    cp "${allDeps}" "${oldDeps}"
done

# found all deps -- clean up last iteration of old and new
rm -f "${oldDeps}"
rm -f "${newDeps}"

if ${showProgress}; then
    echo $(wc -l < "${allDeps}")" targets" >&2
fi

awk -v sep="${sep}" '{
  depth = $NF
  $NF = ""
  gsub(/\s*$/, "")
  print depth sep $0
}' "${allDeps}" | sort -n

if ${interactive}; then
    echo -n "$(date '+%F %-k:%M:%S') Delete ${tmpFiles} ? [n] " >&2
    read answer
    case "${answer}" in [yY]*) rm -fr "${tmpFiles}";; esac
else
    rm -fr "${tmpFiles}"
fi
