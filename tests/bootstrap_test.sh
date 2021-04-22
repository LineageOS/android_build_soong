#!/bin/bash -eu

set -o pipefail

# This test exercises the bootstrapping process of the build system
# in a source tree that only contains enough files for Bazel and Soong to work.

source "$(dirname "$0")/lib.sh"

function test_smoke {
  setup
  run_soong
}

function test_null_build() {
  setup
  run_soong
  local bootstrap_mtime1=$(stat -c "%y" out/soong/.bootstrap/build.ninja)
  local output_mtime1=$(stat -c "%y" out/soong/build.ninja)
  run_soong
  local bootstrap_mtime2=$(stat -c "%y" out/soong/.bootstrap/build.ninja)
  local output_mtime2=$(stat -c "%y" out/soong/build.ninja)

  if [[ "$bootstrap_mtime1" == "$bootstrap_mtime2" ]]; then
    # Bootstrapping is always done. It doesn't take a measurable amount of time.
    fail "Bootstrap Ninja file did not change on null build"
  fi

  if [[ "$output_mtime1" != "$output_mtime2" ]]; then
    fail "Output Ninja file changed on null build"
  fi
}

function test_soong_build_rebuilt_if_blueprint_changes() {
  setup
  run_soong
  local mtime1=$(stat -c "%y" out/soong/.bootstrap/build.ninja)

  sed -i 's/pluginGenSrcCmd/pluginGenSrcCmd2/g' build/blueprint/bootstrap/bootstrap.go

  run_soong
  local mtime2=$(stat -c "%y" out/soong/.bootstrap/build.ninja)

  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Bootstrap Ninja file did not change"
  fi
}

function test_change_android_bp() {
  setup
  mkdir -p a
  cat > a/Android.bp <<'EOF'
python_binary_host {
  name: "my_little_binary_host",
  srcs: ["my_little_binary_host.py"]
}
EOF
  touch a/my_little_binary_host.py
  run_soong

  grep -q "^# Module:.*my_little_binary_host" out/soong/build.ninja || fail "module not found"

  cat > a/Android.bp <<'EOF'
python_binary_host {
  name: "my_great_binary_host",
  srcs: ["my_great_binary_host.py"]
}
EOF
  touch a/my_great_binary_host.py
  run_soong

  grep -q "^# Module:.*my_little_binary_host" out/soong/build.ninja && fail "old module found"
  grep -q "^# Module:.*my_great_binary_host" out/soong/build.ninja || fail "new module not found"
}


function test_add_android_bp() {
  setup
  run_soong
  local mtime1=$(stat -c "%y" out/soong/build.ninja)

  mkdir -p a
  cat > a/Android.bp <<'EOF'
python_binary_host {
  name: "my_little_binary_host",
  srcs: ["my_little_binary_host.py"]
}
EOF
  touch a/my_little_binary_host.py
  run_soong

  local mtime2=$(stat -c "%y" out/soong/build.ninja)
  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Output Ninja file did not change"
  fi

  grep -q "^# Module:.*my_little_binary_host$" out/soong/build.ninja || fail "New module not in output"

  run_soong
}

function test_delete_android_bp() {
  setup
  mkdir -p a
  cat > a/Android.bp <<'EOF'
python_binary_host {
  name: "my_little_binary_host",
  srcs: ["my_little_binary_host.py"]
}
EOF
  touch a/my_little_binary_host.py
  run_soong

  grep -q "^# Module:.*my_little_binary_host$" out/soong/build.ninja || fail "Module not in output"

  rm a/Android.bp
  run_soong

  if grep -q "^# Module:.*my_little_binary_host$" out/soong/build.ninja; then
    fail "Old module in output"
  fi
}

