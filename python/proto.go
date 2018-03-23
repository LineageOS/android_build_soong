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

import (
	"android/soong/android"
	"strings"

	"github.com/google/blueprint"
)

func init() {
	pctx.HostBinToolVariable("protocCmd", "aprotoc")
}

var (
	proto = pctx.AndroidStaticRule("protoc",
		blueprint.RuleParams{
			Command: `rm -rf $out.tmp && mkdir -p $out.tmp && ` +
				`$protocCmd --python_out=$out.tmp -I $protoBase $protoFlags $in && ` +
				`$parCmd -o $out -P $pkgPath -C $out.tmp -D $out.tmp && rm -rf $out.tmp`,
			CommandDeps: []string{
				"$protocCmd",
				"$parCmd",
			},
		}, "protoBase", "protoFlags", "pkgPath")
)

func genProto(ctx android.ModuleContext, p *android.ProtoProperties,
	protoFile android.Path, protoFlags []string, pkgPath string) android.Path {
	srcJarFile := android.PathForModuleGen(ctx, protoFile.Base()+".srcszip")

	protoRoot := android.ProtoCanonicalPathFromRoot(ctx, p)

	var protoBase string
	if protoRoot {
		protoBase = "."
	} else {
		protoBase = strings.TrimSuffix(protoFile.String(), protoFile.Rel())
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        proto,
		Description: "protoc " + protoFile.Rel(),
		Output:      srcJarFile,
		Input:       protoFile,
		Args: map[string]string{
			"protoBase":  protoBase,
			"protoFlags": strings.Join(protoFlags, " "),
			"pkgPath":    pkgPath,
		},
	})

	return srcJarFile
}
