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

package cc

import (
	"android/soong/android"
)

func init() {
	android.RegisterParallelSingletonType("ndk_abi_dump", NdkAbiDumpSingleton)
	android.RegisterParallelSingletonType("ndk_abi_diff", NdkAbiDiffSingleton)
}

func getNdkAbiDumpInstallBase(ctx android.PathContext) android.OutputPath {
	return android.PathForOutput(ctx).Join(ctx, "abi-dumps/ndk")
}

func getNdkAbiDumpTimestampFile(ctx android.PathContext) android.OutputPath {
	return android.PathForOutput(ctx, "ndk_abi_dump.timestamp")
}

func NdkAbiDumpSingleton() android.Singleton {
	return &ndkAbiDumpSingleton{}
}

type ndkAbiDumpSingleton struct{}

func (n *ndkAbiDumpSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var depPaths android.Paths
	ctx.VisitAllModules(func(module android.Module) {
		if !module.Enabled() {
			return
		}

		if m, ok := module.(*Module); ok {
			if installer, ok := m.installer.(*stubDecorator); ok {
				if canDumpAbi(ctx.Config()) {
					depPaths = append(depPaths, installer.abiDumpPath)
				}
			}
		}
	})

	// `m dump-ndk-abi` will dump the NDK ABI.
	// `development/tools/ndk/update_ndk_abi.sh` will dump the NDK ABI and
	// update the golden copies in prebuilts/abi-dumps/ndk.
	ctx.Build(pctx, android.BuildParams{
		Rule:      android.Touch,
		Output:    getNdkAbiDumpTimestampFile(ctx),
		Implicits: depPaths,
	})

	ctx.Phony("dump-ndk-abi", getNdkAbiDumpTimestampFile(ctx))
}

func getNdkAbiDiffTimestampFile(ctx android.PathContext) android.WritablePath {
	return android.PathForOutput(ctx, "ndk_abi_diff.timestamp")
}

func NdkAbiDiffSingleton() android.Singleton {
	return &ndkAbiDiffSingleton{}
}

type ndkAbiDiffSingleton struct{}

func (n *ndkAbiDiffSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	var depPaths android.Paths
	ctx.VisitAllModules(func(module android.Module) {
		if m, ok := module.(android.Module); ok && !m.Enabled() {
			return
		}

		if m, ok := module.(*Module); ok {
			if installer, ok := m.installer.(*stubDecorator); ok {
				depPaths = append(depPaths, installer.abiDiffPaths...)
			}
		}
	})

	depPaths = append(depPaths, getNdkAbiDumpTimestampFile(ctx))

	// `m diff-ndk-abi` will diff the NDK ABI.
	ctx.Build(pctx, android.BuildParams{
		Rule:      android.Touch,
		Output:    getNdkAbiDiffTimestampFile(ctx),
		Implicits: depPaths,
	})

	ctx.Phony("diff-ndk-abi", getNdkAbiDiffTimestampFile(ctx))
}
