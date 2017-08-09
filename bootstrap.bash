#!/bin/bash

set -e

if [ -z "$NO_DEPRECATION_WARNING" ]; then
    echo '== WARNING: bootstrap.bash & ./soong are deprecated ==' >&2
    echo 'Use `m --skip-make` with a standalone OUT_DIR instead.' >&2
    echo 'Without envsetup.sh, use:' >&2
    echo '  build/soong/soong_ui.bash --make-mode --skip-make' >&2
    echo '======================================================' >&2
fi

ORIG_SRCDIR=$(dirname "${BASH_SOURCE[0]}")
if [[ "$ORIG_SRCDIR" != "." ]]; then
  if [[ ! -z "$BUILDDIR" ]]; then
    echo "error: To use BUILDDIR, run from the source directory"
    exit 1
  fi
  export BUILDDIR=$("${ORIG_SRCDIR}/build/soong/scripts/reverse_path.py" "$ORIG_SRCDIR")
  cd $ORIG_SRCDIR
fi
if [[ -z "$BUILDDIR" ]]; then
  echo "error: Run ${BASH_SOURCE[0]} from the build output directory"
  exit 1
fi
export SRCDIR="."
export BOOTSTRAP="${SRCDIR}/bootstrap.bash"
export BLUEPRINTDIR="${SRCDIR}/build/blueprint"

export TOPNAME="Android.bp"
export RUN_TESTS="-t"

case $(uname) in
    Linux)
	export PREBUILTOS="linux-x86"
	;;
    Darwin)
	export PREBUILTOS="darwin-x86"
	;;
    *) echo "unknown OS:" $(uname) && exit 1;;
esac
export GOROOT="${SRCDIR}/prebuilts/go/$PREBUILTOS"

if [[ $# -eq 0 ]]; then
    mkdir -p $BUILDDIR

    if [[ $(find $BUILDDIR -maxdepth 1 -name Android.bp) ]]; then
      echo "FAILED: The build directory must not be a source directory"
      exit 1
    fi

    export SRCDIR_FROM_BUILDDIR=$(build/soong/scripts/reverse_path.py "$BUILDDIR")

    sed -e "s|@@BuildDir@@|${BUILDDIR}|" \
        -e "s|@@SrcDirFromBuildDir@@|${SRCDIR_FROM_BUILDDIR}|" \
        -e "s|@@PrebuiltOS@@|${PREBUILTOS}|" \
        "$SRCDIR/build/soong/soong.bootstrap.in" > $BUILDDIR/.soong.bootstrap
    ln -sf "${SRCDIR_FROM_BUILDDIR}/build/soong/soong.bash" $BUILDDIR/soong
fi

"$SRCDIR/build/blueprint/bootstrap.bash" "$@"
