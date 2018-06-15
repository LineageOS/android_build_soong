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

package cc

import (
	"strings"
	"sync"

	"android/soong/android"
	"android/soong/cc/config"
)

var (
	lsdumpPaths []string
	sabiLock    sync.Mutex
)

type SAbiProperties struct {
	CreateSAbiDumps        bool `blueprint:"mutated"`
	ReexportedIncludeFlags []string
}

type sabi struct {
	Properties SAbiProperties
}

func (sabimod *sabi) props() []interface{} {
	return []interface{}{&sabimod.Properties}
}

func (sabimod *sabi) begin(ctx BaseModuleContext) {}

func (sabimod *sabi) deps(ctx BaseModuleContext, deps Deps) Deps {
	return deps
}

func inListWithPrefixSearch(flag string, filter []string) bool {
	// Assuming the filter is small enough.
	// If the suffix of a filter element is *, try matching prefixes as well.
	for _, f := range filter {
		if (f == flag) || (strings.HasSuffix(f, "*") && strings.HasPrefix(flag, strings.TrimSuffix(f, "*"))) {
			return true
		}
	}
	return false
}

func filterOutWithPrefix(list []string, filter []string) (remainder []string) {
	// Go through the filter, matching and optionally doing a prefix search for list elements.
	for _, l := range list {
		if !inListWithPrefixSearch(l, filter) {
			remainder = append(remainder, l)
		}
	}
	return
}

func (sabimod *sabi) flags(ctx ModuleContext, flags Flags) Flags {
	// Assuming that the cflags which clang LibTooling tools cannot
	// understand have not been converted to ninja variables yet.
	flags.ToolingCFlags = filterOutWithPrefix(flags.CFlags, config.ClangLibToolingUnknownCflags)

	// RSClang does not support recent mcpu option likes exynos-m2.
	// So we need overriding mcpu option when we want to use it.
	mappedArch := map[string]string{
		"exynos-m2":  "cortex-a53",
		"cortex-a55": "cortex-a53",
		"cortex-a75": "cortex-a57",
	}
	if arch, ok := mappedArch[ctx.Arch().CpuVariant]; ok {
		flags.ToolingCFlags = append(flags.ToolingCFlags, "-mcpu="+arch)
	}

	return flags
}

func sabiDepsMutator(mctx android.TopDownMutatorContext) {
	if c, ok := mctx.Module().(*Module); ok &&
		((c.isVndk() && c.useVndk()) || inList(c.Name(), llndkLibraries) ||
			(c.sabi != nil && c.sabi.Properties.CreateSAbiDumps)) {
		mctx.VisitDirectDeps(func(m android.Module) {
			tag := mctx.OtherModuleDependencyTag(m)
			switch tag {
			case staticDepTag, staticExportDepTag, lateStaticDepTag, wholeStaticDepTag:

				cc, _ := m.(*Module)
				if cc == nil {
					return
				}
				cc.sabi.Properties.CreateSAbiDumps = true
			}
		})
	}
}
