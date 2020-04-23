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
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"

	"android/soong/android"
	"android/soong/cc/config"
	"android/soong/remoteexec"
)

const (
	objectExtension        = ".o"
	staticLibraryExtension = ".a"
)

var (
	abiCheckAllowFlags = []string{
		"-allow-unreferenced-changes",
		"-allow-unreferenced-elf-symbol-changes",
	}
)

var (
	pctx = android.NewPackageContext("android/soong/cc")

	cc = pctx.AndroidRemoteStaticRule("cc", android.RemoteRuleSupports{Goma: true, RBE: true},
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "$relPwd ${config.CcWrapper}$ccCmd -c $cFlags -MD -MF ${out}.d -o $out $in",
			CommandDeps: []string{"$ccCmd"},
		},
		"ccCmd", "cFlags")

	ccNoDeps = pctx.AndroidStaticRule("ccNoDeps",
		blueprint.RuleParams{
			Command:     "$relPwd $ccCmd -c $cFlags -o $out $in",
			CommandDeps: []string{"$ccCmd"},
		},
		"ccCmd", "cFlags")

	ld, ldRE = remoteexec.StaticRules(pctx, "ld",
		blueprint.RuleParams{
			Command: "$ldCmd ${crtBegin} @${out}.rsp " +
				"${libFlags} ${crtEnd} -o ${out} ${ldFlags} ${extraLibFlags}",
			CommandDeps:    []string{"$ldCmd"},
			Rspfile:        "${out}.rsp",
			RspfileContent: "${in}",
			// clang -Wl,--out-implib doesn't update its output file if it hasn't changed.
			Restat: true,
		},
		&remoteexec.REParams{Labels: map[string]string{"type": "link", "tool": "clang"},
			ExecStrategy:    "${config.RECXXLinksExecStrategy}",
			Inputs:          []string{"${out}.rsp"},
			RSPFile:         "${out}.rsp",
			OutputFiles:     []string{"${out}"},
			ToolchainInputs: []string{"$ldCmd"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.RECXXLinksPool}"},
		}, []string{"ldCmd", "crtBegin", "libFlags", "crtEnd", "ldFlags", "extraLibFlags"}, nil)

	partialLd, partialLdRE = remoteexec.StaticRules(pctx, "partialLd",
		blueprint.RuleParams{
			// Without -no-pie, clang 7.0 adds -pie to link Android files,
			// but -r and -pie cannot be used together.
			Command:     "$ldCmd -fuse-ld=lld -nostdlib -no-pie -Wl,-r ${in} -o ${out} ${ldFlags}",
			CommandDeps: []string{"$ldCmd"},
		}, &remoteexec.REParams{
			Labels:       map[string]string{"type": "link", "tool": "clang"},
			ExecStrategy: "${config.RECXXLinksExecStrategy}", Inputs: []string{"$inCommaList"},
			OutputFiles:     []string{"${out}"},
			ToolchainInputs: []string{"$ldCmd"},
			Platform:        map[string]string{remoteexec.PoolKey: "${config.RECXXLinksPool}"},
		}, []string{"ldCmd", "ldFlags"}, []string{"inCommaList"})

	ar = pctx.AndroidStaticRule("ar",
		blueprint.RuleParams{
			Command:        "rm -f ${out} && $arCmd $arFlags $out @${out}.rsp",
			CommandDeps:    []string{"$arCmd"},
			Rspfile:        "${out}.rsp",
			RspfileContent: "${in}",
		},
		"arCmd", "arFlags")

	darwinStrip = pctx.AndroidStaticRule("darwinStrip",
		blueprint.RuleParams{
			Command:     "${config.MacStripPath} -u -r -o $out $in",
			CommandDeps: []string{"${config.MacStripPath}"},
		})

	prefixSymbols = pctx.AndroidStaticRule("prefixSymbols",
		blueprint.RuleParams{
			Command:     "$objcopyCmd --prefix-symbols=${prefix} ${in} ${out}",
			CommandDeps: []string{"$objcopyCmd"},
		},
		"objcopyCmd", "prefix")

	_ = pctx.SourcePathVariable("stripPath", "build/soong/scripts/strip.sh")
	_ = pctx.SourcePathVariable("xzCmd", "prebuilts/build-tools/${config.HostPrebuiltTag}/bin/xz")

	// b/132822437: objcopy uses a file descriptor per .o file when called on .a files, which runs the system out of
	// file descriptors on darwin.  Limit concurrent calls to 5 on darwin.
	darwinStripPool = func() blueprint.Pool {
		if runtime.GOOS == "darwin" {
			return pctx.StaticPool("darwinStripPool", blueprint.PoolParams{
				Depth: 5,
			})
		} else {
			return nil
		}
	}()

	strip = pctx.AndroidStaticRule("strip",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "CROSS_COMPILE=$crossCompile XZ=$xzCmd CLANG_BIN=${config.ClangBin} $stripPath ${args} -i ${in} -o ${out} -d ${out}.d",
			CommandDeps: []string{"$stripPath", "$xzCmd"},
			Pool:        darwinStripPool,
		},
		"args", "crossCompile")

	_ = pctx.SourcePathVariable("archiveRepackPath", "build/soong/scripts/archive_repack.sh")

	archiveRepack = pctx.AndroidStaticRule("archiveRepack",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "CLANG_BIN=${config.ClangBin} $archiveRepackPath -i ${in} -o ${out} -d ${out}.d ${objects}",
			CommandDeps: []string{"$archiveRepackPath"},
		},
		"objects")

	emptyFile = pctx.AndroidStaticRule("emptyFile",
		blueprint.RuleParams{
			Command: "rm -f $out && touch $out",
		})

	_ = pctx.SourcePathVariable("tocPath", "build/soong/scripts/toc.sh")

	toc = pctx.AndroidStaticRule("toc",
		blueprint.RuleParams{
			Depfile:     "${out}.d",
			Deps:        blueprint.DepsGCC,
			Command:     "CROSS_COMPILE=$crossCompile $tocPath $format -i ${in} -o ${out} -d ${out}.d",
			CommandDeps: []string{"$tocPath"},
			Restat:      true,
		},
		"crossCompile", "format")

	clangTidy = pctx.AndroidStaticRule("clangTidy",
		blueprint.RuleParams{
			Command:     "rm -f $out && ${config.ClangBin}/clang-tidy $tidyFlags $in -- $cFlags && touch $out",
			CommandDeps: []string{"${config.ClangBin}/clang-tidy"},
		},
		"cFlags", "tidyFlags")

	_ = pctx.SourcePathVariable("yasmCmd", "prebuilts/misc/${config.HostPrebuiltTag}/yasm/yasm")

	yasm = pctx.AndroidStaticRule("yasm",
		blueprint.RuleParams{
			Command:     "$yasmCmd $asFlags -o $out $in && $yasmCmd $asFlags -M $in >$out.d",
			CommandDeps: []string{"$yasmCmd"},
			Depfile:     "$out.d",
			Deps:        blueprint.DepsGCC,
		},
		"asFlags")

	windres = pctx.AndroidStaticRule("windres",
		blueprint.RuleParams{
			Command:     "$windresCmd $flags -I$$(dirname $in) -i $in -o $out --preprocessor \"${config.ClangBin}/clang -E -xc-header -DRC_INVOKED\"",
			CommandDeps: []string{"$windresCmd"},
		},
		"windresCmd", "flags")

	_ = pctx.SourcePathVariable("sAbiDumper", "prebuilts/clang-tools/${config.HostPrebuiltTag}/bin/header-abi-dumper")

	// -w has been added since header-abi-dumper does not need to produce any sort of diagnostic information.
	sAbiDump = pctx.AndroidStaticRule("sAbiDump",
		blueprint.RuleParams{
			Command:     "rm -f $out && $sAbiDumper -o ${out} $in $exportDirs -- $cFlags -w -isystem prebuilts/clang-tools/${config.HostPrebuiltTag}/clang-headers",
			CommandDeps: []string{"$sAbiDumper"},
		},
		"cFlags", "exportDirs")

	_ = pctx.SourcePathVariable("sAbiLinker", "prebuilts/clang-tools/${config.HostPrebuiltTag}/bin/header-abi-linker")

	sAbiLink = pctx.AndroidStaticRule("sAbiLink",
		blueprint.RuleParams{
			Command:        "$sAbiLinker -o ${out} $symbolFilter -arch $arch  $exportedHeaderFlags @${out}.rsp ",
			CommandDeps:    []string{"$sAbiLinker"},
			Rspfile:        "${out}.rsp",
			RspfileContent: "${in}",
		},
		"symbolFilter", "arch", "exportedHeaderFlags")

	_ = pctx.SourcePathVariable("sAbiDiffer", "prebuilts/clang-tools/${config.HostPrebuiltTag}/bin/header-abi-diff")

	sAbiDiff = pctx.RuleFunc("sAbiDiff",
		func(ctx android.PackageRuleContext) blueprint.RuleParams {
			// TODO(b/78139997): Add -check-all-apis back
			commandStr := "($sAbiDiffer ${allowFlags} -lib ${libName} -arch ${arch} -o ${out} -new ${in} -old ${referenceDump})"
			commandStr += "|| (echo 'error: Please update ABI references with: $$ANDROID_BUILD_TOP/development/vndk/tools/header-checker/utils/create_reference_dumps.py ${createReferenceDumpFlags} -l ${libName}'"
			commandStr += " && (mkdir -p $$DIST_DIR/abidiffs && cp ${out} $$DIST_DIR/abidiffs/)"
			commandStr += " && exit 1)"
			return blueprint.RuleParams{
				Command:     commandStr,
				CommandDeps: []string{"$sAbiDiffer"},
			}
		},
		"allowFlags", "referenceDump", "libName", "arch", "createReferenceDumpFlags")

	unzipRefSAbiDump = pctx.AndroidStaticRule("unzipRefSAbiDump",
		blueprint.RuleParams{
			Command: "gunzip -c $in > $out",
		})

	zip = pctx.AndroidStaticRule("zip",
		blueprint.RuleParams{
			Command:        "cat $out.rsp | tr ' ' '\\n' | tr -d \\' | sort -u > ${out}.tmp && ${SoongZipCmd} -o ${out} -C $$OUT_DIR -l ${out}.tmp",
			CommandDeps:    []string{"${SoongZipCmd}"},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		})

	_ = pctx.SourcePathVariable("cxxExtractor",
		"prebuilts/clang-tools/${config.HostPrebuiltTag}/bin/cxx_extractor")
	_ = pctx.SourcePathVariable("kytheVnames", "build/soong/vnames.json")
	_ = pctx.VariableFunc("kytheCorpus",
		func(ctx android.PackageVarContext) string { return ctx.Config().XrefCorpusName() })
	_ = pctx.VariableFunc("kytheCuEncoding",
		func(ctx android.PackageVarContext) string { return ctx.Config().XrefCuEncoding() })
	kytheExtract = pctx.StaticRule("kythe",
		blueprint.RuleParams{
			Command: `rm -f $out && ` +
				`KYTHE_CORPUS=${kytheCorpus} KYTHE_OUTPUT_FILE=$out KYTHE_VNAMES=$kytheVnames KYTHE_KZIP_ENCODING=${kytheCuEncoding} ` +
				`$cxxExtractor $cFlags $in `,
			CommandDeps: []string{"$cxxExtractor", "$kytheVnames"},
		},
		"cFlags")
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

	pctx.HostBinToolVariable("SoongZipCmd", "soong_zip")
	pctx.Import("android/soong/remoteexec")
}

