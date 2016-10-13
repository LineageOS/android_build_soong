// Copyright 2015 Google Inc. All rights reserved.
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

package genrule

import (
	"os"

	"github.com/google/blueprint"

	"android/soong/android"
)

func init() {
	android.RegisterModuleType("gensrcs", GenSrcsFactory)
	android.RegisterModuleType("genrule", GenRuleFactory)
}

var (
	pctx = android.NewPackageContext("android/soong/genrule")
)

func init() {
	pctx.SourcePathVariable("srcDir", "")
	pctx.HostBinToolVariable("hostBin", "")
}

type SourceFileGenerator interface {
	GeneratedSourceFiles() android.Paths
	GeneratedHeaderDir() android.Path
}

type HostToolProvider interface {
	HostToolPath() android.OptionalPath
}

type generatorProperties struct {
	// command to run on one or more input files.  Available variables for substitution:
	// $tool: the path to the `tool` or `tool_file`
	// $in: one or more input files
	// $out: a single output file
	// $srcDir: the root directory of the source tree
	// $genDir: the sandbox directory for this tool; contains $out
	// The host bin directory will be in the path
	Cmd string

	// name of the module (if any) that produces the host executable.   Leave empty for
	// prebuilts or scripts that do not need a module to build them.
	Tool string

	// Local file that is used as the tool
	Tool_file string
}

type generator struct {
	android.ModuleBase

	properties generatorProperties

	tasks taskFunc

	deps android.Paths
	rule blueprint.Rule

	genPath android.Path

	outputFiles android.Paths
}

type taskFunc func(ctx android.ModuleContext) []generateTask

type generateTask struct {
	in  android.Paths
	out android.WritablePaths
}

func (g *generator) GeneratedSourceFiles() android.Paths {
	return g.outputFiles
}

func (g *generator) GeneratedHeaderDir() android.Path {
	return g.genPath
}

func (g *generator) DepsMutator(ctx android.BottomUpMutatorContext) {
	if g, ok := ctx.Module().(*generator); ok {
		if g.properties.Tool != "" {
			ctx.AddFarVariationDependencies([]blueprint.Variation{
				{"arch", ctx.AConfig().BuildOsVariant},
			}, nil, g.properties.Tool)
		}
	}
}

func (g *generator) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if g.properties.Tool != "" && g.properties.Tool_file != "" {
		ctx.ModuleErrorf("`tool` and `tool_file` may not be specified at the same time")
		return
	}

	g.genPath = android.PathForModuleGen(ctx, "")

	cmd := os.Expand(g.properties.Cmd, func(name string) string {
		switch name {
		case "$":
			return "$$"
		case "tool":
			return "${tool}"
		case "in":
			return "${in}"
		case "out":
			return "${out}"
		case "srcDir":
			return "${srcDir}"
		case "genDir":
			return g.genPath.String()
		default:
			ctx.PropertyErrorf("cmd", "unknown variable '%s'", name)
		}
		return ""
	})

	g.rule = ctx.Rule(pctx, "generator", blueprint.RuleParams{
		Command: "PATH=$$PATH:$hostBin " + cmd,
	}, "tool")

	var tool string
	if g.properties.Tool_file != "" {
		toolpath := android.PathForModuleSrc(ctx, g.properties.Tool_file)
		g.deps = append(g.deps, toolpath)
		tool = toolpath.String()
	} else if g.properties.Tool != "" {
		ctx.VisitDirectDeps(func(module blueprint.Module) {
			if t, ok := module.(HostToolProvider); ok {
				p := t.HostToolPath()
				if p.Valid() {
					g.deps = append(g.deps, p.Path())
					tool = p.String()
				} else {
					ctx.ModuleErrorf("host tool %q missing output file", ctx.OtherModuleName(module))
				}
			} else {
				ctx.ModuleErrorf("unknown dependency %q", ctx.OtherModuleName(module))
			}
		})
	}

	for _, task := range g.tasks(ctx) {
		g.generateSourceFile(ctx, task, tool)
	}
}

func (g *generator) generateSourceFile(ctx android.ModuleContext, task generateTask, tool string) {
	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:      g.rule,
		Outputs:   task.out,
		Inputs:    task.in,
		Implicits: g.deps,
		Args: map[string]string{
			"tool": tool,
		},
	})

	for _, outputFile := range task.out {
		g.outputFiles = append(g.outputFiles, outputFile)
	}
}

func generatorFactory(tasks taskFunc, props ...interface{}) (blueprint.Module, []interface{}) {
	module := &generator{
		tasks: tasks,
	}

	props = append(props, &module.properties)

	return android.InitAndroidModule(module, props...)
}

func GenSrcsFactory() (blueprint.Module, []interface{}) {
	properties := &genSrcsProperties{}

	tasks := func(ctx android.ModuleContext) []generateTask {
		srcFiles := ctx.ExpandSources(properties.Srcs, nil)
		tasks := make([]generateTask, 0, len(srcFiles))
		for _, in := range srcFiles {
			tasks = append(tasks, generateTask{
				in:  android.Paths{in},
				out: android.WritablePaths{android.GenPathWithExt(ctx, in, properties.Output_extension)},
			})
		}
		return tasks
	}

	return generatorFactory(tasks, properties)
}

type genSrcsProperties struct {
	// list of input files
	Srcs []string

	// extension that will be substituted for each output file
	Output_extension string
}

func GenRuleFactory() (blueprint.Module, []interface{}) {
	properties := &genRuleProperties{}

	tasks := func(ctx android.ModuleContext) []generateTask {
		outs := make(android.WritablePaths, len(properties.Out))
		for i, out := range properties.Out {
			outs[i] = android.PathForModuleGen(ctx, out)
		}
		return []generateTask{
			{
				in:  ctx.ExpandSources(properties.Srcs, nil),
				out: outs,
			},
		}
	}

	return generatorFactory(tasks, properties)
}

type genRuleProperties struct {
	// list of input files
	Srcs []string

	// names of the output files that will be generated
	Out []string
}
