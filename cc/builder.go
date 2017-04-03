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

package cc

// This file generates the final rules for compiling all C/C++.  All properties related to
// compiling should have been translated into builderFlags or another argument to the Transform*
// functions.

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/google/blueprint"

	"android/soong/android"
	"android/soong/cc/config"
)

const (
	objectExtension        = ".o"
	staticLibraryExtension = ".a"
)

var (
	pctx = android.NewPackageContext("android/soong/cc")

	cc = pctx.AndroidGomaStaticRule("cc",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "$relPwd ${config.CcWrapper}$ccCmd -c $cFlags -MD -MF ${out}.d -o $out $in",
			CommandDeps: []string{"$ccCmd"},
			Description: "cc $out",
		},
		"ccCmd", "cFlags")

	ld = pctx.AndroidStaticRule("ld",
		blueprint.RuleParams{
			Command: "$ldCmd ${crtBegin} @${out}.rsp " +
				"${libFlags} ${crtEnd} -o ${out} ${ldFlags}",
			CommandDeps:    []string{"$ldCmd"},
			Description:    "ld $out",
			Rspfile:        "${out}.rsp",
			RspfileContent: "${in}",
		},
		"ldCmd", "crtBegin", "libFlags", "crtEnd", "ldFlags")

	partialLd = pctx.AndroidStaticRule("partialLd",
		blueprint.RuleParams{
			Command:     "$ldCmd -nostdlib -Wl,-r ${in} -o ${out} ${ldFlags}",
			CommandDeps: []string{"$ldCmd"},
			Description: "partialLd $out",
		},
		"ldCmd", "ldFlags")

	ar = pctx.AndroidStaticRule("ar",
		blueprint.RuleParams{
			Command:        "rm -f ${out} && $arCmd $arFlags $out @${out}.rsp",
			CommandDeps:    []string{"$arCmd"},
			Description:    "ar $out",
			Rspfile:        "${out}.rsp",
			RspfileContent: "${in}",
		},
		"arCmd", "arFlags")

	darwinAr = pctx.AndroidStaticRule("darwinAr",
		blueprint.RuleParams{
			Command:     "rm -f ${out} && ${config.MacArPath} $arFlags $out $in",
			CommandDeps: []string{"${config.MacArPath}"},
			Description: "ar $out",
		},
		"arFlags")

	darwinAppendAr = pctx.AndroidStaticRule("darwinAppendAr",
		blueprint.RuleParams{
			Command:     "cp -f ${inAr} ${out}.tmp && ${config.MacArPath} $arFlags ${out}.tmp $in && mv ${out}.tmp ${out}",
			CommandDeps: []string{"${config.MacArPath}", "${inAr}"},
			Description: "ar $out",
		},
		"arFlags", "inAr")

	darwinStrip = pctx.AndroidStaticRule("darwinStrip",
		blueprint.RuleParams{
			Command:     "${config.MacStripPath} -u -r -o $out $in",
			CommandDeps: []string{"${config.MacStripPath}"},
			Description: "strip $out",
		})

	prefixSymbols = pctx.AndroidStaticRule("prefixSymbols",
		blueprint.RuleParams{
			Command:     "$objcopyCmd --prefix-symbols=${prefix} ${in} ${out}",
			CommandDeps: []string{"$objcopyCmd"},
			Description: "prefixSymbols $out",
		},
		"objcopyCmd", "prefix")

	_ = pctx.SourcePathVariable("stripPath", "build/soong/scripts/strip.sh")

	strip = pctx.AndroidStaticRule("strip",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "CROSS_COMPILE=$crossCompile $stripPath ${args} -i ${in} -o ${out} -d ${out}.d",
			CommandDeps: []string{"$stripPath"},
			Description: "strip $out",
		},
		"args", "crossCompile")

	emptyFile = pctx.AndroidStaticRule("emptyFile",
		blueprint.RuleParams{
			Command:     "rm -f $out && touch $out",
			Description: "empty file $out",
		})

	_ = pctx.SourcePathVariable("copyGccLibPath", "build/soong/scripts/copygcclib.sh")

	copyGccLib = pctx.AndroidStaticRule("copyGccLib",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "$copyGccLibPath $out $ccCmd $cFlags -print-file-name=${libName}",
			CommandDeps: []string{"$copyGccLibPath", "$ccCmd"},
			Description: "copy gcc $out",
		},
		"ccCmd", "cFlags", "libName")

	_ = pctx.SourcePathVariable("tocPath", "build/soong/scripts/toc.sh")

	toc = pctx.AndroidStaticRule("toc",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "CROSS_COMPILE=$crossCompile $tocPath -i ${in} -o ${out} -d ${out}.d",
			CommandDeps: []string{"$tocPath"},
			Restat:      true,
		},
		"crossCompile")

	clangTidy = pctx.AndroidStaticRule("clangTidy",
		blueprint.RuleParams{
			Command:     "rm -f $out && ${config.ClangBin}/clang-tidy $tidyFlags $in -- $cFlags && touch $out",
			CommandDeps: []string{"${config.ClangBin}/clang-tidy"},
			Description: "tidy $out",
		},
		"cFlags", "tidyFlags")

	_ = pctx.SourcePathVariable("yasmCmd", "prebuilts/misc/${config.HostPrebuiltTag}/yasm/yasm")

	yasm = pctx.AndroidStaticRule("yasm",
		blueprint.RuleParams{
			Command:     "$yasmCmd $asFlags -o $out $in",
			CommandDeps: []string{"$yasmCmd"},
			Description: "yasm $out",
		},
		"asFlags")
)

