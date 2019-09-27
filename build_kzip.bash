# /bin/bash -uv
#
# Build kzip files (source files for the indexing pipeline) for the given configuration,
# merge them and place the resulting all.kzip into $DIST_DIR.
# It is assumed that the current directory is the top of the source tree.
# The following environment variables affect the result:
#   TARGET_PRODUCT        target device name, e.g., 'aosp_blueline'
#   TARGET_BUILD_VARIANT  variant, e.g., `userdebug`
#   OUT_DIR               absolute path  where the build is happening ($PWD/out if not specified})
#   DIST_DIR              where the resulting all.kzip will be placed
#   XREF_CORPUS           source code repository URI, e.g., 'android.googlesource.com/platform/superproject'
#   BUILD_NUMBER          build number, used to generate unique ID (will use UUID if not set)

# If OUT_DIR is not set, the build will use out/ as output directory, which is
# a relative path. Make it absolute, otherwise the indexer will not know that it
# contains only generated files.
: ${OUT_DIR:=$PWD/out}
[[ "$OUT_DIR" =~ ^/ ]] || { echo "$OUT_DIR is not an absolute path"; exit 1; }
: ${BUILD_NUMBER:=$(uuidgen)}

# The extraction might fail for some source files, so run with -k
OUT_DIR=$OUT_DIR build/soong/soong_ui.bash --build-mode --all-modules --dir=$PWD -k merge_zips xref_cxx xref_java

# We build with -k, so check that we have generated at least 100K files
# (the actual number is 180K+)
declare -r kzip_count=$(find $OUT_DIR -name '*.kzip' | wc -l)
(($kzip_count>100000)) || { printf "Too few kzip files were generated: %d\n" $kzip_count; exit 1; }

# Pack
# TODO(asmundak): this should be done by soong.
declare -r allkzip="$BUILD_NUMBER.kzip"
"$OUT_DIR/soong/host/linux-x86/bin/merge_zips" "$DIST_DIR/$allkzip" @<(find $OUT_DIR -name '*.kzip')