# Test that an incremental build with a glob doesn't rerun soong_build, and
# only regenerates the globs on the first but not the second incremental build.
function test_glob_noop_incremental() {
  setup

  # This test needs to start from a clean build, but setup creates an
  # initialized tree that has already been built once.  Clear the out
  # directory to start from scratch (see b/185591972)
  rm -rf out

  mkdir -p a
  cat > a/Android.bp <<'EOF'
python_binary_host {
  name: "my_little_binary_host",
  srcs: ["*.py"],
}
EOF
  touch a/my_little_binary_host.py
  run_soong
  local ninja_mtime1=$(stat -c "%y" out/soong/build.ninja)

  local glob_deps_file=out/soong/.primary/globs/0.d

  if [ -e "$glob_deps_file" ]; then
    fail "Glob deps file unexpectedly written on first build"
  fi

  run_soong
  local ninja_mtime2=$(stat -c "%y" out/soong/build.ninja)

  # There is an ineffiencency in glob that requires bpglob to rerun once for each glob to update
  # the entry in the .ninja_log.  It doesn't update the output file, but we can detect the rerun
  # by checking if the deps file was created.
  if [ ! -e "$glob_deps_file" ]; then
    fail "Glob deps file missing after second build"
  fi

  local glob_deps_mtime2=$(stat -c "%y" "$glob_deps_file")

  if [[ "$ninja_mtime1" != "$ninja_mtime2" ]]; then
    fail "Ninja file rewritten on null incremental build"
  fi

  run_soong
  local ninja_mtime3=$(stat -c "%y" out/soong/build.ninja)
  local glob_deps_mtime3=$(stat -c "%y" "$glob_deps_file")

  if [[ "$ninja_mtime2" != "$ninja_mtime3" ]]; then
    fail "Ninja file rewritten on null incremental build"
  fi

  # The bpglob commands should not rerun after the first incremental build.
  if [[ "$glob_deps_mtime2" != "$glob_deps_mtime3" ]]; then
    fail "Glob deps file rewritten on second null incremental build"
  fi
}

function test_add_file_to_glob() {
  setup

  mkdir -p a
  cat > a/Android.bp <<'EOF'
python_binary_host {
  name: "my_little_binary_host",
  srcs: ["*.py"],
}
EOF
  touch a/my_little_binary_host.py
  run_soong
  local mtime1=$(stat -c "%y" out/soong/build.ninja)

  touch a/my_little_library.py
  run_soong

  local mtime2=$(stat -c "%y" out/soong/build.ninja)
  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Output Ninja file did not change"
  fi

  grep -q my_little_library.py out/soong/build.ninja || fail "new file is not in output"
}

function test_soong_build_rerun_iff_environment_changes() {
  setup

  mkdir -p cherry
  cat > cherry/Android.bp <<'EOF'
bootstrap_go_package {
  name: "cherry",
  pkgPath: "android/soong/cherry",
  deps: [
    "blueprint",
    "soong",
    "soong-android",
  ],
  srcs: [
    "cherry.go",
  ],
  pluginFor: ["soong_build"],
}
EOF

  cat > cherry/cherry.go <<'EOF'
package cherry

import (
  "android/soong/android"
  "github.com/google/blueprint"
)

var (
  pctx = android.NewPackageContext("cherry")
)

func init() {
  android.RegisterSingletonType("cherry", CherrySingleton)
}

func CherrySingleton() android.Singleton {
  return &cherrySingleton{}
}

type cherrySingleton struct{}

func (p *cherrySingleton) GenerateBuildActions(ctx android.SingletonContext) {
  cherryRule := ctx.Rule(pctx, "cherry",
    blueprint.RuleParams{
      Command: "echo CHERRY IS " + ctx.Config().Getenv("CHERRY") + " > ${out}",
      CommandDeps: []string{},
      Description: "Cherry",
    })

  outputFile := android.PathForOutput(ctx, "cherry", "cherry.txt")
  var deps android.Paths

  ctx.Build(pctx, android.BuildParams{
    Rule: cherryRule,
    Output: outputFile,
    Inputs: deps,
  })
}
EOF

  export CHERRY=TASTY
  run_soong
  grep -q "CHERRY IS TASTY" out/soong/build.ninja \
    || fail "first value of environment variable is not used"

  export CHERRY=RED
  run_soong
  grep -q "CHERRY IS RED" out/soong/build.ninja \
    || fail "second value of environment variable not used"
  local mtime1=$(stat -c "%y" out/soong/build.ninja)

  run_soong
  local mtime2=$(stat -c "%y" out/soong/build.ninja)
  if [[ "$mtime1" != "$mtime2" ]]; then
    fail "Output Ninja file changed when environment variable did not"
  fi

}

