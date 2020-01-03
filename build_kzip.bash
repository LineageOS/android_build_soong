#! /bin/bash -uv
#
# Build kzip files (source files for the indexing pipeline) for the given configuration,
# merge them and place the resulting all.kzip into $DIST_DIR.
# It is assumed that the current directory is the top of the source tree.
# The following environment variables affect the result:
#   BUILD_NUMBER          build number, used to generate unique ID (will use UUID if not set)
#   DIST_DIR              where the resulting all.kzip will be placed
#   OUT_DIR               output directory (out if not specified})
#   TARGET_BUILD_VARIANT  variant, e.g., `userdebug`
#   TARGET_PRODUCT        target device name, e.g., 'aosp_blueline'
#   XREF_CORPUS           source code repository URI, e.g., 'android.googlesource.com/platform/superproject'

: ${BUILD_NUMBER:=$(uuidgen)}

# The extraction might fail for some source files, so run with -k and then check that
# sufficiently many files were generated.
declare -r out="${OUT_DIR:-out}"
# Build extraction files for C++ and Java. Build `merge_zips` which we use later.
build/soong/soong_ui.bash --build-mode --all-modules --dir=$PWD -k merge_zips xref_cxx xref_java
#Build extraction file for Go files in build/soong directory.
declare -r abspath_out=$(realpath "${out}")
(cd build/soong;
 ../../prebuilts/build-tools/linux-x86/bin/go_extractor \
    --goroot="${PWD}/../../prebuilts/go/linux-x86" \
    --rules=vnames.go.json \
    --canonicalize_package_corpus \
    --output "${abspath_out}/soong/all.go.kzip" \
    ./... )

declare -r kzip_count=$(find "$out" -name '*.kzip' | wc -l)
(($kzip_count>100000)) || { printf "Too few kzip files were generated: %d\n" $kzip_count; exit 1; }

# Pack
# TODO(asmundak): this should be done by soong.
declare -r allkzip="$BUILD_NUMBER.kzip"
"$out/soong/host/linux-x86/bin/merge_zips" "$DIST_DIR/$allkzip" @<(find "$out" -name '*.kzip')

