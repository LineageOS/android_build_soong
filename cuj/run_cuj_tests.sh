#!/bin/bash -e

readonly UNAME="$(uname)"
case "$UNAME" in
Linux)
    readonly OS='linux'
    ;;
Darwin)
    readonly OS='darwin'
    ;;
*)
    echo "Unsupported OS '$UNAME'"
    exit 1
    ;;
esac

readonly ANDROID_TOP="$(cd $(dirname $0)/../../..; pwd)"
cd "$ANDROID_TOP"

export OUT_DIR="${OUT_DIR:-out}"
readonly SOONG_OUT="${OUT_DIR}/soong"

build/soong/soong_ui.bash --make-mode "${SOONG_OUT}/host/${OS}-x86/bin/cuj_tests"

"${SOONG_OUT}/host/${OS}-x86/bin/cuj_tests" || true

if [ -n "${DIST_DIR}" ]; then
  cp -r "${OUT_DIR}/cuj_tests/logs" "${DIST_DIR}"
fi
