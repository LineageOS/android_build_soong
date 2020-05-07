#!/bin/bash
# Generates the golang source file of protos file describing APK set table of
# contents (toc.pb file).

set -e
function die() { echo "ERROR: $1" >&2; exit 1; }

readonly error_msg="Maybe you need to run 'lunch aosp_arm-eng && m aprotoc blueprint_tools'?"

hash aprotoc &>/dev/null || die "could not find aprotoc. ${error_msg}"
# TODO(asmundak): maybe have the paths relative to repo top?
(cd "${0%/*}" && aprotoc --go_out=paths=source_relative:. commands.proto config.proto targeting.proto ) || die "build failed. ${error_msg}"
