#!/bin/bash -ex

: "${OUT_DIR:?Must set OUT_DIR}"
TOP=$(cd $(dirname $0)/../../..; pwd)
cd ${TOP}

UNAME="$(uname)"
case "$UNAME" in
Linux)
    OS='linux'
    ;;
Darwin)
    OS='darwin'
    ;;
*)
    exit 1
    ;;
esac

# Verify that go test and go build work on all the same projects that are parsed by
# build/soong/build_kzip.bash
declare -ar go_modules=(build/blueprint build/soong
      build/make/tools/canoninja build/make/tools/compliance build/make/tools/rbcrun)
export GOROOT=${TOP}/prebuilts/go/${OS}-x86
export GOENV=off
export GOPROXY=off
abs_out_dir=$(cd ${OUT_DIR}; pwd)
export GOPATH=${abs_out_dir}/gopath
export GOCACHE=${abs_out_dir}/gocache
export GOMODCACHE=${abs_out_dir}/gomodcache
export TMPDIR=${abs_out_dir}/gotemp
mkdir -p ${TMPDIR}
${GOROOT}/bin/go env

if [[ ${OS} = linux ]]; then
    # Building with the race detector enabled uses the host linker, set the
    # path to use the hermetic one.
    CLANG_VERSION=$(build/soong/scripts/get_clang_version.py)
    export CC="${TOP}/prebuilts/clang/host/${OS}-x86/${CLANG_VERSION}/bin/clang"
    export CXX="${TOP}/prebuilts/clang/host/${OS}-x86/${CLANG_VERSION}/bin/clang++"
fi

# androidmk_test.go gets confused if ANDROID_BUILD_TOP is set.
unset ANDROID_BUILD_TOP

network_jail=""
if [[ ${OS} = linux ]]; then
    # The go tools often try to fetch dependencies from the network,
    # wrap them in an nsjail to prevent network access.
    network_jail=${TOP}/prebuilts/build-tools/linux-x86/bin/nsjail
    # Quiet
    network_jail="${network_jail} -q"
    # No timeout
    network_jail="${network_jail} -t 0"
    # Set working directory
    network_jail="${network_jail} --cwd=\$PWD"
    # Pass environment variables through
    network_jail="${network_jail} -e"
    # Allow read-only access to everything
    network_jail="${network_jail} -R /"
    # Allow write access to the out directory
    network_jail="${network_jail} -B ${abs_out_dir}"
    # Allow write access to the /tmp directory
    network_jail="${network_jail} -B /tmp"
    # Set high values, as network_jail uses low defaults.
    network_jail="${network_jail} --rlimit_as soft"
    network_jail="${network_jail} --rlimit_core soft"
    network_jail="${network_jail} --rlimit_cpu soft"
    network_jail="${network_jail} --rlimit_fsize soft"
    network_jail="${network_jail} --rlimit_nofile soft"
fi

for dir in "${go_modules[@]}"; do
    (cd "$dir";
     eval ${network_jail} -- ${GOROOT}/bin/go build ./...
     eval ${network_jail} -- ${GOROOT}/bin/go test ./...
     eval ${network_jail} -- ${GOROOT}/bin/go test -race -short ./...
    )
done