type builderFlags struct {
	globalCommonFlags     string
	globalAsFlags         string
	globalYasmFlags       string
	globalCFlags          string
	globalToolingCFlags   string // A separate set of cFlags for clang LibTooling tools
	globalToolingCppFlags string // A separate set of cppFlags for clang LibTooling tools
	globalConlyFlags      string
	globalCppFlags        string
	globalLdFlags         string

	localCommonFlags     string
	localAsFlags         string
	localYasmFlags       string
	localCFlags          string
	localToolingCFlags   string // A separate set of cFlags for clang LibTooling tools
	localToolingCppFlags string // A separate set of cppFlags for clang LibTooling tools
	localConlyFlags      string
	localCppFlags        string
	localLdFlags         string

	libFlags      string
	extraLibFlags string
	tidyFlags     string
	sAbiFlags     string
	aidlFlags     string
	rsFlags       string
	toolchain     config.Toolchain
	tidy          bool
	gcovCoverage  bool
	sAbiDump      bool
	emitXrefs     bool

	assemblerWithCpp bool

	systemIncludeFlags string

	groupStaticLibs bool

	stripKeepSymbols              bool
	stripKeepSymbolsList          string
	stripKeepSymbolsAndDebugFrame bool
	stripKeepMiniDebugInfo        bool
	stripAddGnuDebuglink          bool
	stripUseGnuStrip              bool

	proto            android.ProtoFlags
	protoC           bool
	protoOptionsFile bool

	yacc *YaccProperties
}

