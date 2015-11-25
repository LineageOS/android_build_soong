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
	"android/soong/common"
	"fmt"
	"runtime"
	"strconv"

	"path/filepath"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
)

const (
	objectExtension        = ".o"
	staticLibraryExtension = ".a"
)

var (
	pctx = blueprint.NewPackageContext("android/soong/cc")

	cc = pctx.StaticRule("cc",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "$relPwd $ccCmd -c $cFlags -MD -MF ${out}.d -o $out $in",
			CommandDeps: []string{"$ccCmd"},
			Description: "cc $out",
		},
		"ccCmd", "cFlags")

	ld = pctx.StaticRule("ld",
		blueprint.RuleParams{
			Command: "$ldCmd ${ldDirFlags} ${crtBegin} @${out}.rsp " +
				"${libFlags} ${crtEnd} -o ${out} ${ldFlags}",
			CommandDeps:    []string{"$ldCmd"},
			Description:    "ld $out",
			Rspfile:        "${out}.rsp",
			RspfileContent: "${in}",
		},
		"ldCmd", "ldDirFlags", "crtBegin", "libFlags", "crtEnd", "ldFlags")

	partialLd = pctx.StaticRule("partialLd",
		blueprint.RuleParams{
			Command:     "$ldCmd -nostdlib -Wl,-r ${in} -o ${out} ${ldFlags}",
			CommandDeps: []string{"$ldCmd"},
			Description: "partialLd $out",
		},
		"ldCmd", "ldFlags")

	ar = pctx.StaticRule("ar",
		blueprint.RuleParams{
			Command:        "rm -f ${out} && $arCmd $arFlags $out @${out}.rsp",
			CommandDeps:    []string{"$arCmd"},
			Description:    "ar $out",
			Rspfile:        "${out}.rsp",
			RspfileContent: "${in}",
		},
		"arCmd", "arFlags")

	darwinAr = pctx.StaticRule("darwinAr",
		blueprint.RuleParams{
			Command:     "rm -f ${out} && $arCmd $arFlags $out $in",
			CommandDeps: []string{"$arCmd"},
			Description: "ar $out",
		},
		"arCmd", "arFlags")

	darwinAppendAr = pctx.StaticRule("darwinAppendAr",
		blueprint.RuleParams{
			Command:     "cp -f ${inAr} ${out}.tmp && $arCmd $arFlags ${out}.tmp $in && mv ${out}.tmp ${out}",
			CommandDeps: []string{"$arCmd"},
			Description: "ar $out",
		},
		"arCmd", "arFlags", "inAr")

	prefixSymbols = pctx.StaticRule("prefixSymbols",
		blueprint.RuleParams{
			Command:     "$objcopyCmd --prefix-symbols=${prefix} ${in} ${out}",
			CommandDeps: []string{"$objcopyCmd"},
			Description: "prefixSymbols $out",
		},
		"objcopyCmd", "prefix")

	copyGccLibPath = pctx.StaticVariable("copyGccLibPath", "${SrcDir}/build/soong/copygcclib.sh")

	copyGccLib = pctx.StaticRule("copyGccLib",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "$copyGccLibPath $out $ccCmd $cFlags -print-file-name=${libName}",
			CommandDeps: []string{"$copyGccLibPath", "$ccCmd"},
			Description: "copy gcc $out",
		},
		"ccCmd", "cFlags", "libName")
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
	asFlags     string
	cFlags      string
	conlyFlags  string
	cppFlags    string
	ldFlags     string
	yaccFlags   string
	nocrt       bool
	toolchain   Toolchain
	clang       bool
}

