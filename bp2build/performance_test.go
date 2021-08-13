// Copyright 2021 Google Inc. All rights reserved.
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

package bp2build

// to run the benchmarks in this file, you must run go test with the -bench.
// The benchmarked portion will run for the specified time (can be set via -benchtime)
// This can mean if you are benchmarking a faster portion of a larger operation, it will take
// longer.
// If you are seeing a small number of iterations for a specific run, the data is less reliable, to
// run for longer, set -benchtime to a larger value.

import (
	"android/soong/android"
	"fmt"
	"math"
	"strings"
	"testing"
)

func genCustomModule(i int, convert bool) string {
	var conversionString string
	if convert {
		conversionString = `bazel_module: { bp2build_available: true },`
	}
	return fmt.Sprintf(`
custom {
    name: "arch_paths_%[1]d",
    string_list_prop: ["\t", "\n"],
    string_prop: "a\t\n\r",
    arch_paths: ["outer", ":outer_dep_%[1]d"],
    arch: {
      x86: {
        arch_paths: ["abc", ":x86_dep_%[1]d"],
      },
      x86_64: {
        arch_paths: ["64bit"],
        arch_paths_exclude: ["outer"],
      },
    },
		%[2]s
}

custom {
    name: "outer_dep_%[1]d",
		%[2]s
}

custom {
    name: "x86_dep_%[1]d",
		%[2]s
}
`, i, conversionString)
}

func genCustomModuleBp(pctConverted float64) string {
	modules := 100

	bp := make([]string, 0, modules)
	toConvert := int(math.Round(float64(modules) * pctConverted))

	for i := 0; i < modules; i++ {
		bp = append(bp, genCustomModule(i, i < toConvert))
	}
	return strings.Join(bp, "\n\n")
}

var pctToConvert = []float64{0.0, 0.01, 0.05, 0.10, 0.25, 0.5, 0.75, 1.0}

func BenchmarkManyModulesFull(b *testing.B) {
	dir := "."
	for _, tcSize := range pctToConvert {

		b.Run(fmt.Sprintf("pctConverted %f", tcSize), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				// setup we don't want to measure
				config := android.TestConfig(buildDir, nil, genCustomModuleBp(tcSize), nil)
				ctx := android.NewTestContext(config)

				registerCustomModuleForBp2buildConversion(ctx)
				codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)

				b.StartTimer()
				_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
				if len(errs) > 0 {
					b.Fatalf("Unexpected errors: %s", errs)
				}

				_, errs = ctx.ResolveDependencies(config)
				if len(errs) > 0 {
					b.Fatalf("Unexpected errors: %s", errs)
				}

				generateBazelTargetsForDir(codegenCtx, dir)
				b.StopTimer()
			}
		})
	}
}

func BenchmarkManyModulesResolveDependencies(b *testing.B) {
	dir := "."
	for _, tcSize := range pctToConvert {

		b.Run(fmt.Sprintf("pctConverted %f", tcSize), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				// setup we don't want to measure
				config := android.TestConfig(buildDir, nil, genCustomModuleBp(tcSize), nil)
				ctx := android.NewTestContext(config)

				registerCustomModuleForBp2buildConversion(ctx)
				codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)

				_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
				if len(errs) > 0 {
					b.Fatalf("Unexpected errors: %s", errs)
				}

				b.StartTimer()
				_, errs = ctx.ResolveDependencies(config)
				b.StopTimer()
				if len(errs) > 0 {
					b.Fatalf("Unexpected errors: %s", errs)
				}

				generateBazelTargetsForDir(codegenCtx, dir)
			}
		})
	}
}

func BenchmarkManyModulesGenerateBazelTargetsForDir(b *testing.B) {
	dir := "."
	for _, tcSize := range pctToConvert {

		b.Run(fmt.Sprintf("pctConverted %f", tcSize), func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				b.StopTimer()
				// setup we don't want to measure
				config := android.TestConfig(buildDir, nil, genCustomModuleBp(tcSize), nil)
				ctx := android.NewTestContext(config)

				registerCustomModuleForBp2buildConversion(ctx)
				codegenCtx := NewCodegenContext(config, *ctx.Context, Bp2Build)

				_, errs := ctx.ParseFileList(dir, []string{"Android.bp"})
				if len(errs) > 0 {
					b.Fatalf("Unexpected errors: %s", errs)
				}

				_, errs = ctx.ResolveDependencies(config)
				if len(errs) > 0 {
					b.Fatalf("Unexpected errors: %s", errs)
				}

				b.StartTimer()
				generateBazelTargetsForDir(codegenCtx, dir)
				b.StopTimer()
			}
		})
	}
}
