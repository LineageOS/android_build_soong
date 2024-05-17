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

func (f *filesystem) buildAconfigFlagsFiles(ctx android.ModuleContext, builder *android.RuleBuilder, specs map[string]android.PackagingSpec, dir android.OutputPath) {
	if !proptools.Bool(f.properties.Gen_aconfig_flags_pb) {
		return
	}

	aconfigFlagsBuilderPath := android.PathForModuleOut(ctx, "aconfig_flags_builder.sh")
	aconfigToolPath := ctx.Config().HostToolPath(ctx, "aconfig")
	cmd := builder.Command().Tool(aconfigFlagsBuilderPath).Implicit(aconfigToolPath)

	var caches []string
	for _, ps := range specs {
		cmd.Implicits(ps.GetAconfigPaths())
		caches = append(caches, ps.GetAconfigPaths().Strings()...)
	}
	caches = android.SortedUniqueStrings(caches)

	var sbCaches strings.Builder
	for _, cache := range caches {
		sbCaches.WriteString("  --cache ")
		sbCaches.WriteString(cache)
		sbCaches.WriteString(" \\\n")
	}
	sbCaches.WriteRune('\n')

	var sb strings.Builder
	sb.WriteString("set -e\n")

	installAconfigFlagsPath := dir.Join(ctx, "etc", "aconfig_flags.pb")
	sb.WriteString(aconfigToolPath.String())
	sb.WriteString(" dump-cache --dedup --format protobuf --out ")
	sb.WriteString(installAconfigFlagsPath.String())
	sb.WriteString(" \\\n")
	sb.WriteString(sbCaches.String())
	cmd.ImplicitOutput(installAconfigFlagsPath)

	installAconfigStorageDir := dir.Join(ctx, "etc", "aconfig")
	sb.WriteString("mkdir -p ")
	sb.WriteString(installAconfigStorageDir.String())
	sb.WriteRune('\n')

	generatePartitionAconfigStorageFile := func(fileType, fileName string) {
		sb.WriteString(aconfigToolPath.String())
		sb.WriteString(" create-storage --container ")
		sb.WriteString(f.PartitionType())
		sb.WriteString(" --file ")
		sb.WriteString(fileType)
		sb.WriteString(" --out ")
		sb.WriteString(filepath.Join(installAconfigStorageDir.String(), fileName))
		sb.WriteString(" \\\n")
		sb.WriteString(sbCaches.String())
		cmd.ImplicitOutput(installAconfigStorageDir.Join(ctx, fileName))
	}
	generatePartitionAconfigStorageFile("package_map", "package.map")
	generatePartitionAconfigStorageFile("flag_map", "flag.map")
	generatePartitionAconfigStorageFile("flag_val", "flag.val")

	android.WriteExecutableFileRuleVerbatim(ctx, aconfigFlagsBuilderPath, sb.String())
}