// Generate rules for compiling multiple .c, .cpp, or .S files to individual .o files
func TransformSourceToObj(ctx common.AndroidModuleContext, subdir string, srcFiles []string,
	flags builderFlags, deps []string) (objFiles []string) {

	srcRoot := ctx.AConfig().SrcDir()
	intermediatesRoot := ctx.AConfig().IntermediatesDir()

	objFiles = make([]string, len(srcFiles))
	objDir := common.ModuleObjDir(ctx)
	if subdir != "" {
		objDir = filepath.Join(objDir, subdir)
	}

	cflags := flags.globalFlags + " " + flags.cFlags + " " + flags.conlyFlags
	cppflags := flags.globalFlags + " " + flags.cFlags + " " + flags.cppFlags
	asflags := flags.globalFlags + " " + flags.asFlags

	for i, srcFile := range srcFiles {
		var objFile string
		if strings.HasPrefix(srcFile, intermediatesRoot) {
			objFile = strings.TrimPrefix(srcFile, intermediatesRoot)
			objFile = filepath.Join(objDir, "gen", objFile)
		} else if strings.HasPrefix(srcFile, srcRoot) {
			srcFile, _ = filepath.Rel(srcRoot, srcFile)
			objFile = filepath.Join(objDir, srcFile)
		} else if srcRoot == "." && srcFile[0] != '/' {
			objFile = filepath.Join(objDir, srcFile)
		} else {
			ctx.ModuleErrorf("source file %q is not in source directory %q", srcFile, srcRoot)
			continue
		}

		objFile = pathtools.ReplaceExtension(objFile, "o")

		objFiles[i] = objFile

		var moduleCflags string
		var ccCmd string

		switch filepath.Ext(srcFile) {
		case ".S", ".s":
			ccCmd = "gcc"
			moduleCflags = asflags
		case ".c":
			ccCmd = "gcc"
			moduleCflags = cflags
		case ".cpp", ".cc":
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

			ccCmd = "${clangPath}" + ccCmd
		} else {
			ccCmd = gccCmd(flags.toolchain, ccCmd)
		}

		ctx.Build(pctx, blueprint.BuildParams{
			Rule:      cc,
			Outputs:   []string{objFile},
			Inputs:    []string{srcFile},
			Implicits: deps,
			Args: map[string]string{
				"cFlags": moduleCflags,
				"ccCmd":  ccCmd,
			},
		})
	}

	return objFiles
}