type Objects struct {
	objFiles      android.Paths
	tidyFiles     android.Paths
	coverageFiles android.Paths
	sAbiDumpFiles android.Paths
	kytheFiles    android.Paths
}

func (a Objects) Copy() Objects {
	return Objects{
		objFiles:      append(android.Paths{}, a.objFiles...),
		tidyFiles:     append(android.Paths{}, a.tidyFiles...),
		coverageFiles: append(android.Paths{}, a.coverageFiles...),
		sAbiDumpFiles: append(android.Paths{}, a.sAbiDumpFiles...),
		kytheFiles:    append(android.Paths{}, a.kytheFiles...),
	}
}

func (a Objects) Append(b Objects) Objects {
	return Objects{
		objFiles:      append(a.objFiles, b.objFiles...),
		tidyFiles:     append(a.tidyFiles, b.tidyFiles...),
		coverageFiles: append(a.coverageFiles, b.coverageFiles...),
		sAbiDumpFiles: append(a.sAbiDumpFiles, b.sAbiDumpFiles...),
		kytheFiles:    append(a.kytheFiles, b.kytheFiles...),
	}
}

// Generate rules for compiling multiple .c, .cpp, or .S files to individual .o files
func TransformSourceToObj(ctx android.ModuleContext, subdir string, srcFiles android.Paths,
	flags builderFlags, pathDeps android.Paths, cFlagsDeps android.Paths) Objects {

	objFiles := make(android.Paths, len(srcFiles))
	var tidyFiles android.Paths
	if flags.tidy {
		tidyFiles = make(android.Paths, 0, len(srcFiles))
	}
	var coverageFiles android.Paths
	if flags.gcovCoverage {
		coverageFiles = make(android.Paths, 0, len(srcFiles))
	}
	var kytheFiles android.Paths
	if flags.emitXrefs {
		kytheFiles = make(android.Paths, 0, len(srcFiles))
	}

	// Produce fully expanded flags for use by C tools, C compiles, C++ tools, C++ compiles, and asm compiles
	// respectively.
	toolingCflags := flags.globalCommonFlags + " " +
		flags.globalToolingCFlags + " " +
		flags.globalConlyFlags + " " +
		flags.localCommonFlags + " " +
		flags.localToolingCFlags + " " +
		flags.localConlyFlags + " " +
		flags.systemIncludeFlags

	cflags := flags.globalCommonFlags + " " +
		flags.globalCFlags + " " +
		flags.globalConlyFlags + " " +
		flags.localCommonFlags + " " +
		flags.localCFlags + " " +
		flags.localConlyFlags + " " +
		flags.systemIncludeFlags

	toolingCppflags := flags.globalCommonFlags + " " +
		flags.globalToolingCFlags + " " +
		flags.globalToolingCppFlags + " " +
		flags.localCommonFlags + " " +
		flags.localToolingCFlags + " " +
		flags.localToolingCppFlags + " " +
		flags.systemIncludeFlags

	cppflags := flags.globalCommonFlags + " " +
		flags.globalCFlags + " " +
		flags.globalCppFlags + " " +
		flags.localCommonFlags + " " +
		flags.localCFlags + " " +
		flags.localCppFlags + " " +
		flags.systemIncludeFlags

	asflags := flags.globalCommonFlags + " " +
		flags.globalAsFlags + " " +
		flags.localCommonFlags + " " +
		flags.localAsFlags + " " +
		flags.systemIncludeFlags

	var sAbiDumpFiles android.Paths
	if flags.sAbiDump {
		sAbiDumpFiles = make(android.Paths, 0, len(srcFiles))
	}

	cflags += " ${config.NoOverrideClangGlobalCflags}"
	toolingCflags += " ${config.NoOverrideClangGlobalCflags}"
	cppflags += " ${config.NoOverrideClangGlobalCflags}"
	toolingCppflags += " ${config.NoOverrideClangGlobalCflags}"

	for i, srcFile := range srcFiles {
		objFile := android.ObjPathWithExt(ctx, subdir, srcFile, "o")

		objFiles[i] = objFile

		switch srcFile.Ext() {
		case ".asm":
			ctx.Build(pctx, android.BuildParams{
				Rule:        yasm,
				Description: "yasm " + srcFile.Rel(),
				Output:      objFile,
				Input:       srcFile,
				Implicits:   cFlagsDeps,
				OrderOnly:   pathDeps,
				Args: map[string]string{
					"asFlags": flags.globalYasmFlags + " " + flags.localYasmFlags,
				},
			})
			continue
		case ".rc":
			ctx.Build(pctx, android.BuildParams{
				Rule:        windres,
				Description: "windres " + srcFile.Rel(),
				Output:      objFile,
				Input:       srcFile,
				Implicits:   cFlagsDeps,
				OrderOnly:   pathDeps,
				Args: map[string]string{
					"windresCmd": gccCmd(flags.toolchain, "windres"),
					"flags":      flags.toolchain.WindresFlags(),
				},
			})
			continue
		case ".o":
			objFiles[i] = srcFile
			continue
		}

		var moduleFlags string
		var moduleToolingFlags string

		var ccCmd string
		tidy := flags.tidy
		coverage := flags.gcovCoverage
		dump := flags.sAbiDump
		rule := cc
		emitXref := flags.emitXrefs

		switch srcFile.Ext() {
		case ".s":
			if !flags.assemblerWithCpp {
				rule = ccNoDeps
			}
			fallthrough
		case ".S":
			ccCmd = "clang"
			moduleFlags = asflags
			tidy = false
			coverage = false
			dump = false
			emitXref = false
		case ".c":
			ccCmd = "clang"
			moduleFlags = cflags
			moduleToolingFlags = toolingCflags
		case ".cpp", ".cc", ".cxx", ".mm":
			ccCmd = "clang++"
			moduleFlags = cppflags
			moduleToolingFlags = toolingCppflags
		default:
			ctx.ModuleErrorf("File %s has unknown extension", srcFile)
			continue
		}

		ccDesc := ccCmd

		ccCmd = "${config.ClangBin}/" + ccCmd

		var implicitOutputs android.WritablePaths
		if coverage {
			gcnoFile := android.ObjPathWithExt(ctx, subdir, srcFile, "gcno")
			implicitOutputs = append(implicitOutputs, gcnoFile)
			coverageFiles = append(coverageFiles, gcnoFile)
		}

		ctx.Build(pctx, android.BuildParams{
			Rule:            rule,
			Description:     ccDesc + " " + srcFile.Rel(),
			Output:          objFile,
			ImplicitOutputs: implicitOutputs,
			Input:           srcFile,
			Implicits:       cFlagsDeps,
			OrderOnly:       pathDeps,
			Args: map[string]string{
				"cFlags": moduleFlags,
				"ccCmd":  ccCmd,
			},
		})

		if emitXref {
			kytheFile := android.ObjPathWithExt(ctx, subdir, srcFile, "kzip")
			ctx.Build(pctx, android.BuildParams{
				Rule:        kytheExtract,
				Description: "Xref C++ extractor " + srcFile.Rel(),
				Output:      kytheFile,
				Input:       srcFile,
				Implicits:   cFlagsDeps,
				OrderOnly:   pathDeps,
				Args: map[string]string{
					"cFlags": moduleFlags,
				},
			})
			kytheFiles = append(kytheFiles, kytheFile)
		}

		if tidy {
			tidyFile := android.ObjPathWithExt(ctx, subdir, srcFile, "tidy")
			tidyFiles = append(tidyFiles, tidyFile)

			ctx.Build(pctx, android.BuildParams{
				Rule:        clangTidy,
				Description: "clang-tidy " + srcFile.Rel(),
				Output:      tidyFile,
				Input:       srcFile,
				// We must depend on objFile, since clang-tidy doesn't
				// support exporting dependencies.
				Implicit:  objFile,
				Implicits: cFlagsDeps,
				OrderOnly: pathDeps,
				Args: map[string]string{
					"cFlags":    moduleToolingFlags,
					"tidyFlags": flags.tidyFlags,
				},
			})
		}

		if dump {
			sAbiDumpFile := android.ObjPathWithExt(ctx, subdir, srcFile, "sdump")
			sAbiDumpFiles = append(sAbiDumpFiles, sAbiDumpFile)

			ctx.Build(pctx, android.BuildParams{
				Rule:        sAbiDump,
				Description: "header-abi-dumper " + srcFile.Rel(),
				Output:      sAbiDumpFile,
				Input:       srcFile,
				Implicit:    objFile,
				Implicits:   cFlagsDeps,
				OrderOnly:   pathDeps,
				Args: map[string]string{
					"cFlags":     moduleToolingFlags,
					"exportDirs": flags.sAbiFlags,
				},
			})
		}

	}

	return Objects{
		objFiles:      objFiles,
		tidyFiles:     tidyFiles,
		coverageFiles: coverageFiles,
		sAbiDumpFiles: sAbiDumpFiles,
		kytheFiles:    kytheFiles,
	}
}

