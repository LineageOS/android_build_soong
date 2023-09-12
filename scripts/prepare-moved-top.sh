#!/bin/bash -eu

###############
# Removes the Bazel output base and ninja file.
# This is intended to solve an issue when a build top is moved.
# Starlark symlinks are absolute and a moved build top will have many
# dangling symlinks and fail to function as intended.
# If the bazel output base is removed WITHOUT the top moving,
# then any subsequent builds will fail as soong_build will not rerun.
# Removing the ninja file will force a re-execution.
#
# You MUST lunch again after moving your build top, before running this.
###############

if [[ ! -v ANDROID_BUILD_TOP ]]; then
    echo "ANDROID_BUILD_TOP not found in environment. Please run lunch before running this script"
    exit 1
fi

if [[ ! -v OUT_DIR ]]; then
    out_dir="$ANDROID_BUILD_TOP/out"
else
    out_dir="$ANDROID_BUILD_TOP/$OUT_DIR"
fi

output_base=$out_dir/bazel/output/
ninja_file=$out_dir/soong/build*ninja

if [[ ! -d $output_base ]]; then
    echo "The specified output directory doesn't exist."
    echo "Have you rerun lunch since moving directories?"
    exit 1
fi

read -p "Are you sure you want to remove $output_base and the ninja file $ninja_file? Y/N " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]
then
   rm -rf $output_base
   rm $ninja_file
fi