func init() {
	// We run gcc/clang with PWD=/proc/self/cwd to remove $TOP from the
	// debug output. That way two builds in two different directories will
	// create the same output.
	if runtime.GOOS != "darwin" {
		pctx.StaticVariable("relPwd", "PWD=/proc/self/cwd")
	} else {
		// Darwin doesn't have /proc
		pctx.StaticVariable("relPwd", "")
	}
}

type builderFlags struct {
	globalFlags string
	arFlags     string
	asFlags     string
	cFlags      string
	conlyFlags  string
	cppFlags    string
	ldFlags     string
	libFlags    string
	yaccFlags   string
	protoFlags  string
	tidyFlags   string
	yasmFlags   string
	aidlFlags   string
	toolchain   config.Toolchain
	clang       bool
	tidy        bool
	coverage    bool

	systemIncludeFlags string

	groupStaticLibs bool

	stripKeepSymbols       bool
	stripKeepMiniDebugInfo bool
	stripAddGnuDebuglink   bool
}

type Objects struct {
	objFiles      android.Paths
	tidyFiles     android.Paths
	coverageFiles android.Paths
}

func (a Objects) Copy() Objects {
	return Objects{
		objFiles:      append(android.Paths{}, a.objFiles...),
		tidyFiles:     append(android.Paths{}, a.tidyFiles...),
		coverageFiles: append(android.Paths{}, a.coverageFiles...),
	}
}

func (a Objects) Append(b Objects) Objects {
	return Objects{
		objFiles:      append(a.objFiles, b.objFiles...),
		tidyFiles:     append(a.tidyFiles, b.tidyFiles...),
		coverageFiles: append(a.coverageFiles, b.coverageFiles...),
	}
}