// Generate a rule for compiling multiple .o files to a static library (.a)
func TransformObjToStaticLib(ctx android.ModuleContext, objFiles android.Paths,
	flags builderFlags, outputFile android.ModuleOutPath, deps android.Paths) {

	arCmd := "${config.ClangBin}/llvm-ar"
	arFlags := "crsPD"
	if !ctx.Darwin() {
		arFlags += " -format=gnu"
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        ar,
		Description: "static link " + outputFile.Base(),
		Output:      outputFile,
		Inputs:      objFiles,
		Implicits:   deps,
		Args: map[string]string{
			"arFlags": arFlags,
			"arCmd":   arCmd,
		},
	})
}

// Generate a rule for compiling multiple .o files, plus static libraries, whole static libraries,
// and shared libraries, to a shared library (.so) or dynamic executable
func TransformObjToDynamicBinary(ctx android.ModuleContext,
	objFiles, sharedLibs, staticLibs, lateStaticLibs, wholeStaticLibs, deps android.Paths,
	crtBegin, crtEnd android.OptionalPath, groupLate bool, flags builderFlags, outputFile android.WritablePath, implicitOutputs android.WritablePaths) {

	ldCmd := "${config.ClangBin}/clang++"

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
		libFile := lib.String()
		if ctx.Windows() {
			libFile = pathtools.ReplaceExtension(libFile, "lib")
		}
		libFlagsList = append(libFlagsList, libFile)
	}

	deps = append(deps, staticLibs...)
	deps = append(deps, lateStaticLibs...)
	deps = append(deps, wholeStaticLibs...)
	if crtBegin.Valid() {
		deps = append(deps, crtBegin.Path(), crtEnd.Path())
	}

	rule := ld
	if ctx.Config().IsEnvTrue("RBE_CXX_LINKS") {
		rule = ldRE
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:            rule,
		Description:     "link " + outputFile.Base(),
		Output:          outputFile,
		ImplicitOutputs: implicitOutputs,
		Inputs:          objFiles,
		Implicits:       deps,
		Args: map[string]string{
			"ldCmd":         ldCmd,
			"crtBegin":      crtBegin.String(),
			"libFlags":      strings.Join(libFlagsList, " "),
			"extraLibFlags": flags.extraLibFlags,
			"ldFlags":       flags.globalLdFlags + " " + flags.localLdFlags,
			"crtEnd":        crtEnd.String(),
		},
	})
}

