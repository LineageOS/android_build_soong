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

package python

// This file contains Ninja build actions for building Python program.

import (
	"strings"

	"android/soong/android"

	"github.com/google/blueprint"
	_ "github.com/google/blueprint/bootstrap"
)

var (
	pctx = android.NewPackageContext("android/soong/python")

	zip = pctx.AndroidStaticRule("zip",
		blueprint.RuleParams{
			Command:     `$parCmd -o $out $args`,
			CommandDeps: []string{"$parCmd"},
		},
		"args")

	combineZip = pctx.AndroidStaticRule("combineZip",
		blueprint.RuleParams{
			Command:     `$mergeParCmd $out $in`,
			CommandDeps: []string{"$mergeParCmd"},
		},
	)

	hostPar = pctx.AndroidStaticRule("hostPar",
		blueprint.RuleParams{
			Command: `sed -e 's/%interpreter%/$interp/g' -e 's/%main%/$main/g' $template > $stub && ` +
				`echo "#!/usr/bin/env python" >${out}.prefix &&` +
				`$mergeParCmd -p --prefix ${out}.prefix -pm $stub $out $srcsZips && ` +
				`chmod +x $out && (rm -f $stub; rm -f ${out}.prefix)`,
			CommandDeps: []string{"$mergeParCmd"},
		},
		"interp", "main", "template", "stub", "srcsZips")

	embeddedPar = pctx.AndroidStaticRule("embeddedPar",
		blueprint.RuleParams{
			Command: `rm -f $out.main && ` +
				`sed 's/ENTRY_POINT/$main/' build/soong/python/scripts/main.py >$out.main &&` +
				`$mergeParCmd -p -pm $out.main --prefix $launcher $out $srcsZips && ` +
				`chmod +x $out && rm -rf $out.main`,
			CommandDeps: []string{"$mergeParCmd", "$parCmd", "build/soong/python/scripts/main.py"},
		},
		"main", "srcsZips", "launcher")

	embeddedParNoMain = pctx.AndroidStaticRule("embeddedParNoMain",
		blueprint.RuleParams{
			Command: `$mergeParCmd -p --prefix $launcher $out $srcsZips && ` +
				`chmod +x $out`,
			CommandDeps: []string{"$mergeParCmd"},
		},
		"srcsZips", "launcher")
)

func init() {
	pctx.Import("github.com/google/blueprint/bootstrap")
	pctx.Import("android/soong/android")

	pctx.HostBinToolVariable("parCmd", "soong_zip")
	pctx.HostBinToolVariable("mergeParCmd", "merge_zips")
}

func registerBuildActionForParFile(ctx android.ModuleContext, embeddedLauncher bool,
	launcherPath android.OptionalPath, interpreter, main, binName string,
	srcsZips android.Paths) android.Path {

	// .intermediate output path for bin executable.
	binFile := android.PathForModuleOut(ctx, binName)

	// implicit dependency for parFile build action.
	implicits := srcsZips

	if !embeddedLauncher {
		// the path of stub_template_host.txt from source tree.
		template := android.PathForSource(ctx, stubTemplateHost)
		implicits = append(implicits, template)

		// intermediate output path for __main__.py
		stub := android.PathForModuleOut(ctx, mainFileName).String()

		ctx.Build(pctx, android.BuildParams{
			Rule:        hostPar,
			Description: "host python archive",
			Output:      binFile,
			Implicits:   implicits,
			Args: map[string]string{
				"interp":   strings.Replace(interpreter, "/", `\/`, -1),
				"main":     strings.Replace(main, "/", `\/`, -1),
				"template": template.String(),
				"stub":     stub,
				"srcsZips": strings.Join(srcsZips.Strings(), " "),
			},
		})
	} else if launcherPath.Valid() {
		// added launcherPath to the implicits Ninja dependencies.
		implicits = append(implicits, launcherPath.Path())

		if main == "" {
			ctx.Build(pctx, android.BuildParams{
				Rule:        embeddedParNoMain,
				Description: "embedded python archive",
				Output:      binFile,
				Implicits:   implicits,
				Args: map[string]string{
					"srcsZips": strings.Join(srcsZips.Strings(), " "),
					"launcher": launcherPath.String(),
				},
			})
		} else {
			ctx.Build(pctx, android.BuildParams{
				Rule:        embeddedPar,
				Description: "embedded python archive",
				Output:      binFile,
				Implicits:   implicits,
				Args: map[string]string{
					"main":     strings.Replace(strings.TrimSuffix(main, pyExt), "/", ".", -1),
					"srcsZips": strings.Join(srcsZips.Strings(), " "),
					"launcher": launcherPath.String(),
				},
			})
		}
	}

	return binFile
}
