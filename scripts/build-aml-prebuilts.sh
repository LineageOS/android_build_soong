#!/bin/bash -e

# This is a wrapper around "m" that builds the given modules in multi-arch mode
# for all architectures supported by Mainline modules. The make (kati) stage is
# skipped, so the build targets in the arguments can only be Soong modules or
# intermediate output files - make targets and normal installed paths are not
# supported.
#
# This script is typically used with "sdk" or "module_export" modules, which
# Soong will install in $OUT_DIR/soong/mainline-sdks (cf
# PathForMainlineSdksInstall in android/paths.go).

export OUT_DIR=${OUT_DIR:-out}

if [ -e ${OUT_DIR}/soong/.soong.in_make ]; then
  # If ${OUT_DIR} has been created without --skip-make, Soong will create an
  # ${OUT_DIR}/soong/build.ninja that leaves out many targets which are
  # expected to be supplied by the .mk files, and that might cause errors in
  # "m --skip-make" below. We therefore default to a different out dir
  # location in that case.
  AML_OUT_DIR=out/aml
  echo "Avoiding in-make OUT_DIR '${OUT_DIR}' - building in '${AML_OUT_DIR}' instead"
  OUT_DIR=${AML_OUT_DIR}
fi

if [ ! -e "build/envsetup.sh" ]; then
  echo "$0 must be run from the top of the tree"
  exit 1
fi

source build/envsetup.sh

my_get_build_var() {
  # get_build_var will run Soong in normal in-make mode where it creates
  # .soong.in_make. That would clobber our real out directory, so we need to
  # run it in a different one.
  OUT_DIR=${OUT_DIR}/get_build_var get_build_var "$@"
}

readonly PLATFORM_SDK_VERSION="$(my_get_build_var PLATFORM_SDK_VERSION)"
readonly PLATFORM_VERSION="$(my_get_build_var PLATFORM_VERSION)"
PLATFORM_VERSION_ALL_CODENAMES="$(my_get_build_var PLATFORM_VERSION_ALL_CODENAMES)"

# PLATFORM_VERSION_ALL_CODENAMES is a comma separated list like O,P. We need to
# turn this into ["O","P"].
PLATFORM_VERSION_ALL_CODENAMES="${PLATFORM_VERSION_ALL_CODENAMES/,/'","'}"
PLATFORM_VERSION_ALL_CODENAMES="[\"${PLATFORM_VERSION_ALL_CODENAMES}\"]"

# Logic from build/make/core/goma.mk
if [ "${USE_GOMA}" = true ]; then
  if [ -n "${GOMA_DIR}" ]; then
    goma_dir="${GOMA_DIR}"
  else
    goma_dir="${HOME}/goma"
  fi
  GOMA_CC="${goma_dir}/gomacc"
  export CC_WRAPPER="${CC_WRAPPER}${CC_WRAPPER:+ }${GOMA_CC}"
  export CXX_WRAPPER="${CXX_WRAPPER}${CXX_WRAPPER:+ }${GOMA_CC}"
  export JAVAC_WRAPPER="${JAVAC_WRAPPER}${JAVAC_WRAPPER:+ }${GOMA_CC}"
else
  USE_GOMA=false
fi

readonly SOONG_OUT=${OUT_DIR}/soong
mkdir -p ${SOONG_OUT}
readonly SOONG_VARS=${SOONG_OUT}/soong.variables

# Aml_abis: true
#   -  This flag configures Soong to compile for all architectures required for
#      Mainline modules.
# CrossHost: linux_bionic
# CrossHostArch: x86_64
#   -  Enable Bionic on host as ART needs prebuilts for it.
cat > ${SOONG_VARS}.new << EOF
{
    "Platform_sdk_version": ${PLATFORM_SDK_VERSION},
    "Platform_sdk_codename": "${PLATFORM_VERSION}",
    "Platform_version_active_codenames": ${PLATFORM_VERSION_ALL_CODENAMES},

    "DeviceName": "generic_arm64",
    "HostArch": "x86_64",
    "HostSecondaryArch": "x86",
    "CrossHost": "linux_bionic",
    "CrossHostArch": "x86_64",
    "Aml_abis": true,

    "UseGoma": ${USE_GOMA}
}
EOF

if [ -f ${SOONG_VARS} ] && cmp -s ${SOONG_VARS} ${SOONG_VARS}.new; then
  # Don't touch soong.variables if we don't have to, to avoid Soong rebuilding
  # the ninja file when it isn't necessary.
  rm ${SOONG_VARS}.new
else
  mv ${SOONG_VARS}.new ${SOONG_VARS}
fi

# We use force building LLVM components flag (even though we actually don't
# compile them) because we don't have bionic host prebuilts
# for them.
export FORCE_BUILD_LLVM_COMPONENTS=true

m --skip-make "$@"