function test_add_file_to_soong_build() {
  setup
  run_soong
  local mtime1=$(stat -c "%y" out/soong/build.ninja)

  mkdir -p a
  cat > a/Android.bp <<'EOF'
bootstrap_go_package {
  name: "picard-soong-rules",
  pkgPath: "android/soong/picard",
  deps: [
    "blueprint",
    "soong",
    "soong-android",
  ],
  srcs: [
    "picard.go",
  ],
  pluginFor: ["soong_build"],
}
EOF

  cat > a/picard.go <<'EOF'
package picard

import (
  "android/soong/android"
  "github.com/google/blueprint"
)

var (
  pctx = android.NewPackageContext("picard")
)

func init() {
  android.RegisterSingletonType("picard", PicardSingleton)
}

func PicardSingleton() android.Singleton {
  return &picardSingleton{}
}

type picardSingleton struct{}

func (p *picardSingleton) GenerateBuildActions(ctx android.SingletonContext) {
  picardRule := ctx.Rule(pctx, "picard",
    blueprint.RuleParams{
      Command: "echo Make it so. > ${out}",
      CommandDeps: []string{},
      Description: "Something quotable",
    })

  outputFile := android.PathForOutput(ctx, "picard", "picard.txt")
  var deps android.Paths

  ctx.Build(pctx, android.BuildParams{
    Rule: picardRule,
    Output: outputFile,
    Inputs: deps,
  })
}

EOF

  run_soong
  local mtime2=$(stat -c "%y" out/soong/build.ninja)
  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Output Ninja file did not change"
  fi

  grep -q "Make it so" out/soong/build.ninja || fail "New action not present"
}

# Tests a glob in a build= statement in an Android.bp file, which is interpreted
# during bootstrapping.
function test_glob_during_bootstrapping() {
  setup

  mkdir -p a
  cat > a/Android.bp <<'EOF'
build=["foo*.bp"]
EOF
  cat > a/fooa.bp <<'EOF'
bootstrap_go_package {
  name: "picard-soong-rules",
  pkgPath: "android/soong/picard",
  deps: [
    "blueprint",
    "soong",
    "soong-android",
  ],
  srcs: [
    "picard.go",
  ],
  pluginFor: ["soong_build"],
}
EOF

  cat > a/picard.go <<'EOF'
package picard

import (
  "android/soong/android"
  "github.com/google/blueprint"
)

var (
  pctx = android.NewPackageContext("picard")
)

func init() {
  android.RegisterSingletonType("picard", PicardSingleton)
}

func PicardSingleton() android.Singleton {
  return &picardSingleton{}
}

type picardSingleton struct{}

var Message = "Make it so."

func (p *picardSingleton) GenerateBuildActions(ctx android.SingletonContext) {
  picardRule := ctx.Rule(pctx, "picard",
    blueprint.RuleParams{
      Command: "echo " + Message + " > ${out}",
      CommandDeps: []string{},
      Description: "Something quotable",
    })

  outputFile := android.PathForOutput(ctx, "picard", "picard.txt")
  var deps android.Paths

  ctx.Build(pctx, android.BuildParams{
    Rule: picardRule,
    Output: outputFile,
    Inputs: deps,
  })
}

EOF

  run_soong
  local mtime1=$(stat -c "%y" out/soong/build.ninja)

  grep -q "Make it so" out/soong/build.ninja || fail "Original action not present"

  cat > a/foob.bp <<'EOF'
bootstrap_go_package {
  name: "worf-soong-rules",
  pkgPath: "android/soong/worf",
  deps: [
    "blueprint",
    "soong",
    "soong-android",
    "picard-soong-rules",
  ],
  srcs: [
    "worf.go",
  ],
  pluginFor: ["soong_build"],
}
EOF

  cat > a/worf.go <<'EOF'
package worf

import "android/soong/picard"

func init() {
   picard.Message = "Engage."
}
EOF

  run_soong
  local mtime2=$(stat -c "%y" out/soong/build.ninja)
  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Output Ninja file did not change"
  fi

  grep -q "Engage" out/soong/build.ninja || fail "New action not present"

  if grep -q "Make it so" out/soong/build.ninja; then
    fail "Original action still present"
  fi
}

