#!/bin/bash -e
#
# The script to run locally to re-generate global allowed list of dependencies
# for updatable modules.

if [ ! -e "build/envsetup.sh" ]; then
  echo "ERROR: $0 must be run from the top of the tree"
  exit 1
fi

source build/envsetup.sh > /dev/null || exit 1

readonly ALLOWED_DEPS_FILE="build/soong/apex/allowed_deps.txt"
readonly OUT_DIR=$(get_build_var OUT_DIR)
readonly FILTERED_UPDATABLE_FLATLISTS="${OUT_DIR}/soong/apex/depsinfo/filtered-updatable-flatlists.txt"

# If the script is run after droidcore failure, ${FILTERED_UPDATABLE_FLATLISTS}
# should already be built. If running the script manually, make sure it exists.
m "${FILTERED_UPDATABLE_FLATLISTS}"

cat > "${ALLOWED_DEPS_FILE}" << EndOfFileComment
# A list of allowed dependencies for all updatable modules.
#
# The list tracks all direct and transitive dependencies that end up within any of the updatable
# binaries; specifically excluding external dependencies required to compile those binaries. This
# prevents potential regressions in case a new dependency is not aware of the different functional
# and non-functional requirements being part of an updatable module (e.g. min_sdk_version).
#
# To update the list, run:
# repo-root$ build/soong/scripts/update-apex-allowed-deps.sh
#
# See go/apex-allowed-deps-error for more details.
# TODO(b/157465465): introduce automated quality signals and remove this list.

EndOfFileComment

cat "${FILTERED_UPDATABLE_FLATLISTS}" >> "${ALLOWED_DEPS_FILE}"
