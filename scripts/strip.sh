#!/bin/bash -e

# Copyright 2017 Google Inc. All rights reserved.
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

# Script to handle the various ways soong may need to strip binaries
# Inputs:
#  Environment:
#   CLANG_BIN: path to the clang bin directory
#   CROSS_COMPILE: prefix added to readelf, objcopy tools
#   XZ: path to the xz binary
#  Arguments:
#   -i ${file}: input file (required)
#   -o ${file}: output file (required)
#   -d ${file}: deps file (required)
#   -k symbols: Symbols to keep (optional)
#   --add-gnu-debuglink
#   --keep-mini-debug-info
#   --keep-symbols
#   --use-gnu-strip
#   --remove-build-id

set -o pipefail

OPTSTRING=d:i:o:k:-:

usage() {
    cat <<EOF
Usage: strip.sh [options] -k symbols -i in-file -o out-file -d deps-file
Options:
        --add-gnu-debuglink     Add a gnu-debuglink section to out-file
        --keep-mini-debug-info  Keep compressed debug info in out-file
        --keep-symbols          Keep symbols in out-file
        --use-gnu-strip         Use strip/objcopy instead of llvm-{strip,objcopy}
        --remove-build-id       Remove the gnu build-id section in out-file
EOF
    exit 1
}

# Without --use-gnu-strip, GNU strip is replaced with llvm-strip to work around
# old GNU strip bug on lld output files, b/80093681.
# Similary, calls to objcopy are replaced with llvm-objcopy,
# with some exceptions.

do_strip() {
    # ${CROSS_COMPILE}strip --strip-all does not strip .ARM.attributes,
    # so we tell llvm-strip to keep it too.
    if [ -z "${use_gnu_strip}" ]; then
        "${CLANG_BIN}/llvm-strip" --strip-all -keep-section=.ARM.attributes "${infile}" -o "${outfile}.tmp"
    else
        "${CROSS_COMPILE}strip" --strip-all "${infile}" -o "${outfile}.tmp"
    fi
}

do_strip_keep_symbols() {
    REMOVE_SECTIONS=`"${CROSS_COMPILE}readelf" -S "${infile}" | awk '/.debug_/ {print "--remove-section " $2}' | xargs`
    if [ -z "${use_gnu_strip}" ]; then
        "${CLANG_BIN}/llvm-objcopy" "${infile}" "${outfile}.tmp" ${REMOVE_SECTIONS}
    else
        "${CROSS_COMPILE}objcopy" "${infile}" "${outfile}.tmp" ${REMOVE_SECTIONS}
    fi
}

do_strip_keep_symbol_list() {
    if [ -z "${use_gnu_strip}" ]; then
        echo "do_strip_keep_symbol_list does not work with llvm-objcopy"
        echo "http://b/131631155"
        usage
    fi

    echo "${symbols_to_keep}" | tr ',' '\n' > "${outfile}.symbolList"
    KEEP_SYMBOLS="-w --strip-unneeded-symbol=* --keep-symbols="
    KEEP_SYMBOLS+="${outfile}.symbolList"

    "${CROSS_COMPILE}objcopy" "${infile}" "${outfile}.tmp" ${KEEP_SYMBOLS}
}

do_strip_keep_mini_debug_info() {
    rm -f "${outfile}.dynsyms" "${outfile}.funcsyms" "${outfile}.keep_symbols" "${outfile}.debug" "${outfile}.mini_debuginfo" "${outfile}.mini_debuginfo.xz"
    local fail=
    if [ -z "${use_gnu_strip}" ]; then
        "${CLANG_BIN}/llvm-strip" --strip-all -keep-section=.ARM.attributes -remove-section=.comment "${infile}" -o "${outfile}.tmp" || fail=true
    else
        "${CROSS_COMPILE}strip" --strip-all -R .comment "${infile}" -o "${outfile}.tmp" || fail=true
    fi
    if [ -z $fail ]; then
        # Current prebult llvm-objcopy does not support the following flags:
        #    --only-keep-debug --rename-section --keep-symbols
        # For the following use cases, ${CROSS_COMPILE}objcopy does fine with lld linked files,
        # except the --add-section flag.
        "${CROSS_COMPILE}objcopy" --only-keep-debug "${infile}" "${outfile}.debug"
        "${CROSS_COMPILE}nm" -D "${infile}" --format=posix --defined-only 2> /dev/null | awk '{ print $1 }' | sort >"${outfile}.dynsyms"
        "${CROSS_COMPILE}nm" "${infile}" --format=posix --defined-only | awk '{ if ($2 == "T" || $2 == "t" || $2 == "D") print $1 }' | sort > "${outfile}.funcsyms"
        comm -13 "${outfile}.dynsyms" "${outfile}.funcsyms" > "${outfile}.keep_symbols"
        echo >> "${outfile}.keep_symbols" # Ensure that the keep_symbols file is not empty.
        "${CROSS_COMPILE}objcopy" --rename-section .debug_frame=saved_debug_frame "${outfile}.debug" "${outfile}.mini_debuginfo"
        "${CROSS_COMPILE}objcopy" -S --remove-section .gdb_index --remove-section .comment --keep-symbols="${outfile}.keep_symbols" "${outfile}.mini_debuginfo"
        "${CROSS_COMPILE}objcopy" --rename-section saved_debug_frame=.debug_frame "${outfile}.mini_debuginfo"
        "${XZ}" "${outfile}.mini_debuginfo"
        if [ -z "${use_gnu_strip}" ]; then
            "${CLANG_BIN}/llvm-objcopy" --add-section .gnu_debugdata="${outfile}.mini_debuginfo.xz" "${outfile}.tmp"
        else
            "${CROSS_COMPILE}objcopy" --add-section .gnu_debugdata="${outfile}.mini_debuginfo.xz" "${outfile}.tmp"
        fi
        rm -f "${outfile}.dynsyms" "${outfile}.funcsyms" "${outfile}.keep_symbols" "${outfile}.debug" "${outfile}.mini_debuginfo" "${outfile}.mini_debuginfo.xz"
    else
        cp -f "${infile}" "${outfile}.tmp"
    fi
}

