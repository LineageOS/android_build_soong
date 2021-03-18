#!/bin/bash -eu

# This test exercises the bootstrapping process of the build system
# in a source tree that only contains enough files for Bazel and Soong to work.

HARDWIRED_MOCK_TOP=
# Uncomment this to be able to view the source tree after a test is run
# HARDWIRED_MOCK_TOP=/tmp/td

REAL_TOP="$(readlink -f "$(dirname "$0")"/../..)"

function fail {
  echo ERROR: $1
  exit 1
}

function copy_directory() {
  local dir="$1"
  local parent="$(dirname "$dir")"

  mkdir -p "$MOCK_TOP/$parent"
  cp -R "$REAL_TOP/$dir" "$MOCK_TOP/$parent"
}

function symlink_file() {
  local file="$1"

  mkdir -p "$MOCK_TOP/$(dirname "$file")"
  ln -s "$REAL_TOP/$file" "$MOCK_TOP/$file"
}

function symlink_directory() {
  local dir="$1"

  mkdir -p "$MOCK_TOP/$dir"
  # We need to symlink the contents of the directory individually instead of
  # using one symlink for the whole directory because finder.go doesn't follow
  # symlinks when looking for Android.bp files
  for i in $(ls "$REAL_TOP/$dir"); do
    local target="$MOCK_TOP/$dir/$i"
    local source="$REAL_TOP/$dir/$i"

    if [[ -e "$target" ]]; then
      if [[ ! -d "$source" || ! -d "$target" ]]; then
        fail "Trying to symlink $dir twice"
      fi
    else
      ln -s "$REAL_TOP/$dir/$i" "$MOCK_TOP/$dir/$i";
    fi
  done
}

function setup_bazel() {
  copy_directory build/bazel

  symlink_directory prebuilts/bazel
  symlink_directory prebuilts/jdk

  symlink_file WORKSPACE
  symlink_file tools/bazel
}

function setup() {
  if [[ ! -z "$HARDWIRED_MOCK_TOP" ]]; then
    MOCK_TOP="$HARDWIRED_MOCK_TOP"
    rm -fr "$MOCK_TOP"
    mkdir -p "$MOCK_TOP"
  else
    MOCK_TOP=$(mktemp -t -d st.XXXXX)
    trap 'echo cd / && echo rm -fr "$MOCK_TOP"' EXIT
  fi

  echo "Test case: ${FUNCNAME[1]}, mock top path: $MOCK_TOP"
  cd "$MOCK_TOP"

  copy_directory build/blueprint
  copy_directory build/soong

  symlink_directory prebuilts/go
  symlink_directory prebuilts/build-tools
  symlink_directory external/golang-protobuf

  touch "$MOCK_TOP/Android.bp"

  export ALLOW_MISSING_DEPENDENCIES=true

  mkdir -p out/soong
}

function run_soong() {
  build/soong/soong_ui.bash --make-mode --skip-ninja --skip-make --skip-soong-tests
}

function test_smoke {
  setup
  run_soong
}

function test_bazel_smoke {
  setup
  setup_bazel

  tools/bazel info

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

  grep -q "^# Module:.*my_little_binary_host$" out/soong/build.ninja && fail "Old module in output"
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

test_bazel_smoke
test_smoke
test_null_build
test_soong_build_rebuilt_if_blueprint_changes
test_add_file_to_glob
test_add_android_bp
test_change_android_bp
test_delete_android_bp
test_add_file_to_soong_build
