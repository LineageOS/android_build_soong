#!/bin/bash -eu

set -o pipefail

# This test exercises the bootstrapping process of the build system
# in a source tree that only contains enough files for Bazel and Soong to work.

source "$(dirname "$0")/lib.sh"

readonly GENERATED_BUILD_FILE_NAME="BUILD.bazel"

readonly target_product="${TARGET_PRODUCT:-aosp_arm}"

function test_smoke {
  setup
  run_soong
}

function test_null_build() {
  setup
  run_soong
  local -r bootstrap_mtime1=$(stat -c "%y" out/soong/bootstrap.ninja)
  local -r output_mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  run_soong
  local -r bootstrap_mtime2=$(stat -c "%y" out/soong/bootstrap.ninja)
  local -r output_mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)

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
  local -r mtime1=$(stat -c "%y" out/soong/bootstrap.ninja)

  sed -i 's/pluginGenSrcCmd/pluginGenSrcCmd2/g' build/blueprint/bootstrap/bootstrap.go

  run_soong
  local -r mtime2=$(stat -c "%y" out/soong/bootstrap.ninja)

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

  grep -q "^# Module:.*my_little_binary_host" out/soong/build."${target_product}".ninja || fail "module not found"

  cat > a/Android.bp <<'EOF'
python_binary_host {
  name: "my_great_binary_host",
  srcs: ["my_great_binary_host.py"]
}
EOF
  touch a/my_great_binary_host.py
  run_soong

  grep -q "^# Module:.*my_little_binary_host" out/soong/build."${target_product}".ninja && fail "old module found"
  grep -q "^# Module:.*my_great_binary_host" out/soong/build."${target_product}".ninja || fail "new module not found"
}

function test_add_android_bp() {
  setup
  run_soong
  local -r mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  mkdir -p a
  cat > a/Android.bp <<'EOF'
python_binary_host {
  name: "my_little_binary_host",
  srcs: ["my_little_binary_host.py"]
}
EOF
  touch a/my_little_binary_host.py
  run_soong

  local -r mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Output Ninja file did not change"
  fi

  grep -q "^# Module:.*my_little_binary_host$" out/soong/build."${target_product}".ninja || fail "New module not in output"

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

  grep -q "^# Module:.*my_little_binary_host$" out/soong/build."${target_product}".ninja || fail "Module not in output"

  rm a/Android.bp
  run_soong

  if grep -q "^# Module:.*my_little_binary_host$" out/soong/build."${target_product}".ninja; then
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
  local -r ninja_mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  local glob_deps_file=out/soong/globs/"${target_product}"/0.d

  run_soong
  local -r ninja_mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  # There is an ineffiencency in glob that requires bpglob to rerun once for each glob to update
  # the entry in the .ninja_log.  It doesn't update the output file, but we can detect the rerun
  # by checking if the deps file was created.
  if [ ! -e "$glob_deps_file" ]; then
    fail "Glob deps file missing after second build"
  fi

  local -r glob_deps_mtime2=$(stat -c "%y" "$glob_deps_file")

  if [[ "$ninja_mtime1" != "$ninja_mtime2" ]]; then
    fail "Ninja file rewritten on null incremental build"
  fi

  run_soong
  local -r ninja_mtime3=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  local -r glob_deps_mtime3=$(stat -c "%y" "$glob_deps_file")

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
  local -r mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  touch a/my_little_library.py
  run_soong

  local -r mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Output Ninja file did not change"
  fi

  grep -q my_little_library.py out/soong/build."${target_product}".ninja || fail "new file is not in output"
}

function test_soong_build_rerun_iff_environment_changes() {
  setup

  mkdir -p build/soong/cherry
  cat > build/soong/cherry/Android.bp <<'EOF'
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

  cat > build/soong/cherry/cherry.go <<'EOF'
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
  grep -q "CHERRY IS TASTY" out/soong/build."${target_product}".ninja \
    || fail "first value of environment variable is not used"

  export CHERRY=RED
  run_soong
  grep -q "CHERRY IS RED" out/soong/build."${target_product}".ninja \
    || fail "second value of environment variable not used"
  local -r mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  run_soong
  local -r mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  if [[ "$mtime1" != "$mtime2" ]]; then
    fail "Output Ninja file changed when environment variable did not"
  fi

}

function test_create_global_include_directory() {
  setup
  run_soong
  local -r mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  # Soong needs to know if top level directories like hardware/ exist for use
  # as global include directories.  Make sure that doesn't cause regens for
  # unrelated changes to the top level directory.
  mkdir -p system/core

  run_soong
  local -r mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  if [[ "$mtime1" != "$mtime2" ]]; then
    fail "Output Ninja file changed when top level directory changed"
  fi

  # Make sure it does regen if a missing directory in the path of a global
  # include directory is added.
  mkdir -p system/core/include

  run_soong
  local -r mtime3=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  if [[ "$mtime2" = "$mtime3" ]]; then
    fail "Output Ninja file did not change when global include directory created"
  fi

}

function test_add_file_to_soong_build() {
  setup
  run_soong
  local -r mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  mkdir -p vendor/foo/picard
  cat > vendor/foo/picard/Android.bp <<'EOF'
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

  cat > vendor/foo/picard/picard.go <<'EOF'
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
  local -r mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Output Ninja file did not change"
  fi

  grep -q "Make it so" out/soong/build."${target_product}".ninja || fail "New action not present"
}