// Generate a rule to combine .dump sAbi dump files from multiple source files
// into a single .ldump sAbi dump file
func TransformDumpToLinkedDump(ctx android.ModuleContext, sAbiDumps android.Paths, soFile android.Path,
	baseName, exportedHeaderFlags string, symbolFile android.OptionalPath,
	excludedSymbolVersions, excludedSymbolTags []string) android.OptionalPath {

	outputFile := android.PathForModuleOut(ctx, baseName+".lsdump")

	implicits := android.Paths{soFile}
	symbolFilterStr := "-so " + soFile.String()

	if symbolFile.Valid() {
		implicits = append(implicits, symbolFile.Path())
		symbolFilterStr += " -v " + symbolFile.String()
	}
	for _, ver := range excludedSymbolVersions {
		symbolFilterStr += " --exclude-symbol-version " + ver
	}
	for _, tag := range excludedSymbolTags {
		symbolFilterStr += " --exclude-symbol-tag " + tag
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        sAbiLink,
		Description: "header-abi-linker " + outputFile.Base(),
		Output:      outputFile,
		Inputs:      sAbiDumps,
		Implicits:   implicits,
		Args: map[string]string{
			"symbolFilter":        symbolFilterStr,
			"arch":                ctx.Arch().ArchType.Name,
			"exportedHeaderFlags": exportedHeaderFlags,
		},
	})
	return android.OptionalPathForPath(outputFile)
}