// Generate rules for compiling multiple .c, .cpp, or .S files to individual .o files
func TransformSourceToObj(ctx android.ModuleContext, subdir string, srcFiles android.Paths,
	flags builderFlags, deps android.Paths) Objects {

	objFiles := make(android.Paths, len(srcFiles))
	var tidyFiles android.Paths
	if flags.tidy && flags.clang {
		tidyFiles = make(android.Paths, 0, len(srcFiles))
	}
	var coverageFiles android.Paths
	if flags.coverage {
		coverageFiles = make(android.Paths, 0, len(srcFiles))
	}

	cflags := strings.Join([]string{
		flags.globalFlags,
		flags.systemIncludeFlags,
		flags.cFlags,
		flags.conlyFlags,
	}, " ")

	cppflags := strings.Join([]string{
		flags.globalFlags,
		flags.systemIncludeFlags,
		flags.cFlags,
		flags.cppFlags,
	}, " ")

	asflags := strings.Join([]string{
		flags.globalFlags,
		flags.systemIncludeFlags,
		flags.asFlags,
	}, " ")

	if flags.clang {
		cflags += " ${config.NoOverrideClangGlobalCflags}"
		cppflags += " ${config.NoOverrideClangGlobalCflags}"
	} else {
		cflags += " ${config.NoOverrideGlobalCflags}"
		cppflags += " ${config.NoOverrideGlobalCflags}"
	}

	for i, srcFile := range srcFiles {
		objFile := android.ObjPathWithExt(ctx, subdir, srcFile, "o")

		objFiles[i] = objFile

		if srcFile.Ext() == ".asm" {
			ctx.ModuleBuild(pctx, android.ModuleBuildParams{
				Rule:      yasm,
				Output:    objFile,
				Input:     srcFile,
				OrderOnly: deps,
				Args: map[string]string{
					"asFlags": flags.yasmFlags,
				},
			})
			continue
		}

		var moduleCflags string
		var ccCmd string
		tidy := flags.tidy && flags.clang
		coverage := flags.coverage

		switch srcFile.Ext() {
		case ".S", ".s":
			ccCmd = "gcc"
			moduleCflags = asflags
			tidy = false
			coverage = false
		case ".c":
			ccCmd = "gcc"
			moduleCflags = cflags
		case ".cpp", ".cc", ".mm":
			ccCmd = "g++"
			moduleCflags = cppflags
		default:
			ctx.ModuleErrorf("File %s has unknown extension", srcFile)
			continue
		}

		if flags.clang {
			switch ccCmd {
			case "gcc":
				ccCmd = "clang"
			case "g++":
				ccCmd = "clang++"
			default:
				panic("unrecoginzied ccCmd")
			}

			ccCmd = "${config.ClangBin}/" + ccCmd
		} else {
			ccCmd = gccCmd(flags.toolchain, ccCmd)
		}

		var implicitOutputs android.WritablePaths
		if coverage {
			gcnoFile := android.ObjPathWithExt(ctx, subdir, srcFile, "gcno")
			implicitOutputs = append(implicitOutputs, gcnoFile)
			coverageFiles = append(coverageFiles, gcnoFile)
		}

		ctx.ModuleBuild(pctx, android.ModuleBuildParams{
			Rule:            cc,
			Output:          objFile,
			ImplicitOutputs: implicitOutputs,
			Input:           srcFile,
			OrderOnly:       deps,
			Args: map[string]string{
				"cFlags": moduleCflags,
				"ccCmd":  ccCmd,
			},
		})

		if tidy {
			tidyFile := android.ObjPathWithExt(ctx, subdir, srcFile, "tidy")
			tidyFiles = append(tidyFiles, tidyFile)

			ctx.ModuleBuild(pctx, android.ModuleBuildParams{
				Rule:   clangTidy,
				Output: tidyFile,
				Input:  srcFile,
				// We must depend on objFile, since clang-tidy doesn't
				// support exporting dependencies.
				Implicit: objFile,
				Args: map[string]string{
					"cFlags":    moduleCflags,
					"tidyFlags": flags.tidyFlags,
				},
			})
		}

	}

	return Objects{
		objFiles:      objFiles,
		tidyFiles:     tidyFiles,
		coverageFiles: coverageFiles,
	}
}

// Generate a rule for compiling multiple .o files to a static library (.a)
func TransformObjToStaticLib(ctx android.ModuleContext, objFiles android.Paths,
	flags builderFlags, outputFile android.ModuleOutPath, deps android.Paths) {

	if ctx.Darwin() {
		transformDarwinObjToStaticLib(ctx, objFiles, flags, outputFile, deps)
		return
	}

	arCmd := gccCmd(flags.toolchain, "ar")
	arFlags := "crsPD"
	if flags.arFlags != "" {
		arFlags += " " + flags.arFlags
	}

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:      ar,
		Output:    outputFile,
		Inputs:    objFiles,
		Implicits: deps,
		Args: map[string]string{
			"arFlags": arFlags,
			"arCmd":   arCmd,
		},
	})
}

