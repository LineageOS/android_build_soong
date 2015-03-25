#!/bin/bash

# Determine the build directory location based on the location of this script.
BPBUILD="${BASH_SOURCE[0]}"
BUILDDIR=`dirname "${BASH_SOURCE[0]}"`
BOOTSTRAP=${BUILDDIR}/.soong.bootstrap

# The source directory path and operating system will get written to
# .soong.bootstrap by the bootstrap script.

if [ ! -f ${BUILDDIR}/${BOOTSTRAP} ]; then
    echo "Error: soong script must be located in a directory created by bootstrap.bash"
    exit 1
fi

source ${BUILDDIR}/.soong.bootstrap

if [[ ${SRCDIR_IN:0:1} == '/' ]]; then
    # SRCDIR_IN is an absolute path
    SRCDIR=${SRCDIR_IN}
else
    # SRCDIR_IN is a relative path
    SRCDIR=${BUILDDIR}/${SRCDIR_IN}
fi

# Let Blueprint know that the Ninja we're using performs multiple passes that
# can regenerate the build manifest.
export BLUEPRINT_NINJA_HAS_MULTIPASS=1

# Ninja can't depend on environment variables, so do a manual comparison
# of the relevant environment variables from the last build using the
# soong_env tool and trigger a build manifest regeneration if necessary
ENVFILE=${BUILDDIR}/.soong.environment
ENVTOOL=${BUILDDIR}/.bootstrap/bin/soong_env
if [ -f ${ENVFILE} ]; then
    if [ -x ${ENVTOOL} ]; then
        if ! ${ENVTOOL} ${ENVFILE}; then
            echo "forcing build manifest regeneration"
            rm -f ${ENVFILE}
        fi
    else
        echo "Missing soong_env tool, forcing build manifest regeneration"
        rm -f ${ENVFILE}
    fi
fi

${SRCDIR}/prebuilts/ninja/${PREBUILTOS}/ninja -C ${BUILDDIR} "$@"