do_add_gnu_debuglink() {
    if [ -z "${use_gnu_strip}" ]; then
        "${CLANG_BIN}/llvm-objcopy" --add-gnu-debuglink="${infile}" "${outfile}.tmp"
    else
        "${CROSS_COMPILE}objcopy" --add-gnu-debuglink="${infile}" "${outfile}.tmp"
    fi
}

do_remove_build_id() {
    if [ -z "${use_gnu_strip}" ]; then
        "${CLANG_BIN}/llvm-strip" -remove-section=.note.gnu.build-id "${outfile}.tmp" -o "${outfile}.tmp.no-build-id"
    else
        "${CROSS_COMPILE}strip" --remove-section=.note.gnu.build-id "${outfile}.tmp" -o "${outfile}.tmp.no-build-id"
    fi
    rm -f "${outfile}.tmp"
    mv "${outfile}.tmp.no-build-id" "${outfile}.tmp"
}

while getopts $OPTSTRING opt; do
    case "$opt" in
        d) depsfile="${OPTARG}" ;;
        i) infile="${OPTARG}" ;;
        o) outfile="${OPTARG}" ;;
        k) symbols_to_keep="${OPTARG}" ;;
        -)
            case "${OPTARG}" in
                add-gnu-debuglink) add_gnu_debuglink=true ;;
                keep-mini-debug-info) keep_mini_debug_info=true ;;
                keep-symbols) keep_symbols=true ;;
                remove-build-id) remove_build_id=true ;;
                use-gnu-strip) use_gnu_strip=true ;;
                *) echo "Unknown option --${OPTARG}"; usage ;;
            esac;;
        ?) usage ;;
        *) echo "'${opt}' '${OPTARG}'"
    esac
done

if [ -z "${infile}" ]; then
    echo "-i argument is required"
    usage
fi

if [ -z "${outfile}" ]; then
    echo "-o argument is required"
    usage
fi

if [ -z "${depsfile}" ]; then
    echo "-d argument is required"
    usage
fi

if [ ! -z "${keep_symbols}" -a ! -z "${keep_mini_debug_info}" ]; then
    echo "--keep-symbols and --keep-mini-debug-info cannot be used together"
    usage
fi

if [ ! -z "${symbols_to_keep}" -a ! -z "${keep_symbols}" ]; then
    echo "--keep-symbols and -k cannot be used together"
    usage
fi

if [ ! -z "${add_gnu_debuglink}" -a ! -z "${keep_mini_debug_info}" ]; then
    echo "--add-gnu-debuglink cannot be used with --keep-mini-debug-info"
    usage
fi

rm -f "${outfile}.tmp"

if [ ! -z "${keep_symbols}" ]; then
    do_strip_keep_symbols
elif [ ! -z "${symbols_to_keep}" ]; then
    do_strip_keep_symbol_list
elif [ ! -z "${keep_mini_debug_info}" ]; then
    do_strip_keep_mini_debug_info
else
    do_strip
fi

if [ ! -z "${add_gnu_debuglink}" ]; then
    do_add_gnu_debuglink
fi

if [ ! -z "${remove_build_id}" ]; then
    do_remove_build_id
fi

rm -f "${outfile}"
mv "${outfile}.tmp" "${outfile}"

if [ -z "${use_gnu_strip}" ]; then
  USED_STRIP_OBJCOPY="${CLANG_BIN}/llvm-strip ${CLANG_BIN}/llvm-objcopy"
else
  USED_STRIP_OBJCOPY="${CROSS_COMPILE}strip"
fi

cat <<EOF > "${depsfile}"
${outfile}: \
  ${infile} \
  ${CROSS_COMPILE}nm \
  ${CROSS_COMPILE}objcopy \
  ${CROSS_COMPILE}readelf \
  ${USED_STRIP_OBJCOPY}

EOF