function test_null_build_after_docs {
  setup
  run_soong
  local mtime1=$(stat -c "%y" out/soong/build.ninja)

  prebuilts/build-tools/linux-x86/bin/ninja -f out/soong/build.ninja soong_docs
  run_soong
  local mtime2=$(stat -c "%y" out/soong/build.ninja)

  if [[ "$mtime1" != "$mtime2" ]]; then
    fail "Output Ninja file changed on null build"
  fi
}

function test_bp2build_smoke {
  setup
  GENERATE_BAZEL_FILES=1 run_soong
  [[ -e out/soong/.bootstrap/bp2build_workspace_marker ]] || fail "bp2build marker file not created"
  [[ -e out/soong/workspace ]] || fail "Bazel workspace not created"
}

function test_bp2build_add_android_bp {
  setup

  mkdir -p a
  touch a/a.txt
  cat > a/Android.bp <<'EOF'
filegroup {
  name: "a",
  srcs: ["a.txt"],
  bazel_module: { bp2build_available: true },
}
EOF

  GENERATE_BAZEL_FILES=1 run_soong
  [[ -e out/soong/bp2build/a/BUILD ]] || fail "a/BUILD not created"
  [[ -L out/soong/workspace/a/BUILD ]] || fail "a/BUILD not symlinked"

  mkdir -p b
  touch b/b.txt
  cat > b/Android.bp <<'EOF'
filegroup {
  name: "b",
  srcs: ["b.txt"],
  bazel_module: { bp2build_available: true },
}
EOF

  GENERATE_BAZEL_FILES=1 run_soong
  [[ -e out/soong/bp2build/b/BUILD ]] || fail "a/BUILD not created"
  [[ -L out/soong/workspace/b/BUILD ]] || fail "a/BUILD not symlinked"
}

function test_bp2build_null_build {
  setup

  GENERATE_BAZEL_FILES=1 run_soong
  local mtime1=$(stat -c "%y" out/soong/build.ninja)

  GENERATE_BAZEL_FILES=1 run_soong
  local mtime2=$(stat -c "%y" out/soong/build.ninja)

  if [[ "$mtime1" != "$mtime2" ]]; then
    fail "Output Ninja file changed on null build"
  fi
}

function test_bp2build_add_to_glob {
  setup

  mkdir -p a
  touch a/a1.txt
  cat > a/Android.bp <<'EOF'
filegroup {
  name: "a",
  srcs: ["*.txt"],
  bazel_module: { bp2build_available: true },
}
EOF

  GENERATE_BAZEL_FILES=1 run_soong
  grep -q a1.txt out/soong/bp2build/a/BUILD || fail "a1.txt not in BUILD file"

  touch a/a2.txt
  GENERATE_BAZEL_FILES=1 run_soong
  grep -q a2.txt out/soong/bp2build/a/BUILD || fail "a2.txt not in BUILD file"
}

function test_dump_json_module_graph() {
  setup
  SOONG_DUMP_JSON_MODULE_GRAPH="$MOCK_TOP/modules.json" run_soong
  if [[ ! -r "$MOCK_TOP/modules.json" ]]; then
    fail "JSON file was not created"
  fi
}

