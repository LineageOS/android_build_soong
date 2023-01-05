#! /bin/bash -eu

# Run CKati step separately, tracing given Makefile variables.
# It is expected that the regular Android null build (`m nothing`)
# has been run so that $OUT_DIR/soong/Android-${TARGET_PRODUCT}.mk,
# $OUT_DIR/soong/make_vars-${TARGET_PRODUCT}.mk, etc. files exist.
#
# The output file is in JSON format and can be processed with, say,
# `jq`. For instance, the following invocation outputs all assignment
# traces concisely:
#  jq -c  '.assignments[] | (select (.operation == "assign")) | {("n"): .name, ("l"): .value_stack[0]?, ("v"): .value }' out/ckati.trace
# generates
#  {"n":"<var1>","l":"<file>:<line>","v":"<value1>"}
#  ...

function die() { format=$1; shift; printf "$format\n" $@; exit 1; }
function usage() { die "Usage: %s [-o FILE] VAR ...\n(without -o the output goes to ${outfile})"  ${0##*/}; }

[[ -d build/soong ]] || die "run this script from the top of the Android source tree"
declare -r out=${OUT_DIR:-out}
[[ -x ${out}/soong_ui ]] || die "run Android build first"
: ${TARGET_PRODUCT:?not set, run lunch?}
: ${TARGET_BUILD_VARIANT:?not set, run lunch?}
declare -r androidmk=${out}/soong/Android-${TARGET_PRODUCT}.mk
declare -r makevarsmk=${out}/soong/make_vars-${TARGET_PRODUCT}.mk
declare -r target_device_dir=$(${out}/soong_ui --dumpvar-mode TARGET_DEVICE_DIR)
: ${target_device_dir:?cannot find device directory for ${TARGET_PRODUCT}}
declare -r target_device=$(${out}/soong_ui --dumpvar-mode TARGET_DEVICE)
: ${target_device:?cannot find target device for ${TARGET_PRODUCT}}
declare -r timestamp_file=${out}/build_date.txt
# Files should exist, so ls should succeed:
ls -1d "$androidmk" "$makevarsmk" "$target_device_dir" "$timestamp_file" >/dev/null

outfile=${out}/ckati.trace
while getopts "ho:" opt; do
  case $opt in
    h) usage ;;
    o) outfile=$OPTARG ;;
    ?) usage ;;
  esac
done

if (($#>0)); then
  declare -a tracing=(--variable_assignment_trace_filter="$*" --dump_variable_assignment_trace "$outfile")
else
  printf "running ckati without tracing variables\n"
fi

# Touch one input for ckati, otherwise it will just print
# 'No need to regenerate ninja file' and exit.
touch "$androidmk"
prebuilts/build-tools/linux-x86/bin/ckati \
  --gen_all_targets \
  -i \
  --ignore_optional_include=out/%.P \
  --ninja \
  --ninja_dir=out \
  --ninja_suffix=-${TARGET_PRODUCT} \
  --no_builtin_rules \
  --no_ninja_prelude \
  --regen \
  --top_level_phony \
  --use_find_emulator \
  --use_ninja_phony_output \
  --use_ninja_symlink_outputs \
  --werror_find_emulator \
  --werror_implicit_rules \
  --werror_overriding_commands \
  --werror_phony_looks_real \
  --werror_real_to_phony \
  --werror_suffix_rules \
  --werror_writable \
  --writable out/ \
  -f build/make/core/main.mk \
  "${tracing[@]}" \
  ANDROID_JAVA_HOME=prebuilts/jdk/jdk17/linux-x86 \
  ASAN_SYMBOLIZER_PATH=$PWD/prebuilts/clang/host/linux-x86/llvm-binutils-stable/llvm-symbolizer \
  BUILD_DATETIME_FILE="$timestamp_file" \
  BUILD_HOSTNAME=$(hostname) \
  BUILD_USERNAME="$USER" \
  JAVA_HOME=$PWD/prebuilts/jdk/jdk17/linux-x86 \
  KATI_PACKAGE_MK_DIR="{$out}/target/product/${target_device}/CONFIG/kati_packaging" \
  OUT_DIR="$out" \
  PATH="$PWD/prebuilts/build-tools/path/linux-x86:$PWD/${out}/.path" \
  PYTHONDONTWRITEBYTECODE=1 \
  SOONG_ANDROID_MK="$androidmk" \
  SOONG_MAKEVARS_MK="$makevarsmk" \
  TARGET_BUILD_VARIANT="$TARGET_BUILD_VARIANT" \
  TARGET_DEVICE_DIR="$target_device_dir" \
  TARGET_PRODUCT=${TARGET_PRODUCT} \
  TMPDIR="$PWD/$out/soong/.temp"
