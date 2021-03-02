// Copyright 2017 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package build

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"android/soong/shared"
	soong_metrics_proto "android/soong/ui/metrics/metrics_proto"

	"github.com/golang/protobuf/proto"
	"github.com/google/blueprint/microfactory"

	"android/soong/ui/metrics"
	"android/soong/ui/status"
)

// This uses Android.bp files and various tools to generate <builddir>/build.ninja.
//
// However, the execution of <builddir>/build.ninja happens later in build/soong/ui/build/build.go#Build()
//
// We want to rely on as few prebuilts as possible, so there is some bootstrapping here.
//
// "Microfactory" is a tool for compiling Go code. We use it to build two other tools:
// - minibp, used to generate build.ninja files. This is really build/blueprint/bootstrap/command.go#Main()
// - bpglob, used during incremental builds to identify files in a glob that have changed
//
// In reality, several build.ninja files are generated and/or used during the bootstrapping and build process.
// See build/blueprint/bootstrap/doc.go for more information.
//
func runSoong(ctx Context, config Config) {
	ctx.BeginTrace(metrics.RunSoong, "soong")
	defer ctx.EndTrace()

	// Use an anonymous inline function for tracing purposes (this pattern is used several times below).
	func() {
		ctx.BeginTrace(metrics.RunSoong, "blueprint bootstrap")
		defer ctx.EndTrace()

		// Use validations to depend on tests.
		args := []string{"-n"}

		if !config.skipSoongTests {
			// Run tests.
			args = append(args, "-t")
		}

		cmd := Command(ctx, config, "blueprint bootstrap", "build/blueprint/bootstrap.bash", args...)
		cmd.Environment.Set("BLUEPRINTDIR", "./build/blueprint")
		cmd.Environment.Set("BOOTSTRAP", "./build/blueprint/bootstrap.bash")
		cmd.Environment.Set("BUILDDIR", config.SoongOutDir())
		cmd.Environment.Set("GOROOT", "./"+filepath.Join("prebuilts/go", config.HostPrebuiltTag()))
		cmd.Environment.Set("BLUEPRINT_LIST_FILE", filepath.Join(config.FileListDir(), "Android.bp.list"))
		cmd.Environment.Set("NINJA_BUILDDIR", config.OutDir())
		cmd.Environment.Set("SRCDIR", ".")
		cmd.Environment.Set("TOPNAME", "Android.bp")
		cmd.Sandbox = soongSandbox

		cmd.RunAndPrintOrFatal()
	}()

	func() {
		ctx.BeginTrace(metrics.RunSoong, "environment check")
		defer ctx.EndTrace()

		envFile := filepath.Join(config.SoongOutDir(), ".soong.environment")
		getenv := func(k string) string {
			v, _ := config.Environment().Get(k)
			return v
		}
		if stale, _ := shared.StaleEnvFile(envFile, getenv); stale {
			os.Remove(envFile)
		}
	}()

	var cfg microfactory.Config
	cfg.Map("github.com/google/blueprint", "build/blueprint")

	cfg.TrimPath = absPath(ctx, ".")

	func() {
		ctx.BeginTrace(metrics.RunSoong, "minibp")
		defer ctx.EndTrace()

		minibp := filepath.Join(config.SoongOutDir(), ".minibootstrap/minibp")
		if _, err := microfactory.Build(&cfg, minibp, "github.com/google/blueprint/bootstrap/minibp"); err != nil {
			ctx.Fatalln("Failed to build minibp:", err)
		}
	}()

	func() {
		ctx.BeginTrace(metrics.RunSoong, "bpglob")
		defer ctx.EndTrace()

		bpglob := filepath.Join(config.SoongOutDir(), ".minibootstrap/bpglob")
		if _, err := microfactory.Build(&cfg, bpglob, "github.com/google/blueprint/bootstrap/bpglob"); err != nil {
			ctx.Fatalln("Failed to build bpglob:", err)
		}
	}()

	ninja := func(name, file string) {
		ctx.BeginTrace(metrics.RunSoong, name)
		defer ctx.EndTrace()

		fifo := filepath.Join(config.OutDir(), ".ninja_fifo")
		nr := status.NewNinjaReader(ctx, ctx.Status.StartTool(), fifo)
		defer nr.Close()

		cmd := Command(ctx, config, "soong "+name,
			config.PrebuiltBuildTool("ninja"),
			"-d", "keepdepfile",
			"-d", "stats",
			"-o", "usesphonyoutputs=yes",
			"-o", "preremoveoutputs=yes",
			"-w", "dupbuild=err",
			"-w", "outputdir=err",
			"-w", "missingoutfile=err",
			"-j", strconv.Itoa(config.Parallel()),
			"--frontend_file", fifo,
			"-f", filepath.Join(config.SoongOutDir(), file))

		// For Bazel mixed builds.
		cmd.Environment.Set("BAZEL_PATH", "./tools/bazel")
		cmd.Environment.Set("BAZEL_HOME", filepath.Join(config.BazelOutDir(), "bazelhome"))
		cmd.Environment.Set("BAZEL_OUTPUT_BASE", filepath.Join(config.BazelOutDir(), "output"))
		cmd.Environment.Set("BAZEL_WORKSPACE", absPath(ctx, "."))
		cmd.Environment.Set("BAZEL_METRICS_DIR", config.BazelMetricsDir())

		cmd.Environment.Set("SOONG_SANDBOX_SOONG_BUILD", "true")
		cmd.Sandbox = soongSandbox
		cmd.RunAndStreamOrFatal()
	}

	// This build generates .bootstrap/build.ninja, which is used in the next step.
	ninja("minibootstrap", ".minibootstrap/build.ninja")

	// This build generates <builddir>/build.ninja, which is used later by build/soong/ui/build/build.go#Build().
	ninja("bootstrap", ".bootstrap/build.ninja")

	var soongBuildMetrics *soong_metrics_proto.SoongBuildMetrics
	if shouldCollectBuildSoongMetrics(config) {
		soongBuildMetrics := loadSoongBuildMetrics(ctx, config)
		logSoongBuildMetrics(ctx, soongBuildMetrics)
	}

	distGzipFile(ctx, config, config.SoongNinjaFile(), "soong")

	if !config.SkipKati() {
		distGzipFile(ctx, config, config.SoongAndroidMk(), "soong")
		distGzipFile(ctx, config, config.SoongMakeVarsMk(), "soong")
	}

	if shouldCollectBuildSoongMetrics(config) && ctx.Metrics != nil {
		ctx.Metrics.SetSoongBuildMetrics(soongBuildMetrics)
	}
}

