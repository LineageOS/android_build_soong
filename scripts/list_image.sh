#! /bin/bash

# Recursively list Android image directory.
set -eu
set -o pipefail

function die() { format=$1; shift; printf "$format\n" "$@"; exit 1; }

# Figure out the filer utility.
declare filer=
[[ -z "${ANDROID_HOST_OUT:-}" ]] || filer=${ANDROID_HOST_OUT}/bin/debugfs_static
if [[ "${1:-}" =~ --debugfs_path=(.*) ]]; then
  filer=${BASH_REMATCH[1]}
  shift
fi
if [[ -z "${filer:-}" ]]; then
  maybefiler="$(dirname $0)/debugfs_static"
  [[ ! -x "$maybefiler" ]] || filer="$maybefiler"
fi

(( $# >0 )) || die "%s [--debugfs_path=<path>] IMAGE" "$0"

[[ -n "${filer:-}" ]] || die "cannot locate 'debugfs' executable: \
--debugfs_path= is missing, ANDROID_HOST_OUT is not set, \
and 'debugfs_static' is not colocated with this script"
declare -r image="$1"

function dolevel() {
  printf "%s/\n" "$1"
  # Each line of the file output consists of 6 fields separated with '/'.
  # The second one contains the file's attributes, and the fifth its name.
  $filer -R "ls -l -p $1" "$image" 2>/dev/null |\
    sed -nr 's|^/.*/(.*)/.*/.*/(.+)/.*/$|\2 \1|p' | LANG=C sort | \
  while read name attr; do
    [[ "$name" != '.' && "$name" != '..' ]] || continue
    path="$1/$name"
    # If the second char of the attributes is '4', it is a directory.
    if [[ $attr =~ ^.4 ]]; then
      dolevel "$path"
    else
      printf "%s\n" "$path"
    fi
  done
}

# The filer always prints its version on stderr, so we are going
# to redirect it to the bit bucket. On the other hand, the filer's
# return code on error is still 0. Let's run it once to without
# redirecting stderr to see that there is at least one entry.
$filer -R "ls -l -p" "$image" | grep -q -m1 -P '^/.*/.*/.*/.*/.+/.*/$'
dolevel .
