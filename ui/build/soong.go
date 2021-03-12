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

func writeEnvironmentFile(ctx Context, envFile string, envDeps map[string]string) error {
	data, err := shared.EnvFileContents(envDeps)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(envFile, data, 0644)
}

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

	// We have two environment files: .available is the one with every variable,
	// .used with the ones that were actually used. The latter is used to
	// determine whether Soong needs to be re-run since why re-run it if only
	// unused variables were changed?
	envFile := filepath.Join(config.SoongOutDir(), "soong.environment.available")

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

	soongBuildEnv := config.Environment().Copy()
	soongBuildEnv.Set("TOP", os.Getenv("TOP"))
	// These two dependencies are read from bootstrap.go, but also need to be here
	// so that soong_build can declare a dependency on them
	soongBuildEnv.Set("SOONG_DELVE", os.Getenv("SOONG_DELVE"))
	soongBuildEnv.Set("SOONG_DELVE_PATH", os.Getenv("SOONG_DELVE_PATH"))
	soongBuildEnv.Set("SOONG_OUTDIR", config.SoongOutDir())
	// For Bazel mixed builds.
	soongBuildEnv.Set("BAZEL_PATH", "./tools/bazel")
	soongBuildEnv.Set("BAZEL_HOME", filepath.Join(config.BazelOutDir(), "bazelhome"))
	soongBuildEnv.Set("BAZEL_OUTPUT_BASE", filepath.Join(config.BazelOutDir(), "output"))
	soongBuildEnv.Set("BAZEL_WORKSPACE", absPath(ctx, "."))
	soongBuildEnv.Set("BAZEL_METRICS_DIR", config.BazelMetricsDir())

	err := writeEnvironmentFile(ctx, envFile, soongBuildEnv.AsMap())
	if err != nil {
		ctx.Fatalf("failed to write environment file %s: %s", envFile, err)
	}

	func() {
		ctx.BeginTrace(metrics.RunSoong, "environment check")
		defer ctx.EndTrace()

		envFile := filepath.Join(config.SoongOutDir(), "soong.environment.used")
		getenv := func(k string) string {
			v, _ := soongBuildEnv.Get(k)
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

		var ninjaEnv Environment

		// This is currently how the command line to invoke soong_build finds the
		// root of the source tree and the output root
		ninjaEnv.Set("TOP", os.Getenv("TOP"))
		ninjaEnv.Set("SOONG_OUTDIR", config.SoongOutDir())

		// For debugging
		if os.Getenv("SOONG_DELVE") != "" {
			ninjaEnv.Set("SOONG_DELVE", os.Getenv("SOONG_DELVE"))
			ninjaEnv.Set("SOONG_DELVE_PATH", shared.ResolveDelveBinary())
		}

		cmd.Environment = &ninjaEnv
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
