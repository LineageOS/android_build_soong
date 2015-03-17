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

	"blueprint"
	"blueprint/pathtools"
	"path/filepath"
	"strings"
)

const (
	sharedLibraryExtension = ".so"
	staticLibraryExtension = ".a"
)

var (
	pctx = blueprint.NewPackageContext("android/soong/cc")

	cc = pctx.StaticRule("cc",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "$ccCmd $incFlags -c $cFlags -MD -MF ${out}.d -o $out $in",
			Description: "cc $out",
		},
		"ccCmd", "incFlags", "cFlags")

	ld = pctx.StaticRule("ld",
		blueprint.RuleParams{
			Command: "$ldCmd ${ldDirFlags} ${crtBegin} ${in} " +
				"${libFlags} ${crtEnd} -o ${out} ${ldFlags} ${ldLibs}",
			Description: "ld $out",
		},
		"ldCmd", "ldDirFlags", "crtBegin", "libFlags", "crtEnd", "ldFlags", "ldLibs")

	partialLd = pctx.StaticRule("partialLd",
		blueprint.RuleParams{
			Command:     "$ldCmd -r ${in} -o ${out}",
			Description: "partialLd $out",
		},
		"ldCmd")

	ar = pctx.StaticRule("ar",
		blueprint.RuleParams{
			Command:     "rm -f ${out} && $arCmd $arFlags $out $in",
			Description: "ar $out",
		},
		"arCmd", "arFlags")

	copyGccLibPath = pctx.StaticVariable("copyGccLibPath", "${SrcDir}/build/soong/copygcclib.sh")

	copyGccLib = pctx.StaticRule("copyGccLib",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "$copyGccLibPath $out $ccCmd $cFlags -print-file-name=${libName}",
			Description: "copy gcc $out",
		},
		"ccCmd", "cFlags", "libName")
)

type builderFlags struct {
	globalFlags string
	asFlags     string
	cFlags      string
	conlyFlags  string
	cppFlags    string
	ldFlags     string
	ldLibs      string
	incFlags    string
	nocrt       bool
	toolchain   toolchain
	clang       bool
}

// Generate rules for compiling multiple .c, .cpp, or .S files to individual .o files
func TransformSourceToObj(ctx common.AndroidModuleContext, subdir string, srcFiles []string,
	flags builderFlags) (objFiles []string) {

	objFiles = make([]string, len(srcFiles))
	objDir := common.ModuleObjDir(ctx)
	if subdir != "" {
		objDir = filepath.Join(objDir, subdir)
	}

	cflags := flags.globalFlags + " " + flags.cFlags + " " + flags.conlyFlags
	cppflags := flags.globalFlags + " " + flags.cFlags + " " + flags.cppFlags
	asflags := flags.globalFlags + " " + flags.asFlags

	for i, srcFile := range srcFiles {
		objFile := strings.TrimPrefix(srcFile, common.ModuleSrcDir(ctx))
		objFile = filepath.Join(objDir, objFile)
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
			Implicits: []string{ccCmd},
			Args: map[string]string{
				"cFlags":   moduleCflags,
				"incFlags": flags.incFlags,
				"ccCmd":    ccCmd,
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
		Rule:      ar,
		Outputs:   []string{outputFile},
		Inputs:    objFiles,
		Implicits: []string{arCmd},
		Args: map[string]string{
			"arFlags": arFlags,
			"arCmd":   arCmd,
		},
	})
}

// Generate a rule for compiling multiple .o files, plus static libraries, whole static libraries,
// and shared libraires, to a shared library (.so) or dynamic executable
func TransformObjToDynamicBinary(ctx common.AndroidModuleContext,
	objFiles, sharedLibs, staticLibs, lateStaticLibs, wholeStaticLibs []string,
	crtBegin, crtEnd string, flags builderFlags, outputFile string) {

	var ldCmd string
	if flags.clang {
		ldCmd = "${clangPath}clang++"
	} else {
		ldCmd = gccCmd(flags.toolchain, "g++")
	}

	var ldDirs []string
	var libFlagsList []string

	if len(wholeStaticLibs) > 0 {
		libFlagsList = append(libFlagsList, "-Wl,--whole-archive ")
		libFlagsList = append(libFlagsList, wholeStaticLibs...)
		libFlagsList = append(libFlagsList, "-Wl,--no-whole-archive ")
	}

	libFlagsList = append(libFlagsList, staticLibs...)

	for _, lib := range sharedLibs {
		dir, file := filepath.Split(lib)
		if !strings.HasPrefix(file, "lib") {
			panic("shared library " + lib + " does not start with lib")
		}
		if !strings.HasSuffix(file, sharedLibraryExtension) {
			panic("shared library " + lib + " does not end with " + sharedLibraryExtension)
		}
		libFlagsList = append(libFlagsList,
			"-l"+strings.TrimSuffix(strings.TrimPrefix(file, "lib"), sharedLibraryExtension))
		ldDirs = append(ldDirs, dir)
	}

	libFlagsList = append(libFlagsList, lateStaticLibs...)

	deps := []string{ldCmd}
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
			"ldLibs":     flags.ldLibs,
		},
	})
}

// Generate a rule for compiling multiple .o files to a .o using ld partial linking
func TransformObjsToObj(ctx common.AndroidModuleContext, objFiles []string,
	flags builderFlags, outputFile string) {

	ldCmd := gccCmd(flags.toolchain, "ld")

	deps := []string{ldCmd}

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:      partialLd,
		Outputs:   []string{outputFile},
		Inputs:    objFiles,
		Implicits: deps,
		Args: map[string]string{
			"ldCmd": ldCmd,
		},
	})
}

func CopyGccLib(ctx common.AndroidModuleContext, libName string,
	flags builderFlags, outputFile string) {

	ctx.Build(pctx, blueprint.BuildParams{
		Rule:    copyGccLib,
		Outputs: []string{outputFile},
		Implicits: []string{
			"$copyGccLibPath",
			gccCmd(flags.toolchain, "gcc"),
		},
		Args: map[string]string{
			"ccCmd":   gccCmd(flags.toolchain, "gcc"),
			"cFlags":  flags.globalFlags,
			"libName": libName,
		},
	})
}

func gccCmd(toolchain toolchain, cmd string) string {
	return filepath.Join(toolchain.GccRoot(), "bin", toolchain.GccTriple()+"-"+cmd)
}
