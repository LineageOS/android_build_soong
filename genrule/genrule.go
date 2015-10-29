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
	"path/filepath"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"

	"android/soong"
	"android/soong/common"
)

func init() {
	soong.RegisterModuleType("gensrcs", GenSrcsFactory)
	soong.RegisterModuleType("genrule", GenRuleFactory)

	common.RegisterBottomUpMutator("genrule_deps", genruleDepsMutator)
}

var (
	pctx = blueprint.NewPackageContext("android/soong/genrule")
)

func init() {
	pctx.VariableConfigMethod("srcDir", common.Config.SrcDir)
	pctx.VariableConfigMethod("hostBin", common.Config.HostBin)
}

type SourceFileGenerator interface {
	GeneratedSourceFiles() []string
}

type HostToolProvider interface {
	HostToolPath() string
}

type generatorProperties struct {
	// command to run on one or more input files.  Available variables for substitution:
	// $in: one or more input files
	// $out: a single output file
	// $srcDir: the root directory of the source tree
	// The host bin directory will be in the path
	Cmd string

	// name of the module (if any) that produces the host executable.   Leave empty for
	// prebuilts or scripts that do not need a module to build them.
	Tool string
}

type generator struct {
	common.AndroidModuleBase

	properties generatorProperties

	tasks taskFunc

	deps []string
	rule blueprint.Rule

	outputFiles []string
}

type taskFunc func(ctx common.AndroidModuleContext) []generateTask

type generateTask struct {
	in  []string
	out string
}

func (g *generator) GeneratedSourceFiles() []string {
	return g.outputFiles
}

func genruleDepsMutator(ctx common.AndroidBottomUpMutatorContext) {
	if g, ok := ctx.Module().(*generator); ok {
		if g.properties.Tool != "" {
			ctx.AddFarVariationDependencies([]blueprint.Variation{{"hostordevice", common.Host.String()}},
				g.properties.Tool)
		}
	}
}

func (g *generator) GenerateAndroidBuildActions(ctx common.AndroidModuleContext) {
	g.rule = ctx.Rule(pctx, "generator", blueprint.RuleParams{
		Command: "PATH=$$PATH:$hostBin " + g.properties.Cmd,
	})

	ctx.VisitDirectDeps(func(module blueprint.Module) {
		if t, ok := module.(HostToolProvider); ok {
			p := t.HostToolPath()
			if p != "" {
				g.deps = append(g.deps, p)
			} else {
				ctx.ModuleErrorf("host tool %q missing output file", ctx.OtherModuleName(module))
			}
		} else {
			ctx.ModuleErrorf("unknown dependency %q", ctx.OtherModuleName(module))
		}
	})

	for _, task := range g.tasks(ctx) {
		g.generateSourceFile(ctx, task)
	}
}

func (g *generator) generateSourceFile(ctx common.AndroidModuleContext, task generateTask) {

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      g.rule,
		Inputs:    task.in,
		Implicits: g.deps,
		Outputs:   []string{task.out},
	})

	g.outputFiles = append(g.outputFiles, task.out)
}

func generatorFactory(tasks taskFunc, props ...interface{}) (blueprint.Module, []interface{}) {
	module := &generator{
		tasks: tasks,
	}

	props = append(props, &module.properties)

	return common.InitAndroidModule(module, props...)
}

func GenSrcsFactory() (blueprint.Module, []interface{}) {
	properties := &genSrcsProperties{}

	tasks := func(ctx common.AndroidModuleContext) []generateTask {
		srcFiles := ctx.ExpandSources(properties.Srcs, nil)
		tasks := make([]generateTask, 0, len(srcFiles))
		for _, in := range srcFiles {
			out := pathtools.ReplaceExtension(in, properties.Output_extension)
			out = filepath.Join(common.ModuleGenDir(ctx), out)
			tasks = append(tasks, generateTask{[]string{in}, out})
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

	tasks := func(ctx common.AndroidModuleContext) []generateTask {
		return []generateTask{
			{
				in:  ctx.ExpandSources(properties.Srcs, nil),
				out: filepath.Join(common.ModuleGenDir(ctx), properties.Out),
			},
		}
	}

	return generatorFactory(tasks, properties)
}

type genRuleProperties struct {
	// list of input files
	Srcs []string

	// name of the output file that will be generated
	Out string
}
