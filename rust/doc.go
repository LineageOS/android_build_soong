// Copyright 2021 The Android Open Source Project
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

package rust

import (
	"android/soong/android"
)

func init() {
	android.RegisterParallelSingletonType("rustdoc", RustdocSingleton)
}

func RustdocSingleton() android.Singleton {
	return &rustdocSingleton{}
}

type rustdocSingleton struct{}

func (n *rustdocSingleton) GenerateBuildActions(ctx android.SingletonContext) {
	docDir := android.PathForOutput(ctx, "rustdoc")
	docZip := android.PathForOutput(ctx, "rustdoc.zip")
	rule := android.NewRuleBuilder(pctx, ctx)
	zipCmd := rule.Command().BuiltTool("soong_zip").
		FlagWithOutput("-o ", docZip).
		FlagWithArg("-C ", docDir.String()).
		FlagWithArg("-D ", docDir.String())

	ctx.VisitAllModules(func(module android.Module) {
		if !module.Enabled() {
			return
		}

		if m, ok := module.(*Module); ok {
			if m.docTimestampFile.Valid() {
				zipCmd.Implicit(m.docTimestampFile.Path())
			}
		}
	})
	rule.Build("rustdoc-zip", "Zipping all built Rust documentation...")
	ctx.Phony("rustdoc", docZip)
}
