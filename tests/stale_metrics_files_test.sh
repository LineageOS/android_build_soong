#!/bin/bash -e

# This test ensures that stale metrics files are deleted after each run

# Run bazel
# Note - bp2build metrics are present after clean runs, only
build/soong/soong_ui.bash --make-mode clean
build/bazel/bin/b build libcore:all
soong_build_metrics_files=("out/soong_build_metrics.pb" "out/build_progress.pb" "out/soong_metrics" "out/bp2build_metrics.pb")
bazel_build_metrics_files=("out/bazel_metrics.pb" "out/build_progress.pb" "out/soong_metrics" "out/bp2build_metrics.pb")

# Ensure bazel metrics files are present
for i in ${!bazel_build_metrics_files[@]};
do
  file=${bazel_build_metrics_files[$i]}
  if [[ ! -f $file ]]; then
     echo "Missing metrics file for Bazel build " $file
     exit 1
  fi
done


# Run a soong build
build/soong/soong_ui.bash --make-mode nothing

for i in ${!soong_build_metrics_files[@]};
do
  file=${soong_build_metrics_files[$i]}
  if [[ ! -f $file ]]; then
     echo "Missing metrics file for Soong build " $file
     exit 1
  fi
done

# Ensure that bazel_metrics.pb is deleted
if [[ -f out/bazel_metrics.pb ]]; then
   echo "Stale out/bazel_metrics.pb file detected"
   exit 1
fi

# Run bazel again - to make sure that soong_build_metrics.pb gets deleted
build/bazel/bin/b build libcore:all

if [[ -f out/soong_build_metrics.pb ]]; then
   echo "Stale out/soong_build_metrics.pb file detected"
   exit 1
fi