func UnzipRefDump(ctx android.ModuleContext, zippedRefDump android.Path, baseName string) android.Path {
	outputFile := android.PathForModuleOut(ctx, baseName+"_ref.lsdump")
	ctx.Build(pctx, android.BuildParams{
		Rule:        unzipRefSAbiDump,
		Description: "gunzip" + outputFile.Base(),
		Output:      outputFile,
		Input:       zippedRefDump,
	})
	return outputFile
}

func SourceAbiDiff(ctx android.ModuleContext, inputDump android.Path, referenceDump android.Path,
	baseName, exportedHeaderFlags string, isLlndk, isNdk, isVndkExt bool) android.OptionalPath {

	outputFile := android.PathForModuleOut(ctx, baseName+".abidiff")
	libName := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	createReferenceDumpFlags := ""

	localAbiCheckAllowFlags := append([]string(nil), abiCheckAllowFlags...)
	if exportedHeaderFlags == "" {
		localAbiCheckAllowFlags = append(localAbiCheckAllowFlags, "-advice-only")
	}
	if isLlndk || isNdk {
		createReferenceDumpFlags = "--llndk"
		if isLlndk {
			// TODO(b/130324828): "-consider-opaque-types-different" should apply to
			// both LLNDK and NDK shared libs. However, a known issue in header-abi-diff
			// breaks libaaudio. Remove the if-guard after the issue is fixed.
			localAbiCheckAllowFlags = append(localAbiCheckAllowFlags, "-consider-opaque-types-different")
		}
	}
	if isVndkExt {
		localAbiCheckAllowFlags = append(localAbiCheckAllowFlags, "-allow-extensions")
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        sAbiDiff,
		Description: "header-abi-diff " + outputFile.Base(),
		Output:      outputFile,
		Input:       inputDump,
		Implicit:    referenceDump,
		Args: map[string]string{
			"referenceDump":            referenceDump.String(),
			"libName":                  libName,
			"arch":                     ctx.Arch().ArchType.Name,
			"allowFlags":               strings.Join(localAbiCheckAllowFlags, " "),
			"createReferenceDumpFlags": createReferenceDumpFlags,
		},
	})
	return android.OptionalPathForPath(outputFile)
}