// Generate a rule for compiling multiple .o files to a static library (.a) on
// darwin.  The darwin ar tool doesn't support @file for list files, and has a
// very small command line length limit, so we have to split the ar into multiple
// steps, each appending to the previous one.
func transformDarwinObjToStaticLib(ctx android.ModuleContext, objFiles android.Paths,
	flags builderFlags, outputPath android.ModuleOutPath, deps android.Paths) {

	arFlags := "cqs"

	if len(objFiles) == 0 {
		dummy := android.PathForModuleOut(ctx, "dummy"+objectExtension)
		dummyAr := android.PathForModuleOut(ctx, "dummy"+staticLibraryExtension)

		ctx.ModuleBuild(pctx, android.ModuleBuildParams{
			Rule:      emptyFile,
			Output:    dummy,
			Implicits: deps,
		})

		ctx.ModuleBuild(pctx, android.ModuleBuildParams{
			Rule:   darwinAr,
			Output: dummyAr,
			Input:  dummy,
			Args: map[string]string{
				"arFlags": arFlags,
			},
		})

		ctx.ModuleBuild(pctx, android.ModuleBuildParams{
			Rule:   darwinAppendAr,
			Output: outputPath,
			Input:  dummy,
			Args: map[string]string{
				"arFlags": "d",
				"inAr":    dummyAr.String(),
			},
		})

		return
	}

	// ARG_MAX on darwin is 262144, use half that to be safe
	objFilesLists, err := splitListForSize(objFiles.Strings(), 131072)
	if err != nil {
		ctx.ModuleErrorf("%s", err.Error())
	}

	outputFile := outputPath.String()

	var in, out string
	for i, l := range objFilesLists {
		in = out
		out = outputFile
		if i != len(objFilesLists)-1 {
			out += "." + strconv.Itoa(i)
		}

		if in == "" {
			ctx.Build(pctx, blueprint.BuildParams{
				Rule:      darwinAr,
				Outputs:   []string{out},
				Inputs:    l,
				Implicits: deps.Strings(),
				Args: map[string]string{
					"arFlags": arFlags,
				},
			})
		} else {
			ctx.Build(pctx, blueprint.BuildParams{
				Rule:    darwinAppendAr,
				Outputs: []string{out},
				Inputs:  l,
				Args: map[string]string{
					"arFlags": arFlags,
					"inAr":    in,
				},
			})
		}
	}
}

// Generate a rule for compiling multiple .o files, plus static libraries, whole static libraries,
// and shared libraires, to a shared library (.so) or dynamic executable
func TransformObjToDynamicBinary(ctx android.ModuleContext,
	objFiles, sharedLibs, staticLibs, lateStaticLibs, wholeStaticLibs, deps android.Paths,
	crtBegin, crtEnd android.OptionalPath, groupLate bool, flags builderFlags, outputFile android.WritablePath) {

	var ldCmd string
	if flags.clang {
		ldCmd = "${config.ClangBin}/clang++"
	} else {
		ldCmd = gccCmd(flags.toolchain, "g++")
	}

	var libFlagsList []string

	if len(flags.libFlags) > 0 {
		libFlagsList = append(libFlagsList, flags.libFlags)
	}

	if len(wholeStaticLibs) > 0 {
		if ctx.Host() && ctx.Darwin() {
			libFlagsList = append(libFlagsList, android.JoinWithPrefix(wholeStaticLibs.Strings(), "-force_load "))
		} else {
			libFlagsList = append(libFlagsList, "-Wl,--whole-archive ")
			libFlagsList = append(libFlagsList, wholeStaticLibs.Strings()...)
			libFlagsList = append(libFlagsList, "-Wl,--no-whole-archive ")
		}
	}

	if flags.groupStaticLibs && !ctx.Darwin() && len(staticLibs) > 0 {
		libFlagsList = append(libFlagsList, "-Wl,--start-group")
	}
	libFlagsList = append(libFlagsList, staticLibs.Strings()...)
	if flags.groupStaticLibs && !ctx.Darwin() && len(staticLibs) > 0 {
		libFlagsList = append(libFlagsList, "-Wl,--end-group")
	}

	if groupLate && !ctx.Darwin() && len(lateStaticLibs) > 0 {
		libFlagsList = append(libFlagsList, "-Wl,--start-group")
	}
	libFlagsList = append(libFlagsList, lateStaticLibs.Strings()...)
	if groupLate && !ctx.Darwin() && len(lateStaticLibs) > 0 {
		libFlagsList = append(libFlagsList, "-Wl,--end-group")
	}

	for _, lib := range sharedLibs {
		libFlagsList = append(libFlagsList, lib.String())
	}

	deps = append(deps, staticLibs...)
	deps = append(deps, lateStaticLibs...)
	deps = append(deps, wholeStaticLibs...)
	if crtBegin.Valid() {
		deps = append(deps, crtBegin.Path(), crtEnd.Path())
	}

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:      ld,
		Output:    outputFile,
		Inputs:    objFiles,
		Implicits: deps,
		Args: map[string]string{
			"ldCmd":    ldCmd,
			"crtBegin": crtBegin.String(),
			"libFlags": strings.Join(libFlagsList, " "),
			"ldFlags":  flags.ldFlags,
			"crtEnd":   crtEnd.String(),
		},
	})
}

