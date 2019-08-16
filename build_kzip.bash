# /bin/bash -uv
#
# Build kzip files (source files for the indexing pipeline) for the given configuration,
# merge them and place the resulting all.kzip into $DIST_DIR.
# It is assumed that the current directory is the top of the source tree.
# The following enviromnet variables affect the result:
#   TARGET_PRODUCT        target device name, e.g., `aosp_blueline`
#   TARGET_BUILD_VARIANT  variant, e.g., `userdebug`
#   OUT_DIR               where the build is happening (./out if not specified)
#   DIST_DIR              where the resulting all.kzip will be placed
#   XREF_CORPUS           source code repository URI, e.g.,
#                        `android.googlesource.com/platform/superproject`

# The extraction might fail for some source files, so run with -k
build/soong/soong_ui.bash --build-mode --all-modules --dir=$PWD -k merge_zips xref_cxx xref_java

# We build with -k, so check that we have generated at least 100K files
# (the actual number is 180K+)
declare -r kzip_count=$(find $OUT_DIR -name '*.kzip' | wc -l)
(($kzip_count>100000)) || { printf "Too few kzip files were generated: %d\n" $kzip_count; exit 1; }

# Pack
# TODO(asmundak): this should be done by soong.
declare -r allkzip=all.kzip
"${OUT_DIR:-out}/soong/host/linux-x86/bin/merge_zips" "$DIST_DIR/$allkzip" @<(find $OUT_DIR -name '*.kzip')