// Generate a rule for extracting a table of contents from a shared library (.so)
func TransformSharedObjectToToc(ctx android.ModuleContext, inputFile android.Path,
	outputFile android.WritablePath, flags builderFlags) {

	var format string
	var crossCompile string
	if ctx.Darwin() {
		format = "--macho"
		crossCompile = "${config.MacToolPath}"
	} else if ctx.Windows() {
		format = "--pe"
		crossCompile = gccCmd(flags.toolchain, "")
	} else {
		format = "--elf"
		crossCompile = gccCmd(flags.toolchain, "")
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        toc,
		Description: "generate toc " + inputFile.Base(),
		Output:      outputFile,
		Input:       inputFile,
		Args: map[string]string{
			"crossCompile": crossCompile,
			"format":       format,
		},
	})
}

// Generate a rule for compiling multiple .o files to a .o using ld partial linking
func TransformObjsToObj(ctx android.ModuleContext, objFiles android.Paths,
	flags builderFlags, outputFile android.WritablePath, deps android.Paths) {

	ldCmd := "${config.ClangBin}/clang++"

	rule := partialLd
	args := map[string]string{
		"ldCmd":   ldCmd,
		"ldFlags": flags.globalLdFlags + " " + flags.localLdFlags,
	}
	if ctx.Config().IsEnvTrue("RBE_CXX_LINKS") {
		rule = partialLdRE
		args["inCommaList"] = strings.Join(objFiles.Strings(), ",")
	}
	ctx.Build(pctx, android.BuildParams{
		Rule:        rule,
		Description: "link " + outputFile.Base(),
		Output:      outputFile,
		Inputs:      objFiles,
		Implicits:   deps,
		Args:        args,
	})
}