function test_bp2build_bazel_workspace_structure {
  setup

  mkdir -p a/b
  touch a/a.txt
  touch a/b/b.txt
  cat > a/b/Android.bp <<'EOF'
filegroup {
  name: "b",
  srcs: ["b.txt"],
  bazel_module: { bp2build_available: true },
}
EOF

  GENERATE_BAZEL_FILES=1 run_soong
  [[ -e out/soong/workspace ]] || fail "Bazel workspace not created"
  [[ -d out/soong/workspace/a/b ]] || fail "module directory not a directory"
  [[ -L out/soong/workspace/a/b/BUILD ]] || fail "BUILD file not symlinked"
  [[ "$(readlink -f out/soong/workspace/a/b/BUILD)" =~ bp2build/a/b/BUILD$ ]] \
    || fail "BUILD files symlinked at the wrong place"
  [[ -L out/soong/workspace/a/b/b.txt ]] || fail "a/b/b.txt not symlinked"
  [[ -L out/soong/workspace/a/a.txt ]] || fail "a/b/a.txt not symlinked"
  [[ ! -e out/soong/workspace/out ]] || fail "out directory symlinked"
}

function test_bp2build_bazel_workspace_add_file {
  setup

  mkdir -p a
  touch a/a.txt
  cat > a/Android.bp <<EOF
filegroup {
  name: "a",
  srcs: ["a.txt"],
  bazel_module: { bp2build_available: true },
}
EOF

  GENERATE_BAZEL_FILES=1 run_soong

  touch a/a2.txt  # No reference in the .bp file needed
  GENERATE_BAZEL_FILES=1 run_soong
  [[ -L out/soong/workspace/a/a2.txt ]] || fail "a/a2.txt not symlinked"
}

function test_bp2build_build_file_precedence {
  setup

  mkdir -p a
  touch a/a.txt
  touch a/BUILD
  cat > a/Android.bp <<EOF
filegroup {
  name: "a",
  srcs: ["a.txt"],
  bazel_module: { bp2build_available: true },
}
EOF

  GENERATE_BAZEL_FILES=1 run_soong
  [[ -L out/soong/workspace/a/BUILD ]] || fail "BUILD file not symlinked"
  [[ "$(readlink -f out/soong/workspace/a/BUILD)" =~ bp2build/a/BUILD$ ]] \
    || fail "BUILD files symlinked to the wrong place"
}

function test_bp2build_reports_multiple_errors {
  setup

  mkdir -p a/BUILD
  touch a/a.txt
  cat > a/Android.bp <<EOF
filegroup {
  name: "a",
  srcs: ["a.txt"],
  bazel_module: { bp2build_available: true },
}
EOF

  mkdir -p b/BUILD
  touch b/b.txt
  cat > b/Android.bp <<EOF
filegroup {
  name: "b",
  srcs: ["b.txt"],
  bazel_module: { bp2build_available: true },
}
EOF

  if GENERATE_BAZEL_FILES=1 run_soong >& "$MOCK_TOP/errors"; then
    fail "Build should have failed"
  fi

  grep -q "a/BUILD' exist" "$MOCK_TOP/errors" || fail "Error for a/BUILD not found"
  grep -q "b/BUILD' exist" "$MOCK_TOP/errors" || fail "Error for b/BUILD not found"
}

test_smoke
test_null_build
test_null_build_after_docs
test_soong_build_rebuilt_if_blueprint_changes
test_glob_noop_incremental
test_add_file_to_glob
test_add_android_bp
test_change_android_bp
test_delete_android_bp
test_add_file_to_soong_build
test_glob_during_bootstrapping
test_soong_build_rerun_iff_environment_changes
test_dump_json_module_graph
test_bp2build_smoke
test_bp2build_null_build
test_bp2build_add_android_bp
test_bp2build_add_to_glob
test_bp2build_bazel_workspace_structure
test_bp2build_bazel_workspace_add_file
test_bp2build_build_file_precedence
test_bp2build_reports_multiple_errors
