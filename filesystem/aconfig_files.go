// Copyright (C) 2024 The Android Open Source Project
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

package filesystem

import (
	"android/soong/android"
	"path/filepath"
	"strings"

	"github.com/google/blueprint/proptools"
)

func (f *filesystem) buildAconfigFlagsFiles(ctx android.ModuleContext, builder *android.RuleBuilder, specs map[string]android.PackagingSpec, dir android.Path) {
	if !proptools.Bool(f.properties.Gen_aconfig_flags_pb) {
		return
	}

	aconfigFlagsBuilderPath := android.PathForModuleOut(ctx, "aconfig_flags_builder.sh")
	aconfigToolPath := ctx.Config().HostToolPath(ctx, "aconfig")
	cmd := builder.Command().Tool(aconfigFlagsBuilderPath).Implicit(aconfigToolPath)

	installAconfigFlags := filepath.Join(dir.String(), "etc", "aconfig_flags_"+f.partitionName()+".pb")

	var sb strings.Builder
	sb.WriteString("set -e\n")
	sb.WriteString(aconfigToolPath.String())
	sb.WriteString(" dump-cache --dedup --format protobuf --out ")
	sb.WriteString(installAconfigFlags)
	sb.WriteString(" \\\n")

	var caches []string
	for _, ps := range specs {
		cmd.Implicits(ps.GetAconfigPaths())
		caches = append(caches, ps.GetAconfigPaths().Strings()...)
	}
	caches = android.SortedUniqueStrings(caches)

	for _, cache := range caches {
		sb.WriteString("  --cache ")
		sb.WriteString(cache)
		sb.WriteString(" \\\n")
	}
	sb.WriteRune('\n')

	android.WriteExecutableFileRuleVerbatim(ctx, aconfigFlagsBuilderPath, sb.String())
}