# Tests a glob in a build= statement in an Android.bp file, which is interpreted
# during bootstrapping.
function test_glob_during_bootstrapping() {
  setup

  mkdir -p build/soong/picard
  cat > build/soong/picard/Android.bp <<'EOF'
build=["foo*.bp"]
EOF
  cat > build/soong/picard/fooa.bp <<'EOF'
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

  cat > build/soong/picard/picard.go <<'EOF'
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
  local -r mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  grep -q "Make it so" out/soong/build."${target_product}".ninja || fail "Original action not present"

  cat > build/soong/picard/foob.bp <<'EOF'
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

  cat > build/soong/picard/worf.go <<'EOF'
package worf

import "android/soong/picard"

func init() {
   picard.Message = "Engage."
}
EOF

  run_soong
  local -r mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Output Ninja file did not change"
  fi

  grep -q "Engage" out/soong/build."${target_product}".ninja || fail "New action not present"

  if grep -q "Make it so" out/soong/build."${target_product}".ninja; then
    fail "Original action still present"
  fi
}

function test_soong_docs_smoke() {
  setup

  run_soong soong_docs

  [[ -e "out/soong/docs/soong_build.html" ]] || fail "Documentation for main page not created"
  [[ -e "out/soong/docs/cc.html" ]] || fail "Documentation for C++ modules not created"
}

function test_null_build_after_soong_docs() {
  setup

  run_soong
  local -r ninja_mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  run_soong soong_docs
  local -r docs_mtime1=$(stat -c "%y" out/soong/docs/soong_build.html)

  run_soong soong_docs
  local -r docs_mtime2=$(stat -c "%y" out/soong/docs/soong_build.html)

  if [[ "$docs_mtime1" != "$docs_mtime2" ]]; then
    fail "Output Ninja file changed on null build"
  fi

  run_soong
  local -r ninja_mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  if [[ "$ninja_mtime1" != "$ninja_mtime2" ]]; then
    fail "Output Ninja file changed on null build"
  fi
}

function test_write_to_source_tree {
  setup
  mkdir -p a
  cat > a/Android.bp <<EOF
genrule {
  name: "write_to_source_tree",
  out: ["write_to_source_tree"],
  cmd: "touch file_in_source_tree && touch \$(out)",
}
EOF
  readonly EXPECTED_OUT=out/soong/.intermediates/a/write_to_source_tree/gen/write_to_source_tree
  readonly ERROR_LOG=${MOCK_TOP}/out/error.log
  readonly ERROR_MSG="Read-only file system"
  readonly ERROR_HINT_PATTERN="BUILD_BROKEN_SRC_DIR"
  # Test in ReadOnly source tree
  run_ninja BUILD_BROKEN_SRC_DIR_IS_WRITABLE=false ${EXPECTED_OUT} &> /dev/null && \
    fail "Write to source tree should not work in a ReadOnly source tree"

  if grep -q "${ERROR_MSG}" "${ERROR_LOG}" && grep -q "${ERROR_HINT_PATTERN}" "${ERROR_LOG}" ; then
    echo Error message and error hint found in logs >/dev/null
  else
    fail "Did not find Read-only error AND error hint in error.log"
  fi

  # Test in ReadWrite source tree
  run_ninja BUILD_BROKEN_SRC_DIR_IS_WRITABLE=true ${EXPECTED_OUT} &> /dev/null || \
    fail "Write to source tree did not succeed in a ReadWrite source tree"

  if  grep -q "${ERROR_MSG}\|${ERROR_HINT_PATTERN}" "${ERROR_LOG}" ; then
    fail "Found read-only error OR error hint in error.log"
  fi
}

function test_dump_json_module_graph() {
  setup
  run_soong json-module-graph
  if [[ ! -r "out/soong/module-graph.json" ]]; then
    fail "JSON file was not created"
  fi
}

function test_json_module_graph_back_and_forth_null_build() {
  setup

  run_soong
  local -r ninja_mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  run_soong json-module-graph
  local -r json_mtime1=$(stat -c "%y" out/soong/module-graph.json)

  run_soong
  local -r ninja_mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)
  if [[ "$ninja_mtime1" != "$ninja_mtime2" ]]; then
    fail "Output Ninja file changed after writing JSON module graph"
  fi

  run_soong json-module-graph
  local -r json_mtime2=$(stat -c "%y" out/soong/module-graph.json)
  if [[ "$json_mtime1" != "$json_mtime2" ]]; then
    fail "JSON module graph file changed after writing Ninja file"
  fi

}

function test_queryview_null_build() {
  setup

  run_soong queryview
  local -r output_mtime1=$(stat -c "%y" out/soong/queryview.marker)

  run_soong queryview
  local -r output_mtime2=$(stat -c "%y" out/soong/queryview.marker)

  if [[ "$output_mtime1" != "$output_mtime2" ]]; then
    fail "Queryview marker file changed on null build"
  fi
}

# This test verifies that adding a new glob to a blueprint file only
# causes build."${target_product}".ninja to be regenerated on the *next* build, and *not*
# the build after. (This is a regression test for a bug where globs
# resulted in two successive regenerations.)
function test_new_glob_incrementality {
  setup

  run_soong nothing
  local -r mtime1=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  mkdir -p globdefpkg/
  cat > globdefpkg/Android.bp <<'EOF'
filegroup {
  name: "fg_with_glob",
  srcs: ["*.txt"],
}
EOF

  run_soong nothing
  local -r mtime2=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  if [[ "$mtime1" == "$mtime2" ]]; then
    fail "Ninja file was not regenerated, despite a new bp file"
  fi

  run_soong nothing
  local -r mtime3=$(stat -c "%y" out/soong/build."${target_product}".ninja)

  if [[ "$mtime2" != "$mtime3" ]]; then
    fail "Ninja file was regenerated despite no previous bp changes"
  fi
}

scan_and_run_tests