// Generate a rule for runing objcopy --prefix-symbols on a binary
func TransformBinaryPrefixSymbols(ctx android.ModuleContext, prefix string, inputFile android.Path,
	flags builderFlags, outputFile android.WritablePath) {

	objcopyCmd := gccCmd(flags.toolchain, "objcopy")

	ctx.Build(pctx, android.BuildParams{
		Rule:        prefixSymbols,
		Description: "prefix symbols " + outputFile.Base(),
		Output:      outputFile,
		Input:       inputFile,
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
	if flags.stripKeepSymbolsList != "" {
		args += " -k" + flags.stripKeepSymbolsList
	}
	if flags.stripKeepSymbolsAndDebugFrame {
		args += " --keep-symbols-and-debug-frame"
	}
	if flags.stripUseGnuStrip {
		args += " --use-gnu-strip"
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        strip,
		Description: "strip " + outputFile.Base(),
		Output:      outputFile,
		Input:       inputFile,
		Args: map[string]string{
			"crossCompile": crossCompile,
			"args":         args,
		},
	})
}

func TransformDarwinStrip(ctx android.ModuleContext, inputFile android.Path,
	outputFile android.WritablePath) {

	ctx.Build(pctx, android.BuildParams{
		Rule:        darwinStrip,
		Description: "strip " + outputFile.Base(),
		Output:      outputFile,
		Input:       inputFile,
	})
}

func TransformCoverageFilesToZip(ctx android.ModuleContext,
	inputs Objects, baseName string) android.OptionalPath {

	if len(inputs.coverageFiles) > 0 {
		outputFile := android.PathForModuleOut(ctx, baseName+".zip")

		ctx.Build(pctx, android.BuildParams{
			Rule:        zip,
			Description: "zip " + outputFile.Base(),
			Inputs:      inputs.coverageFiles,
			Output:      outputFile,
		})

		return android.OptionalPathForPath(outputFile)
	}

	return android.OptionalPath{}
}

func TransformArchiveRepack(ctx android.ModuleContext, inputFile android.Path,
	outputFile android.WritablePath, objects []string) {

	ctx.Build(pctx, android.BuildParams{
		Rule:        archiveRepack,
		Description: "Repack archive " + outputFile.Base(),
		Output:      outputFile,
		Input:       inputFile,
		Args: map[string]string{
			"objects": strings.Join(objects, " "),
		},
	})
}

func gccCmd(toolchain config.Toolchain, cmd string) string {
	return filepath.Join(toolchain.GccRoot(), "bin", toolchain.GccTriple()+"-"+cmd)
}

func splitListForSize(list android.Paths, limit int) (lists []android.Paths, err error) {
	var i int

	start := 0
	bytes := 0
	for i = range list {
		l := len(list[i].String())
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
