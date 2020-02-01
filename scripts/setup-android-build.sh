#! /bin/bash
#
# Sets the current directory as Android build output directory for a
# given target by writing the "prefix script" to it. Commands prefixed
# by this script are executed in the Android build environment. E.g.,
# running
#   ./run <command>
# runs <command> as if we issued
#   cd <source>
#   mount --bind <build dir> out
#   . build/envsetup.sh
#   lunch <config>
#   <command>
#   exit
#
# This arrangement eliminates the need to issue envsetup/lunch commands
# manually, and allows to run multiple builds from the same shell.
# Thus, if your source tree is in ~/aosp and you are building for
# 'blueline' and 'cuttlefish', issuing
#   cd /sdx/blueline && \
#      ~/aosp/build/soong/scripts/setup-android-build.sh aosp_blueline-userdebug
#   cd /sdx/cuttlefish && \
#      ~/aosp/build/soong/scripts/setup-android-build.sh aosp_cf_arm64_phone-userdebug
# sets up build directories in /sdx/blueline and /sdx/cuttlefish respectively.
# After that, issue
#   /sdx/blueline/run m
# to build blueline image, and issue
#   /sdx/cuttlefish atest CtsSecurityBulletinHostTestCases
# to run CTS tests. Notice there is no need to change to a specific directory for that.
#
# Argument:
# * configuration (one of those shown by `lunch` command).
#
set -e
function die() { printf "$@"; exit 1; }

# Find out where the source tree using the fact that we are in its
# build/ subdirectory.
[[ "$(uname)" == Linux ]] || die "This setup runs only on Linux\n"
declare -r mydir="${0%/*}"
declare -r source="${mydir%/build/soong/scripts}"
[[ "/${mydir}/" =~ '/build/soong/scripts/' ]] || \
  die "$0 should be in build/soong/scripts/ subdirectory of the source tree\n"
[[ ! -e .repo && ! -e .git ]] || \
  die "Current directory looks like source. You should be in the _target_ directory.\n"
# Do not override old run script.
if [[ -x ./run ]]; then
  # Set variables from config=xxx and source=xxx comments in the existing script.
  . <(sed -nr 's/^# *source=(.*)/oldsource=\1/p;s/^# *config=(.*)/oldconfig=\1/p' run)
  die "This directory has been already set up to build Android for %s from %s.\n\
Remove 'run' file if you want to set it up afresh\n" "$oldconfig" "$oldsource"
fi

(($#<2)) || die "usage: %s [<config>]\n" $0

if (($#==1)); then
  # Configuration is provided, emit run script.
  declare -r config="$1"
  declare -r target="$PWD"
  cat >./run <<EOF
#! /bin/bash
# source=$source
# config=$config
declare -r cmd=\$(printf ' %q' "\$@")
"$source/prebuilts/build-tools/linux-x86/bin/nsjail"\
 -Mo -q -e -t 0\
 -EANDROID_QUIET_BUILD=true \
 -B / -B "$target:$source/out"\
 --cwd "$source"\
 --skip_setsid \
 --keep_caps\
 --disable_clone_newcgroup\
 --disable_clone_newnet\
 --rlimit_as soft\
 --rlimit_core soft\
 --rlimit_cpu soft\
 --rlimit_fsize soft\
 --rlimit_nofile soft\
 --proc_rw\
 --hostname $(hostname) \
 --\
 /bin/bash -i -c ". build/envsetup.sh && lunch "$config" &&\$cmd"
EOF
  chmod +x ./run
else
  # No configuration, show available ones.
  printf "Please specify build target. Common values:\n"
  (cd "$source"
   . build/envsetup.sh
   get_build_var COMMON_LUNCH_CHOICES | tr ' ' '\n' | pr -c4 -tT -W"$(tput cols)"
  )
  exit 1
fi
