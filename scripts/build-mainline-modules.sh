#!/bin/bash -e

# Non exhaustive list of modules where we want prebuilts. More can be added as
# needed.
MAINLINE_MODULES=(
  com.android.art
  com.android.art.debug
  com.android.art.testing
  com.android.conscrypt
  com.android.i18n
  com.android.os.statsd
  com.android.runtime
  com.android.tzdata
)

# List of SDKs and module exports we know of.
MODULES_SDK_AND_EXPORTS=(
  art-module-sdk
  art-module-test-exports
  conscrypt-module-host-exports
  conscrypt-module-sdk
  conscrypt-module-test-exports
  i18n-module-host-exports
  i18n-module-sdk
  i18n-module-test-exports
  platform-mainline-sdk
  platform-mainline-test-exports
  runtime-module-host-exports
  runtime-module-sdk
  stats-log-api-gen-exports
  statsd-module-sdk
  tzdata-module-test-exports
)

# List of libraries installed on the platform that are needed for ART chroot
# testing.
PLATFORM_LIBRARIES=(
  liblog
  libartpalette-system
)

# We want to create apex modules for all supported architectures.
PRODUCTS=(
  aosp_arm
  aosp_arm64
  aosp_x86
  aosp_x86_64
)

if [ ! -e "build/make/core/Makefile" ]; then
  echo "$0 must be run from the top of the tree"
  exit 1
fi

echo_and_run() {
  echo "$*"
  "$@"
}

lib_dir() {
  case $1 in
    (aosp_arm|aosp_x86) echo "lib";;
    (aosp_arm64|aosp_x86_64) echo "lib64";;
  esac
}

# Make sure this build builds from source, regardless of the default.
export SOONG_CONFIG_art_module_source_build=true

OUT_DIR=$(source build/envsetup.sh > /dev/null; TARGET_PRODUCT= get_build_var OUT_DIR)
DIST_DIR=$(source build/envsetup.sh > /dev/null; TARGET_PRODUCT= get_build_var DIST_DIR)

for product in "${PRODUCTS[@]}"; do
  echo_and_run build/soong/soong_ui.bash --make-mode $@ \
    TARGET_PRODUCT=${product} \
    ${MAINLINE_MODULES[@]} \
    ${PLATFORM_LIBRARIES[@]}

  PRODUCT_OUT=$(source build/envsetup.sh > /dev/null; TARGET_PRODUCT=${product} get_build_var PRODUCT_OUT)
  TARGET_ARCH=$(source build/envsetup.sh > /dev/null; TARGET_PRODUCT=${product} get_build_var TARGET_ARCH)
  rm -rf ${DIST_DIR}/${TARGET_ARCH}/
  mkdir -p ${DIST_DIR}/${TARGET_ARCH}/
  for module in "${MAINLINE_MODULES[@]}"; do
    echo_and_run cp ${PWD}/${PRODUCT_OUT}/system/apex/${module}.apex ${DIST_DIR}/${TARGET_ARCH}/
  done
  for library in "${PLATFORM_LIBRARIES[@]}"; do
    libdir=$(lib_dir $product)
    echo_and_run cp ${PWD}/${PRODUCT_OUT}/system/${libdir}/${library}.so ${DIST_DIR}/${TARGET_ARCH}/
  done
done

# Create multi-archs SDKs in a different out directory. The multi-arch script
# uses Soong in --skip-make mode which cannot use the same directory as normal
# mode with make.
export OUT_DIR=${OUT_DIR}/aml
echo_and_run build/soong/scripts/build-aml-prebuilts.sh ${MODULES_SDK_AND_EXPORTS[@]}

rm -rf ${DIST_DIR}/mainline-sdks
echo_and_run cp -R ${OUT_DIR}/soong/mainline-sdks ${DIST_DIR}