func shouldCollectBuildSoongMetrics(config Config) bool {
	// Do not collect metrics protobuf if the soong_build binary ran as the bp2build converter.
	return config.Environment().IsFalse("GENERATE_BAZEL_FILES")
}

func loadSoongBuildMetrics(ctx Context, config Config) *soong_metrics_proto.SoongBuildMetrics {
	soongBuildMetricsFile := filepath.Join(config.OutDir(), "soong", "soong_build_metrics.pb")
	buf, err := ioutil.ReadFile(soongBuildMetricsFile)
	if err != nil {
		ctx.Fatalf("Failed to load %s: %s", soongBuildMetricsFile, err)
	}
	soongBuildMetrics := &soong_metrics_proto.SoongBuildMetrics{}
	err = proto.Unmarshal(buf, soongBuildMetrics)
	if err != nil {
		ctx.Fatalf("Failed to unmarshal %s: %s", soongBuildMetricsFile, err)
	}
	return soongBuildMetrics
}

func logSoongBuildMetrics(ctx Context, metrics *soong_metrics_proto.SoongBuildMetrics) {
	ctx.Verbosef("soong_build metrics:")
	ctx.Verbosef(" modules: %v", metrics.GetModules())
	ctx.Verbosef(" variants: %v", metrics.GetVariants())
	ctx.Verbosef(" max heap size: %v MB", metrics.GetMaxHeapSize()/1e6)
	ctx.Verbosef(" total allocation count: %v", metrics.GetTotalAllocCount())
	ctx.Verbosef(" total allocation size: %v MB", metrics.GetTotalAllocSize()/1e6)

}
