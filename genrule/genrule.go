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
	"fmt"
	"path"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/bootstrap"

	"android/soong/android"
	"android/soong/shared"
	"path/filepath"
)

func init() {
	android.RegisterModuleType("gensrcs", GenSrcsFactory)
	android.RegisterModuleType("genrule", GenRuleFactory)
}

var (
	pctx = android.NewPackageContext("android/soong/genrule")
)

func init() {
	pctx.HostBinToolVariable("sboxCmd", "sbox")
}

type SourceFileGenerator interface {
	GeneratedSourceFiles() android.Paths
	GeneratedHeaderDirs() android.Paths
}

type HostToolProvider interface {
	HostToolPath() android.OptionalPath
}

type hostToolDependencyTag struct {
	blueprint.BaseDependencyTag
}

var hostToolDepTag hostToolDependencyTag

type generatorProperties struct {
	// The command to run on one or more input files. Cmd supports substitution of a few variables
	// (the actual substitution is implemented in GenerateAndroidBuildActions below)
	//
	// Available variables for substitution:
	//
	//  $(location): the path to the first entry in tools or tool_files
	//  $(location <label>): the path to the tool or tool_file with name <label>
	//  $(in): one or more input files
	//  $(out): a single output file
	//  $(depfile): a file to which dependencies will be written, if the depfile property is set to true
	//  $(genDir): the sandbox directory for this tool; contains $(out)
	//  $$: a literal $
	//
	// All files used must be declared as inputs (to ensure proper up-to-date checks).
	// Use "$(in)" directly in Cmd to ensure that all inputs used are declared.
	Cmd string

	// Enable reading a file containing dependencies in gcc format after the command completes
	Depfile bool

	// name of the modules (if any) that produces the host executable.   Leave empty for
	// prebuilts or scripts that do not need a module to build them.
	Tools []string

	// Local file that is used as the tool
	Tool_files []string

	// List of directories to export generated headers from
	Export_include_dirs []string

	// list of input files
	Srcs []string
}

type Module struct {
	android.ModuleBase

	// For other packages to make their own genrules with extra
	// properties
	Extra interface{}

	properties generatorProperties

	tasks taskFunc

	deps android.Paths
	rule blueprint.Rule

	exportedIncludeDirs android.Paths

	outputFiles android.Paths
}

type taskFunc func(ctx android.ModuleContext, srcFiles android.Paths) []generateTask

type generateTask struct {
	in  android.Paths
	out android.WritablePaths
}

func (g *Module) GeneratedSourceFiles() android.Paths {
	return g.outputFiles
}

func (g *Module) Srcs() android.Paths {
	return g.outputFiles
}

func (g *Module) GeneratedHeaderDirs() android.Paths {
	return g.exportedIncludeDirs
}

func (g *Module) DepsMutator(ctx android.BottomUpMutatorContext) {
	android.ExtractSourcesDeps(ctx, g.properties.Srcs)
	if g, ok := ctx.Module().(*Module); ok {
		if len(g.properties.Tools) > 0 {
			ctx.AddFarVariationDependencies([]blueprint.Variation{
				{"arch", ctx.AConfig().BuildOsVariant},
			}, hostToolDepTag, g.properties.Tools...)
		}
	}
}

