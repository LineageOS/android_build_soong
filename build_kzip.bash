#! /bin/bash -uv
#
# Build kzip files (source files for the indexing pipeline) for the given configuration,
# merge them and place the resulting all.kzip into $DIST_DIR.
# It is assumed that the current directory is the top of the source tree.
# The following environment variables affect the result:
#   BUILD_NUMBER          build number, used to generate unique ID (will use UUID if not set)
#   SUPERPROJECT_SHA      superproject sha, used to generate unique id (will use BUILD_NUMBER if not set)
#   SUPERPROJECT_REVISION superproject revision, used for unique id if defined as a sha
#   KZIP_NAME             name of the output file (will use SUPERPROJECT_REVISION|SUPERPROJECT_SHA|BUILD_NUMBER|UUID if not set)
#   DIST_DIR              where the resulting all.kzip will be placed
#   KYTHE_KZIP_ENCODING   proto or json (proto is default)
#   KYTHE_JAVA_SOURCE_BATCH_SIZE maximum number of the Java source files in a compilation unit
#   OUT_DIR               output directory (out if not specified})
#   TARGET_BUILD_VARIANT  variant, e.g., `userdebug`
#   TARGET_PRODUCT        target device name, e.g., 'aosp_blueline'
#   XREF_CORPUS           source code repository URI, e.g., 'android.googlesource.com/platform/superproject'

# If the SUPERPROJECT_REVISION is defined as a sha, use this as the default value if no
# SUPERPROJECT_SHA is specified.
if [[ ${SUPERPROJECT_REVISION:-} =~ [0-9a-f]{40} ]]; then
  : ${KZIP_NAME:=${SUPERPROJECT_REVISION:-}}
fi

: ${KZIP_NAME:=${SUPERPROJECT_SHA:-}}
: ${KZIP_NAME:=${BUILD_NUMBER:-}}
: ${KZIP_NAME:=$(uuidgen)}

: ${KYTHE_JAVA_SOURCE_BATCH_SIZE:=500}
: ${KYTHE_KZIP_ENCODING:=proto}
: ${XREF_CORPUS:?should be set}
export KYTHE_JAVA_SOURCE_BATCH_SIZE KYTHE_KZIP_ENCODING

# The extraction might fail for some source files, so run with -k and then check that
# sufficiently many files were generated.
declare -r out="${OUT_DIR:-out}"

# Build extraction files and `merge_zips` which we use later.
kzip_targets=(
  merge_zips
  xref_cxx
  xref_java
  # TODO: b/286390153 - reenable rust
  # xref_rust
)

build/soong/soong_ui.bash --build-mode --all-modules --dir=$PWD -k --skip-soong-tests --ninja_weight_source=not_used "${kzip_targets[@]}"

# Build extraction file for Go the files in build/{blueprint,soong} directories.
declare -r abspath_out=$(realpath "${out}")
declare -r go_extractor=$(realpath prebuilts/build-tools/linux-x86/bin/go_extractor)
declare -r go_root=$(realpath prebuilts/go/linux-x86)
declare -r source_root=$PWD

# For the Go code, we invoke the extractor directly. The two caveats are that
# the extractor's rewrite rules are generated on the fly as they depend on the
# value of XREF_CORPUS, and that the name of the kzip file is derived from the
# directory name by replacing '/' with '_'.
# Go extractor should succeed.
declare -ar go_modules=(build/blueprint build/soong
  build/make/tools/canoninja build/make/tools/compliance build/make/tools/rbcrun)
set -e
for dir in "${go_modules[@]}"; do
  (cd "$dir";
   outfile=$(echo "$dir" | sed -r 's|/|_|g;s|(.*)|\1.go.kzip|');
   KYTHE_ROOT_DIRECTORY="${source_root}" "$go_extractor" --goroot="$go_root" \
   --rules=<(printf '[{"pattern": "(.*)","vname": {"path": "@1@", "corpus":"%s"}}]' "${XREF_CORPUS}") \
   --canonicalize_package_corpus --output "${abspath_out}/soong/$outfile" ./...
  )
done
set +e

declare -r kzip_count=$(find "$out" -name '*.kzip' | wc -l)
(($kzip_count>100000)) || { >&2 printf "ERROR: Too few kzip files were generated: %d\n" $kzip_count; exit 1; }

# Pack
declare -r allkzip="$KZIP_NAME.kzip"
"$out/host/linux-x86/bin/merge_zips" "$DIST_DIR/$allkzip" @<(find "$out" -name '*.kzip')