// Generate a rule for extract a table of contents from a shared library (.so)
func TransformSharedObjectToToc(ctx android.ModuleContext, inputFile android.WritablePath,
	outputFile android.WritablePath, flags builderFlags) {

	crossCompile := gccCmd(flags.toolchain, "")

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:   toc,
		Output: outputFile,
		Input:  inputFile,
		Args: map[string]string{
			"crossCompile": crossCompile,
		},
	})
}

// Generate a rule for compiling multiple .o files to a .o using ld partial linking
func TransformObjsToObj(ctx android.ModuleContext, objFiles android.Paths,
	flags builderFlags, outputFile android.WritablePath) {

	var ldCmd string
	if flags.clang {
		ldCmd = "${config.ClangBin}/clang++"
	} else {
		ldCmd = gccCmd(flags.toolchain, "g++")
	}

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:   partialLd,
		Output: outputFile,
		Inputs: objFiles,
		Args: map[string]string{
			"ldCmd":   ldCmd,
			"ldFlags": flags.ldFlags,
		},
	})
}

// Generate a rule for runing objcopy --prefix-symbols on a binary
func TransformBinaryPrefixSymbols(ctx android.ModuleContext, prefix string, inputFile android.Path,
	flags builderFlags, outputFile android.WritablePath) {

	objcopyCmd := gccCmd(flags.toolchain, "objcopy")

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:   prefixSymbols,
		Output: outputFile,
		Input:  inputFile,
		Args: map[string]string{
			"objcopyCmd": objcopyCmd,
			"prefix":     prefix,
		},
	})
}

func TransformStrip(ctx android.ModuleContext, inputFile android.Path,
	outputFile android.WritablePath, flags builderFlags) {

	crossCompile := gccCmd(flags.toolchain, "")
	args := ""
	if flags.stripAddGnuDebuglink {
		args += " --add-gnu-debuglink"
	}
	if flags.stripKeepMiniDebugInfo {
		args += " --keep-mini-debug-info"
	}
	if flags.stripKeepSymbols {
		args += " --keep-symbols"
	}

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:   strip,
		Output: outputFile,
		Input:  inputFile,
		Args: map[string]string{
			"crossCompile": crossCompile,
			"args":         args,
		},
	})
}

func TransformDarwinStrip(ctx android.ModuleContext, inputFile android.Path,
	outputFile android.WritablePath) {

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:   darwinStrip,
		Output: outputFile,
		Input:  inputFile,
	})
}

func TransformCoverageFilesToLib(ctx android.ModuleContext,
	inputs Objects, flags builderFlags, baseName string) android.OptionalPath {

	if len(inputs.coverageFiles) > 0 {
		outputFile := android.PathForModuleOut(ctx, baseName+".gcnodir")

		TransformObjToStaticLib(ctx, inputs.coverageFiles, flags, outputFile, nil)

		return android.OptionalPathForPath(outputFile)
	}

	return android.OptionalPath{}
}

func CopyGccLib(ctx android.ModuleContext, libName string,
	flags builderFlags, outputFile android.WritablePath) {

	ctx.ModuleBuild(pctx, android.ModuleBuildParams{
		Rule:   copyGccLib,
		Output: outputFile,
		Args: map[string]string{
			"ccCmd":   gccCmd(flags.toolchain, "gcc"),
			"cFlags":  flags.globalFlags,
			"libName": libName,
		},
	})
}

func gccCmd(toolchain config.Toolchain, cmd string) string {
	return filepath.Join(toolchain.GccRoot(), "bin", toolchain.GccTriple()+"-"+cmd)
}

func splitListForSize(list []string, limit int) (lists [][]string, err error) {
	var i int

	start := 0
	bytes := 0
	for i = range list {
		l := len(list[i])
		if l > limit {
			return nil, fmt.Errorf("list element greater than size limit (%d)", limit)
		}
		if bytes+l > limit {
			lists = append(lists, list[start:i])
			start = i
			bytes = 0
		}
		bytes += l + 1 // count a space between each list element
	}

	lists = append(lists, list[start:])

	totalLen := 0
	for _, l := range lists {
		totalLen += len(l)
	}
	if totalLen != len(list) {
		panic(fmt.Errorf("Failed breaking up list, %d != %d", len(list), totalLen))
	}
	return lists, nil
}
