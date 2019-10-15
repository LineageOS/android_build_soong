#!/bin/bash -e

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

# Script to extract and repack an archive with specified object files.
# Inputs:
#  Environment:
#   CLANG_BIN: path to the clang bin directory
#  Arguments:
#   -i ${file}: input file
#   -o ${file}: output file
#   -d ${file}: deps file

set -o pipefail

OPTSTRING=d:i:o:

usage() {
    cat <<EOF
Usage: archive_repack.sh [options] <objects to repack>

OPTIONS:
    -i <file>: input file
    -o <file>: output file
    -d <file>: deps file
EOF
    exit 1
}

while getopts $OPTSTRING opt; do
    case "$opt" in
        d) depsfile="${OPTARG}" ;;
        i) infile="${OPTARG}" ;;
        o) outfile="${OPTARG}" ;;
        ?) usage ;;
    esac
done
shift "$(($OPTIND -1))"

if [ -z "${infile}" ]; then
    echo "-i argument is required"
    usage
fi

if [ -z "${outfile}" ]; then
    echo "-o argument is required"
    usage
fi

# Produce deps file
if [ ! -z "${depsfile}" ]; then
    cat <<EOF > "${depsfile}"
${outfile}: ${infile} ${CLANG_BIN}/llvm-ar
EOF
fi

# Get absolute path for outfile and llvm-ar.
LLVM_AR="${PWD}/${CLANG_BIN}/llvm-ar"
if [[ "$outfile" != /* ]]; then
    outfile="${PWD}/${outfile}"
fi

tempdir="${outfile}.tmp"

# Clean up any previous temporary files.
rm -f "${outfile}"
rm -rf "${tempdir}"

# Do repack
# We have to change working directory since ar only allows extracting to CWD.
mkdir "${tempdir}"
cp "${infile}" "${tempdir}/archive"
cd "${tempdir}"
"${LLVM_AR}" x "archive"
"${LLVM_AR}" --format=gnu qc "${outfile}" "$@"
