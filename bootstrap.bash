#!/bin/bash

export BOOTSTRAP="${BASH_SOURCE[0]}"
export SRCDIR=$(dirname "${BASH_SOURCE[0]}")
export TOPNAME="Android.bp"
export BOOTSTRAP_MANIFEST="${SRCDIR}/build/soong/build.ninja.in"
export RUN_TESTS="-t"

case $(uname) in
    Linux)
	export GOOS="linux"
	export PREBUILTOS="linux-x86"
	;;
    Darwin)
	export GOOS="darwin"
	export PREBUILTOS="darwin-x86"
	;;
    *) echo "unknown OS:" $(uname) && exit 1;;
esac
export GOROOT="${SRCDIR}/prebuilts/go/$PREBUILTOS/"
export GOARCH="amd64"
export GOCHAR="6"

if [[ $(find . -maxdepth 1 -name $(basename "${BOOTSTRAP}")) ]]; then
  echo "FAILED: Tried to run "$(basename "${BOOTSTRAP}")" from "$(pwd)""
  exit 1
fi

if [[ $# -eq 0 ]]; then
    sed -e "s|@@SrcDir@@|${SRCDIR}|" \
        -e "s|@@PrebuiltOS@@|${PREBUILTOS}|" \
        "${SRCDIR}/build/soong/soong.bootstrap.in" > .soong.bootstrap
    ln -sf "${SRCDIR}/build/soong/soong.bash" soong
fi

"${SRCDIR}/build/blueprint/bootstrap.bash" "$@"
