// Copyright 2021 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package bloaty implements a singleton that measures binary (e.g. ELF
// executable, shared library or Rust rlib) section sizes at build time.
package bloaty

import (
	"android/soong/android"

	"github.com/google/blueprint"
)

const bloatyDescriptorExt = ".bloaty.csv"
const protoFilename = "binary_sizes.pb.gz"

var (
	fileSizeMeasurerKey blueprint.ProviderKey[measuredFiles]
	pctx                = android.NewPackageContext("android/soong/bloaty")

	// bloaty is used to measure a binary section sizes.
	bloaty = pctx.AndroidStaticRule("bloaty",
		blueprint.RuleParams{
			Command:     "${bloaty} -n 0 --csv ${in} > ${out}",
			CommandDeps: []string{"${bloaty}"},
		})

	// The bloaty merger script is used to combine the outputs from bloaty
	// into a single protobuf.
	bloatyMerger = pctx.AndroidStaticRule("bloatyMerger",
		blueprint.RuleParams{
			Command:        "${bloatyMerger} ${out}.lst ${out}",
			CommandDeps:    []string{"${bloatyMerger}"},
			Rspfile:        "${out}.lst",
			RspfileContent: "${in}",
		})
)

func init() {
	pctx.VariableConfigMethod("hostPrebuiltTag", android.Config.PrebuiltOS)
	pctx.SourcePathVariable("bloaty", "prebuilts/build-tools/${hostPrebuiltTag}/bin/bloaty")
	pctx.HostBinToolVariable("bloatyMerger", "bloaty_merger")
	android.RegisterParallelSingletonType("file_metrics", fileSizesSingleton)
	fileSizeMeasurerKey = blueprint.NewProvider[measuredFiles]()
}

// measuredFiles contains the paths of the files measured by a module.
type measuredFiles struct {
	paths []android.WritablePath
}

// MeasureSizeForPaths should be called by binary producers to measure the
// sizes of artifacts. It must only be called once per module; it will panic
// otherwise.
func MeasureSizeForPaths(ctx android.ModuleContext, paths ...android.OptionalPath) {
	mf := measuredFiles{}
	for _, p := range paths {
		if !p.Valid() {
			continue
		}
		if p, ok := p.Path().(android.WritablePath); ok {
			mf.paths = append(mf.paths, p)
		}
	}
	android.SetProvider(ctx, fileSizeMeasurerKey, mf)
}

type sizesSingleton struct{}

func fileSizesSingleton() android.Singleton {
	return &sizesSingleton{}
}

func (singleton *sizesSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var deps android.Paths
	ctx.VisitAllModules(func(m android.Module) {
		if !m.ExportedToMake() {
			return
		}
		filePaths, ok := android.SingletonModuleProvider(ctx, m, fileSizeMeasurerKey)
		if !ok {
			return
		}
		for _, path := range filePaths.paths {
			filePath := path.(android.ModuleOutPath)
			sizeFile := filePath.InSameDir(ctx, filePath.Base()+bloatyDescriptorExt)
			ctx.Build(pctx, android.BuildParams{
				Rule:        bloaty,
				Description: "bloaty " + filePath.Rel(),
				Input:       filePath,
				Output:      sizeFile,
			})
			deps = append(deps, sizeFile)
		}
	})

	ctx.Build(pctx, android.BuildParams{
		Rule:   bloatyMerger,
		Inputs: android.SortedUniquePaths(deps),
		Output: android.PathForOutput(ctx, protoFilename),
	})
}

func (singleton *sizesSingleton) MakeVars(ctx android.MakeVarsContext) {
	ctx.DistForGoalWithFilename("checkbuild", android.PathForOutput(ctx, protoFilename), protoFilename)
}