func (g *Module) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(g.properties.Tools) == 0 && len(g.properties.Tool_files) == 0 {
		ctx.ModuleErrorf("at least one `tools` or `tool_files` is required")
		return
	}

	if len(g.properties.Export_include_dirs) > 0 {
		for _, dir := range g.properties.Export_include_dirs {
			g.exportedIncludeDirs = append(g.exportedIncludeDirs,
				android.PathForModuleGen(ctx, ctx.ModuleDir(), dir))
		}
	} else {
		g.exportedIncludeDirs = append(g.exportedIncludeDirs, android.PathForModuleGen(ctx, ""))
	}

	tools := map[string]android.Path{}

	if len(g.properties.Tools) > 0 {
		ctx.VisitDirectDeps(func(module android.Module) {
			switch ctx.OtherModuleDependencyTag(module) {
			case android.SourceDepTag:
				// Nothing to do
			case hostToolDepTag:
				tool := ctx.OtherModuleName(module)
				var path android.OptionalPath

				if t, ok := module.(HostToolProvider); ok {
					path = t.HostToolPath()
				} else if t, ok := module.(bootstrap.GoBinaryTool); ok {
					if s, err := filepath.Rel(android.PathForOutput(ctx).String(), t.InstallPath()); err == nil {
						path = android.OptionalPathForPath(android.PathForOutput(ctx, s))
					} else {
						ctx.ModuleErrorf("cannot find path for %q: %v", tool, err)
						break
					}
				} else {
					ctx.ModuleErrorf("%q is not a host tool provider", tool)
					break
				}

				if path.Valid() {
					g.deps = append(g.deps, path.Path())
					if _, exists := tools[tool]; !exists {
						tools[tool] = path.Path()
					} else {
						ctx.ModuleErrorf("multiple tools for %q, %q and %q", tool, tools[tool], path.Path().String())
					}
				} else {
					ctx.ModuleErrorf("host tool %q missing output file", tool)
				}
			default:
				ctx.ModuleErrorf("unknown dependency on %q", ctx.OtherModuleName(module))
			}
		})
	}

	if ctx.Failed() {
		return
	}

	for _, tool := range g.properties.Tool_files {
		toolPath := android.PathForModuleSrc(ctx, tool)
		g.deps = append(g.deps, toolPath)
		if _, exists := tools[tool]; !exists {
			tools[tool] = toolPath
		} else {
			ctx.ModuleErrorf("multiple tools for %q, %q and %q", tool, tools[tool], toolPath.String())
		}
	}

	rawCommand, err := android.Expand(g.properties.Cmd, func(name string) (string, error) {
		switch name {
		case "location":
			if len(g.properties.Tools) > 0 {
				return tools[g.properties.Tools[0]].String(), nil
			} else {
				return tools[g.properties.Tool_files[0]].String(), nil
			}
		case "in":
			return "${in}", nil
		case "out":
			return "__SBOX_OUT_FILES__", nil
		case "depfile":
			if !g.properties.Depfile {
				return "", fmt.Errorf("$(depfile) used without depfile property")
			}
			return "${depfile}", nil
		case "genDir":
			genPath := android.PathForModuleGen(ctx, "").String()
			var relativePath string
			var err error
			outputPath := android.PathForOutput(ctx).String()
			relativePath, err = filepath.Rel(outputPath, genPath)
			if err != nil {
				panic(err)
			}
			return path.Join("__SBOX_OUT_DIR__", relativePath), nil
		default:
			if strings.HasPrefix(name, "location ") {
				label := strings.TrimSpace(strings.TrimPrefix(name, "location "))
				if tool, ok := tools[label]; ok {
					return tool.String(), nil
				} else {
					return "", fmt.Errorf("unknown location label %q", label)
				}
			}
			return "", fmt.Errorf("unknown variable '$(%s)'", name)
		}
	})

	if err != nil {
		ctx.PropertyErrorf("cmd", "%s", err.Error())
		return
	}

	// tell the sbox command which directory to use as its sandbox root
	buildDir := android.PathForOutput(ctx).String()
	sandboxPath := shared.TempDirForOutDir(buildDir)

	// recall that Sprintf replaces percent sign expressions, whereas dollar signs expressions remain as written,
	// to be replaced later by ninja_strings.go
	sandboxCommand := fmt.Sprintf("$sboxCmd --sandbox-path %s --output-root %s -c %q $allouts", sandboxPath, buildDir, rawCommand)

	ruleParams := blueprint.RuleParams{
		Command:     sandboxCommand,
		CommandDeps: []string{"$sboxCmd"},
	}
	args := []string{"allouts"}
	if g.properties.Depfile {
		ruleParams.Deps = blueprint.DepsGCC
		args = append(args, "depfile")
	}
	g.rule = ctx.Rule(pctx, "generator", ruleParams, args...)

	srcFiles := ctx.ExpandSources(g.properties.Srcs, nil)
	for _, task := range g.tasks(ctx, srcFiles) {
		g.generateSourceFile(ctx, task)
	}
}

func (g *Module) generateSourceFile(ctx android.ModuleContext, task generateTask) {
	desc := "generate"
	if len(task.out) == 0 {
		ctx.ModuleErrorf("must have at least one output file")
		return
	}
	if len(task.out) == 1 {
		desc += " " + task.out[0].Base()
	}

	params := android.BuildParams{
		Rule:            g.rule,
		Description:     "generate",
		Output:          task.out[0],
		ImplicitOutputs: task.out[1:],
		Inputs:          task.in,
		Implicits:       g.deps,
		Args: map[string]string{
			"allouts": strings.Join(task.out.Strings(), " "),
		},
	}
	if g.properties.Depfile {
		depfile := android.GenPathWithExt(ctx, "", task.out[0], task.out[0].Ext()+".d")
		params.Depfile = depfile
	}
	ctx.Build(pctx, params)

	for _, outputFile := range task.out {
		g.outputFiles = append(g.outputFiles, outputFile)
	}
}

func generatorFactory(tasks taskFunc, props ...interface{}) *Module {
	module := &Module{
		tasks: tasks,
	}

	module.AddProperties(props...)
	module.AddProperties(&module.properties)

	return module
}

func NewGenSrcs() *Module {
	properties := &genSrcsProperties{}

	tasks := func(ctx android.ModuleContext, srcFiles android.Paths) []generateTask {
		tasks := make([]generateTask, 0, len(srcFiles))
		for _, in := range srcFiles {
			tasks = append(tasks, generateTask{
				in:  android.Paths{in},
				out: android.WritablePaths{android.GenPathWithExt(ctx, "", in, properties.Output_extension)},
			})
		}
		return tasks
	}

	return generatorFactory(tasks, properties)
}

func GenSrcsFactory() android.Module {
	m := NewGenSrcs()
	android.InitAndroidModule(m)
	return m
}

type genSrcsProperties struct {
	// extension that will be substituted for each output file
	Output_extension string
}

func NewGenRule() *Module {
	properties := &genRuleProperties{}

	tasks := func(ctx android.ModuleContext, srcFiles android.Paths) []generateTask {
		outs := make(android.WritablePaths, len(properties.Out))
		for i, out := range properties.Out {
			outs[i] = android.PathForModuleGen(ctx, out)
		}
		return []generateTask{
			{
				in:  srcFiles,
				out: outs,
			},
		}
	}

	return generatorFactory(tasks, properties)
}

func GenRuleFactory() android.Module {
	m := NewGenRule()
	android.InitAndroidModule(m)
	return m
}

type genRuleProperties struct {
	// names of the output files that will be generated
	Out []string
}
