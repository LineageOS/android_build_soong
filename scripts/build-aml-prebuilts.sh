#!/bin/bash -e

# This script is similar to "m" but builds in --soong-only mode, and handles
# special cases to make that mode work. All arguments are passed on to
# build/soong/soong_ui.bash.
#
# --soong-only bypasses the kati step and hence the make logic that e.g. doesn't
# handle more than two device architectures. It is particularly intended for use
# with TARGET_PRODUCT=mainline_sdk to build 'sdk' and 'module_export' Soong
# modules in TARGET_ARCH_SUITE=mainline_sdk mode so that they get all four
# device architectures (artifacts get installed in $OUT_DIR/soong/mainline-sdks
# - cf PathForMainlineSdksInstall in android/paths.go).
#
# TODO(b/174315599): Replace this script completely with a 'soong_ui.bash
# --soong-only' invocation. For now it is still necessary to set up
# build_number.txt.

if [ ! -e build/soong/soong_ui.bash ]; then
  echo "$0 must be run from the top of the tree"
  exit 1
fi

export OUT_DIR=${OUT_DIR:-out}

if [ -e ${OUT_DIR}/soong/.soong.kati_enabled ]; then
  # If ${OUT_DIR} has been created without --skip-make, Soong will create an
  # ${OUT_DIR}/soong/build.ninja that leaves out many targets which are
  # expected to be supplied by the .mk files, and that might cause errors in
  # "m --skip-make" below. We therefore default to a different out dir
  # location in that case.
  AML_OUT_DIR=out/aml
  echo "Avoiding in-make OUT_DIR '${OUT_DIR}' - building in '${AML_OUT_DIR}' instead"
  OUT_DIR=${AML_OUT_DIR}
fi

mkdir -p ${OUT_DIR}/soong

# The --dumpvars-mode invocation will run Soong in normal make mode where it
# creates .soong.kati_enabled. That would clobber our real out directory, so we
# need to use a different OUT_DIR.
vars="$(OUT_DIR=${OUT_DIR}/dumpvars_mode build/soong/soong_ui.bash \
        --dumpvars-mode --vars=BUILD_NUMBER)"
# Assign to a variable and eval that, since bash ignores any error status
# from the command substitution if it's directly on the eval line.
eval $vars

# Some Soong build rules may require this, and the failure mode if it's missing
# is confusing (b/172548608).
echo -n ${BUILD_NUMBER} > ${OUT_DIR}/soong/build_number.txt

build/soong/soong_ui.bash --make-mode --soong-only "$@"