// Generate a rule for compiling multiple .o files to a static library (.a)
func TransformObjToStaticLib(ctx common.AndroidModuleContext, objFiles []string,
	flags builderFlags, outputFile string) {

	arCmd := gccCmd(flags.toolchain, "ar")
	arFlags := "crsPD"

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    ar,
		Outputs: []string{outputFile},
		Inputs:  objFiles,
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
func TransformDarwinObjToStaticLib(ctx common.AndroidModuleContext, objFiles []string,
	flags builderFlags, outputFile string) {

	arCmd := "ar"
	arFlags := "cqs"

	// ARG_MAX on darwin is 262144, use half that to be safe
	objFilesLists, err := splitListForSize(objFiles, 131072)
	if err != nil {
		ctx.ModuleErrorf("%s", err.Error())
	}

	var in, out string
	for i, l := range objFilesLists {
		in = out
		out = outputFile
		if i != len(objFilesLists)-1 {
			out += "." + strconv.Itoa(i)
		}

		if in == "" {
			ctx.Build(pctx, blueprint.BuildParams{
				Rule:    darwinAr,
				Outputs: []string{out},
				Inputs:  l,
				Args: map[string]string{
					"arFlags": arFlags,
					"arCmd":   arCmd,
				},
			})
		} else {
			ctx.Build(pctx, blueprint.BuildParams{
				Rule:      darwinAppendAr,
				Outputs:   []string{out},
				Inputs:    l,
				Implicits: []string{in},
				Args: map[string]string{
					"arFlags": arFlags,
					"arCmd":   arCmd,
					"inAr":    in,
				},
			})
		}
	}
}

// Generate a rule for compiling multiple .o files, plus static libraries, whole static libraries,
// and shared libraires, to a shared library (.so) or dynamic executable
func TransformObjToDynamicBinary(ctx common.AndroidModuleContext,
	objFiles, sharedLibs, staticLibs, lateStaticLibs, wholeStaticLibs, deps []string,
	crtBegin, crtEnd string, groupLate bool, flags builderFlags, outputFile string) {

	var ldCmd string
	if flags.clang {
		ldCmd = "${clangPath}clang++"
	} else {
		ldCmd = gccCmd(flags.toolchain, "g++")
	}

	var ldDirs []string
	var libFlagsList []string

	if len(wholeStaticLibs) > 0 {
		if ctx.Host() && ctx.Darwin() {
			libFlagsList = append(libFlagsList, common.JoinWithPrefix(wholeStaticLibs, "-force_load "))
		} else {
			libFlagsList = append(libFlagsList, "-Wl,--whole-archive ")
			libFlagsList = append(libFlagsList, wholeStaticLibs...)
			libFlagsList = append(libFlagsList, "-Wl,--no-whole-archive ")
		}
	}

	libFlagsList = append(libFlagsList, staticLibs...)

	if groupLate && len(lateStaticLibs) > 0 {
		libFlagsList = append(libFlagsList, "-Wl,--start-group")
	}
	libFlagsList = append(libFlagsList, lateStaticLibs...)
	if groupLate && len(lateStaticLibs) > 0 {
		libFlagsList = append(libFlagsList, "-Wl,--end-group")
	}

	for _, lib := range sharedLibs {
		dir, file := filepath.Split(lib)
		if !strings.HasPrefix(file, "lib") {
			panic("shared library " + lib + " does not start with lib")
		}
		if !strings.HasSuffix(file, flags.toolchain.ShlibSuffix()) {
			panic("shared library " + lib + " does not end with " + flags.toolchain.ShlibSuffix())
		}
		libFlagsList = append(libFlagsList,
			"-l"+strings.TrimSuffix(strings.TrimPrefix(file, "lib"), flags.toolchain.ShlibSuffix()))
		ldDirs = append(ldDirs, dir)
	}

	deps = append(deps, sharedLibs...)
	deps = append(deps, staticLibs...)
	deps = append(deps, lateStaticLibs...)
	deps = append(deps, wholeStaticLibs...)
	if crtBegin != "" {
		deps = append(deps, crtBegin, crtEnd)
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      ld,
		Outputs:   []string{outputFile},
		Inputs:    objFiles,
		Implicits: deps,
		Args: map[string]string{
			"ldCmd":      ldCmd,
			"ldDirFlags": ldDirsToFlags(ldDirs),
			"crtBegin":   crtBegin,
			"libFlags":   strings.Join(libFlagsList, " "),
			"ldFlags":    flags.ldFlags,
			"crtEnd":     crtEnd,
		},
	})
}

// Generate a rule for compiling multiple .o files to a .o using ld partial linking
func TransformObjsToObj(ctx common.AndroidModuleContext, objFiles []string,
	flags builderFlags, outputFile string) {

	var ldCmd string
	if flags.clang {
		ldCmd = "${clangPath}clang++"
	} else {
		ldCmd = gccCmd(flags.toolchain, "g++")
	}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    partialLd,
		Outputs: []string{outputFile},
		Inputs:  objFiles,
		Args: map[string]string{
			"ldCmd": ldCmd,
			"ldFlags": flags.ldFlags,
		},
	})
}

// Generate a rule for runing objcopy --prefix-symbols on a binary
func TransformBinaryPrefixSymbols(ctx common.AndroidModuleContext, prefix string, inputFile string,
	flags builderFlags, outputFile string) {

	objcopyCmd := gccCmd(flags.toolchain, "objcopy")

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    prefixSymbols,
		Outputs: []string{outputFile},
		Inputs:  []string{inputFile},
		Args: map[string]string{
			"objcopyCmd": objcopyCmd,
			"prefix":     prefix,
		},
	})
}

func CopyGccLib(ctx common.AndroidModuleContext, libName string,
	flags builderFlags, outputFile string) {

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    copyGccLib,
		Outputs: []string{outputFile},
		Args: map[string]string{
			"ccCmd":   gccCmd(flags.toolchain, "gcc"),
			"cFlags":  flags.globalFlags,
			"libName": libName,
		},
	})
}

func gccCmd(toolchain Toolchain, cmd string) string {
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
