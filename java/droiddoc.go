// Copyright 2018 Google Inc. All rights reserved.
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

package java

import (
	"android/soong/android"
	"android/soong/java/config"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/google/blueprint"
)

var (
	javadoc = pctx.AndroidStaticRule("javadoc",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$srcJarDir" "$stubsDir" && mkdir -p "$outDir" "$srcJarDir" "$stubsDir" && ` +
				`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
				`${config.SoongJavacWrapper} ${config.JavadocCmd} -encoding UTF-8 @$out.rsp @$srcJarDir/list ` +
				`$opts $bootclasspathArgs $classpathArgs $sourcepathArgs ` +
				`-d $outDir -quiet  && ` +
				`${config.SoongZipCmd} -write_if_changed -d -o $docZip -C $outDir -D $outDir && ` +
				`${config.SoongZipCmd} -write_if_changed -jar -o $out -C $stubsDir -D $stubsDir $postDoclavaCmds && ` +
				`rm -rf "$srcJarDir"`,

			CommandDeps: []string{
				"${config.ZipSyncCmd}",
				"${config.JavadocCmd}",
				"${config.SoongZipCmd}",
			},
			CommandOrderOnly: []string{"${config.SoongJavacWrapper}"},
			Rspfile:          "$out.rsp",
			RspfileContent:   "$in",
			Restat:           true,
		},
		"outDir", "srcJarDir", "stubsDir", "srcJars", "opts",
		"bootclasspathArgs", "classpathArgs", "sourcepathArgs", "docZip", "postDoclavaCmds")

	apiCheck = pctx.AndroidStaticRule("apiCheck",
		blueprint.RuleParams{
			Command: `( ${config.ApiCheckCmd} -JXmx1024m -J"classpath $classpath" $opts ` +
				`$apiFile $apiFileToCheck $removedApiFile $removedApiFileToCheck ` +
				`&& touch $out ) || (echo -e "$msg" ; exit 38)`,
			CommandDeps: []string{
				"${config.ApiCheckCmd}",
			},
		},
		"classpath", "opts", "apiFile", "apiFileToCheck", "removedApiFile", "removedApiFileToCheck", "msg")

	updateApi = pctx.AndroidStaticRule("updateApi",
		blueprint.RuleParams{
			Command: `( ( cp -f $srcApiFile $destApiFile && cp -f $srcRemovedApiFile $destRemovedApiFile ) ` +
				`&& touch $out ) || (echo failed to update public API ; exit 38)`,
		},
		"srcApiFile", "destApiFile", "srcRemovedApiFile", "destRemovedApiFile")

	metalava = pctx.AndroidStaticRule("metalava",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$srcJarDir" "$stubsDir" && ` +
				`mkdir -p "$outDir" "$srcJarDir" "$stubsDir" && ` +
				`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
				`${config.JavaCmd} -jar ${config.MetalavaJar} -encoding UTF-8 -source $javaVersion @$out.rsp @$srcJarDir/list ` +
				`$bootclasspathArgs $classpathArgs $sourcepathArgs --no-banner --color --quiet --format=v2 ` +
				`$opts && ` +
				`${config.SoongZipCmd} -write_if_changed -jar -o $out -C $stubsDir -D $stubsDir && ` +
				`(if $writeSdkValues; then ${config.SoongZipCmd} -write_if_changed -d -o $metadataZip ` +
				`-C $metadataDir -D $metadataDir; fi) && ` +
				`rm -rf "$srcJarDir"`,
			CommandDeps: []string{
				"${config.ZipSyncCmd}",
				"${config.JavaCmd}",
				"${config.MetalavaJar}",
				"${config.SoongZipCmd}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
			Restat:         true,
		},
		"outDir", "srcJarDir", "stubsDir", "srcJars", "javaVersion", "bootclasspathArgs",
		"classpathArgs", "sourcepathArgs", "opts", "writeSdkValues", "metadataZip", "metadataDir")

	metalavaApiCheck = pctx.AndroidStaticRule("metalavaApiCheck",
		blueprint.RuleParams{
			Command: `( rm -rf "$srcJarDir" && mkdir -p "$srcJarDir" && ` +
				`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
				`${config.JavaCmd} -jar ${config.MetalavaJar} -encoding UTF-8 -source $javaVersion @$out.rsp @$srcJarDir/list ` +
				`$bootclasspathArgs $classpathArgs $sourcepathArgs --no-banner --color --quiet --format=v2 ` +
				`$opts && touch $out && rm -rf "$srcJarDir") || ` +
				`( echo -e "$msg" ; exit 38 )`,
			CommandDeps: []string{
				"${config.ZipSyncCmd}",
				"${config.JavaCmd}",
				"${config.MetalavaJar}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
		},
		"srcJarDir", "srcJars", "javaVersion", "bootclasspathArgs", "classpathArgs", "sourcepathArgs", "opts", "msg")

	nullabilityWarningsCheck = pctx.AndroidStaticRule("nullabilityWarningsCheck",
		blueprint.RuleParams{
			Command: `( diff $expected $actual && touch $out ) || ( echo -e "$msg" ; exit 38 )`,
		},
		"expected", "actual", "msg")

	dokka = pctx.AndroidStaticRule("dokka",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$srcJarDir" "$stubsDir" && ` +
				`mkdir -p "$outDir" "$srcJarDir" "$stubsDir" && ` +
				`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
				`${config.JavaCmd} -jar ${config.DokkaJar} $srcJarDir ` +
				`$classpathArgs -format dac -dacRoot /reference/kotlin -output $outDir $opts && ` +
				`${config.SoongZipCmd} -write_if_changed -d -o $docZip -C $outDir -D $outDir && ` +
				`${config.SoongZipCmd} -write_if_changed -jar -o $out -C $stubsDir -D $stubsDir && ` +
				`rm -rf "$srcJarDir"`,
			CommandDeps: []string{
				"${config.ZipSyncCmd}",
				"${config.DokkaJar}",
				"${config.MetalavaJar}",
				"${config.SoongZipCmd}",
			},
			Restat: true,
		},
		"outDir", "srcJarDir", "stubsDir", "srcJars", "classpathArgs", "opts", "docZip")
)

func init() {
	android.RegisterModuleType("doc_defaults", DocDefaultsFactory)
	android.RegisterModuleType("stubs_defaults", StubsDefaultsFactory)

	android.RegisterModuleType("droiddoc", DroiddocFactory)
	android.RegisterModuleType("droiddoc_host", DroiddocHostFactory)
	android.RegisterModuleType("droiddoc_exported_dir", ExportedDroiddocDirFactory)
	android.RegisterModuleType("javadoc", JavadocFactory)
	android.RegisterModuleType("javadoc_host", JavadocHostFactory)

	android.RegisterModuleType("droidstubs", DroidstubsFactory)
	android.RegisterModuleType("droidstubs_host", DroidstubsHostFactory)
}

var (
	srcsLibTag = dependencyTag{name: "sources from javalib"}
)

type JavadocProperties struct {
	// list of source files used to compile the Java module.  May be .java, .logtags, .proto,
	// or .aidl files.
	Srcs []string `android:"path,arch_variant"`

	// list of directories rooted at the Android.bp file that will
	// be added to the search paths for finding source files when passing package names.
	Local_sourcepaths []string

	// list of source files that should not be used to build the Java module.
	// This is most useful in the arch/multilib variants to remove non-common files
	// filegroup or genrule can be included within this property.
	Exclude_srcs []string `android:"path,arch_variant"`

	// list of java libraries that will be in the classpath.
	Libs []string `android:"arch_variant"`

	// don't build against the default libraries (bootclasspath, ext, and framework for device
	// targets)
	No_standard_libs *bool

	// don't build against the framework libraries (ext, and framework for device targets)
	No_framework_libs *bool

	// the java library (in classpath) for documentation that provides java srcs and srcjars.
	Srcs_lib *string

	// the base dirs under srcs_lib will be scanned for java srcs.
	Srcs_lib_whitelist_dirs []string

	// the sub dirs under srcs_lib_whitelist_dirs will be scanned for java srcs.
	Srcs_lib_whitelist_pkgs []string

	// If set to false, don't allow this module(-docs.zip) to be exported. Defaults to true.
	Installable *bool

	// if not blank, set to the version of the sdk to compile against
	Sdk_version *string `android:"arch_variant"`

	Aidl struct {
		// Top level directories to pass to aidl tool
		Include_dirs []string

		// Directories rooted at the Android.bp file to pass to aidl tool
		Local_include_dirs []string
	}

	// If not blank, set the java version passed to javadoc as -source
	Java_version *string

	// local files that are used within user customized droiddoc options.
	Arg_files []string `android:"path"`

	// user customized droiddoc args.
	// Available variables for substitution:
	//
	//  $(location <label>): the path to the arg_files with name <label>
	Args *string

	// names of the output files used in args that will be generated
	Out []string
}

type ApiToCheck struct {
	// path to the API txt file that the new API extracted from source code is checked
	// against. The path can be local to the module or from other module (via :module syntax).
	Api_file *string `android:"path"`

	// path to the API txt file that the new @removed API extractd from source code is
	// checked against. The path can be local to the module or from other module (via
	// :module syntax).
	Removed_api_file *string `android:"path"`

	// If not blank, path to the baseline txt file for approved API check violations.
	Baseline_file *string `android:"path"`

	// Arguments to the apicheck tool.
	Args *string
}

type DroiddocProperties struct {
	// directory relative to top of the source tree that contains doc templates files.
	Custom_template *string

	// directories under current module source which contains html/jd files.
	Html_dirs []string

	// set a value in the Clearsilver hdf namespace.
	Hdf []string

	// proofread file contains all of the text content of the javadocs concatenated into one file,
	// suitable for spell-checking and other goodness.
	Proofread_file *string `android:"path"`

	// a todo file lists the program elements that are missing documentation.
	// At some point, this might be improved to show more warnings.
	Todo_file *string `android:"path"`

	// directory under current module source that provide additional resources (images).
	Resourcesdir *string

	// resources output directory under out/soong/.intermediates.
	Resourcesoutdir *string

	// if set to true, collect the values used by the Dev tools and
	// write them in files packaged with the SDK. Defaults to false.
	Write_sdk_values *bool

	// index.html under current module will be copied to docs out dir, if not null.
	Static_doc_index_redirect *string `android:"path"`

	// source.properties under current module will be copied to docs out dir, if not null.
	Static_doc_properties *string `android:"path"`

	// a list of files under current module source dir which contains known tags in Java sources.
	// filegroup or genrule can be included within this property.
	Knowntags []string `android:"path"`

	// the tag name used to distinguish if the API files belong to public/system/test.
	Api_tag_name *string

	// the generated public API filename by Doclava.
	Api_filename *string

	// the generated public Dex API filename by Doclava.
	Dex_api_filename *string

	// the generated private API filename by Doclava.
	Private_api_filename *string

	// the generated private Dex API filename by Doclava.
	Private_dex_api_filename *string

	// the generated removed API filename by Doclava.
	Removed_api_filename *string

	// the generated removed Dex API filename by Doclava.
	Removed_dex_api_filename *string

	// mapping of dex signatures to source file and line number. This is a temporary property and
	// will be deleted; you probably shouldn't be using it.
	Dex_mapping_filename *string

	// the generated exact API filename by Doclava.
	Exact_api_filename *string

	// the generated proguard filename by Doclava.
	Proguard_filename *string

	// if set to false, don't allow droiddoc to generate stubs source files. Defaults to true.
	Create_stubs *bool

	Check_api struct {
		Last_released ApiToCheck

		Current ApiToCheck

		// do not perform API check against Last_released, in the case that both two specified API
		// files by Last_released are modules which don't exist.
		Ignore_missing_latest_api *bool `blueprint:"mutated"`
	}

	// if set to true, generate docs through Dokka instead of Doclava.
	Dokka_enabled *bool
}

type DroidstubsProperties struct {
	// the tag name used to distinguish if the API files belong to public/system/test.
	Api_tag_name *string

	// the generated public API filename by Metalava.
	Api_filename *string

	// the generated public Dex API filename by Metalava.
	Dex_api_filename *string

	// the generated private API filename by Metalava.
	Private_api_filename *string

	// the generated private Dex API filename by Metalava.
	Private_dex_api_filename *string

	// the generated removed API filename by Metalava.
	Removed_api_filename *string

	// the generated removed Dex API filename by Metalava.
	Removed_dex_api_filename *string

	// mapping of dex signatures to source file and line number. This is a temporary property and
	// will be deleted; you probably shouldn't be using it.
	Dex_mapping_filename *string

	// the generated exact API filename by Metalava.
	Exact_api_filename *string

	// the generated proguard filename by Metalava.
	Proguard_filename *string

	Check_api struct {
		Last_released ApiToCheck

		Current ApiToCheck

		// do not perform API check against Last_released, in the case that both two specified API
		// files by Last_released are modules which don't exist.
		Ignore_missing_latest_api *bool `blueprint:"mutated"`
	}

	// user can specify the version of previous released API file in order to do compatibility check.
	Previous_api *string `android:"path"`

	// is set to true, Metalava will allow framework SDK to contain annotations.
	Annotations_enabled *bool

	// a list of top-level directories containing files to merge qualifier annotations (i.e. those intended to be included in the stubs written) from.
	Merge_annotations_dirs []string

	// a list of top-level directories containing Java stub files to merge show/hide annotations from.
	Merge_inclusion_annotations_dirs []string

	// a file containing a list of classes to do nullability validation for.
	Validate_nullability_from_list *string

	// a file containing expected warnings produced by validation of nullability annotations.
	Check_nullability_warnings *string

	// if set to true, allow Metalava to generate doc_stubs source files. Defaults to false.
	Create_doc_stubs *bool

	// is set to true, Metalava will allow framework SDK to contain API levels annotations.
	Api_levels_annotations_enabled *bool

	// the dirs which Metalava extracts API levels annotations from.
	Api_levels_annotations_dirs []string

	// if set to true, collect the values used by the Dev tools and
	// write them in files packaged with the SDK. Defaults to false.
	Write_sdk_values *bool

	// If set to true, .xml based public API file will be also generated, and
	// JDiff tool will be invoked to genreate javadoc files. Defaults to false.
	Jdiff_enabled *bool
}

//
// Common flags passed down to build rule
//
type droiddocBuilderFlags struct {
	bootClasspathArgs  string
	classpathArgs      string
	sourcepathArgs     string
	dokkaClasspathArgs string
	aidlFlags          string
	aidlDeps           android.Paths

	doclavaStubsFlags string
	doclavaDocsFlags  string
	postDoclavaCmds   string

	metalavaStubsFlags                string
	metalavaAnnotationsFlags          string
	metalavaMergeAnnoDirFlags         string
	metalavaInclusionAnnotationsFlags string
	metalavaApiLevelsAnnotationsFlags string

	metalavaApiToXmlFlags string
}

func InitDroiddocModule(module android.DefaultableModule, hod android.HostOrDeviceSupported) {
	android.InitAndroidArchModule(module, hod, android.MultilibCommon)
	android.InitDefaultableModule(module)
}

func apiCheckEnabled(ctx android.ModuleContext, apiToCheck ApiToCheck, apiVersionTag string) bool {
	if ctx.Config().IsEnvTrue("WITHOUT_CHECK_API") {
		return false
	} else if String(apiToCheck.Api_file) != "" && String(apiToCheck.Removed_api_file) != "" {
		return true
	} else if String(apiToCheck.Api_file) != "" {
		panic("for " + apiVersionTag + " removed_api_file has to be non-empty!")
	} else if String(apiToCheck.Removed_api_file) != "" {
		panic("for " + apiVersionTag + " api_file has to be non-empty!")
	}

	return false
}

func ignoreMissingModules(ctx android.BottomUpMutatorContext, apiToCheck *ApiToCheck) {
	api_file := String(apiToCheck.Api_file)
	removed_api_file := String(apiToCheck.Removed_api_file)

	api_module := android.SrcIsModule(api_file)
	removed_api_module := android.SrcIsModule(removed_api_file)

	if api_module == "" || removed_api_module == "" {
		return
	}

	if ctx.OtherModuleExists(api_module) || ctx.OtherModuleExists(removed_api_module) {
		return
	}

	apiToCheck.Api_file = nil
	apiToCheck.Removed_api_file = nil
}

type ApiFilePath interface {
	ApiFilePath() android.Path
}

func transformUpdateApi(ctx android.ModuleContext, destApiFile, destRemovedApiFile,
	srcApiFile, srcRemovedApiFile android.Path, output android.WritablePath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        updateApi,
		Description: "Update API",
		Output:      output,
		Implicits: append(android.Paths{}, srcApiFile, srcRemovedApiFile,
			destApiFile, destRemovedApiFile),
		Args: map[string]string{
			"destApiFile":        destApiFile.String(),
			"srcApiFile":         srcApiFile.String(),
			"destRemovedApiFile": destRemovedApiFile.String(),
			"srcRemovedApiFile":  srcRemovedApiFile.String(),
		},
	})
}

//
// Javadoc
//
type Javadoc struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties JavadocProperties

	srcJars     android.Paths
	srcFiles    android.Paths
	sourcepaths android.Paths
	argFiles    android.Paths

	args string

	docZip      android.WritablePath
	stubsSrcJar android.WritablePath
}

func (j *Javadoc) Srcs() android.Paths {
	return android.Paths{j.stubsSrcJar}
}

func JavadocFactory() android.Module {
	module := &Javadoc{}

	module.AddProperties(&module.properties)

	InitDroiddocModule(module, android.HostAndDeviceSupported)
	return module
}

func JavadocHostFactory() android.Module {
	module := &Javadoc{}

	module.AddProperties(&module.properties)

	InitDroiddocModule(module, android.HostSupported)
	return module
}

var _ android.SourceFileProducer = (*Javadoc)(nil)

func (j *Javadoc) sdkVersion() string {
	return String(j.properties.Sdk_version)
}

func (j *Javadoc) minSdkVersion() string {
	return j.sdkVersion()
}

func (j *Javadoc) targetSdkVersion() string {
	return j.sdkVersion()
}

func (j *Javadoc) addDeps(ctx android.BottomUpMutatorContext) {
	if ctx.Device() {
		if !Bool(j.properties.No_standard_libs) {
			sdkDep := decodeSdkDep(ctx, sdkContext(j))
			if sdkDep.useDefaultLibs {
				ctx.AddVariationDependencies(nil, bootClasspathTag, config.DefaultBootclasspathLibraries...)
				if ctx.Config().TargetOpenJDK9() {
					ctx.AddVariationDependencies(nil, systemModulesTag, config.DefaultSystemModules)
				}
				if !Bool(j.properties.No_framework_libs) {
					ctx.AddVariationDependencies(nil, libTag, config.DefaultLibraries...)
				}
			} else if sdkDep.useModule {
				if ctx.Config().TargetOpenJDK9() {
					ctx.AddVariationDependencies(nil, systemModulesTag, sdkDep.systemModules)
				}
				ctx.AddVariationDependencies(nil, bootClasspathTag, sdkDep.modules...)
			}
		}
	}

	ctx.AddVariationDependencies(nil, libTag, j.properties.Libs...)
	if j.properties.Srcs_lib != nil {
		ctx.AddVariationDependencies(nil, srcsLibTag, *j.properties.Srcs_lib)
	}
}

func (j *Javadoc) genWhitelistPathPrefixes(whitelistPathPrefixes map[string]bool) {
	for _, dir := range j.properties.Srcs_lib_whitelist_dirs {
		for _, pkg := range j.properties.Srcs_lib_whitelist_pkgs {
			// convert foo.bar.baz to foo/bar/baz
			pkgAsPath := filepath.Join(strings.Split(pkg, ".")...)
			prefix := filepath.Join(dir, pkgAsPath)
			if _, found := whitelistPathPrefixes[prefix]; !found {
				whitelistPathPrefixes[prefix] = true
			}
		}
	}
}

func (j *Javadoc) collectAidlFlags(ctx android.ModuleContext, deps deps) droiddocBuilderFlags {
	var flags droiddocBuilderFlags

	flags.aidlFlags, flags.aidlDeps = j.aidlFlags(ctx, deps.aidlPreprocess, deps.aidlIncludeDirs)

	return flags
}

func (j *Javadoc) aidlFlags(ctx android.ModuleContext, aidlPreprocess android.OptionalPath,
	aidlIncludeDirs android.Paths) (string, android.Paths) {

	aidlIncludes := android.PathsForModuleSrc(ctx, j.properties.Aidl.Local_include_dirs)
	aidlIncludes = append(aidlIncludes, android.PathsForSource(ctx, j.properties.Aidl.Include_dirs)...)

	var flags []string
	var deps android.Paths

	if aidlPreprocess.Valid() {
		flags = append(flags, "-p"+aidlPreprocess.String())
		deps = append(deps, aidlPreprocess.Path())
	} else {
		flags = append(flags, android.JoinWithPrefix(aidlIncludeDirs.Strings(), "-I"))
	}

	flags = append(flags, android.JoinWithPrefix(aidlIncludes.Strings(), "-I"))
	flags = append(flags, "-I"+android.PathForModuleSrc(ctx).String())
	if src := android.ExistentPathForSource(ctx, ctx.ModuleDir(), "src"); src.Valid() {
		flags = append(flags, "-I"+src.String())
	}

	return strings.Join(flags, " "), deps
}

func (j *Javadoc) genSources(ctx android.ModuleContext, srcFiles android.Paths,
	flags droiddocBuilderFlags) android.Paths {

	outSrcFiles := make(android.Paths, 0, len(srcFiles))

	for _, srcFile := range srcFiles {
		switch srcFile.Ext() {
		case ".aidl":
			javaFile := genAidl(ctx, srcFile, flags.aidlFlags, flags.aidlDeps)
			outSrcFiles = append(outSrcFiles, javaFile)
		case ".sysprop":
			javaFile := genSysprop(ctx, srcFile)
			outSrcFiles = append(outSrcFiles, javaFile)
		default:
			outSrcFiles = append(outSrcFiles, srcFile)
		}
	}

	return outSrcFiles
}

func (j *Javadoc) collectDeps(ctx android.ModuleContext) deps {
	var deps deps

	sdkDep := decodeSdkDep(ctx, sdkContext(j))
	if sdkDep.invalidVersion {
		ctx.AddMissingDependencies(sdkDep.modules)
	} else if sdkDep.useFiles {
		deps.bootClasspath = append(deps.bootClasspath, sdkDep.jars...)
	}

	ctx.VisitDirectDeps(func(module android.Module) {
		otherName := ctx.OtherModuleName(module)
		tag := ctx.OtherModuleDependencyTag(module)

		switch tag {
		case bootClasspathTag:
			if dep, ok := module.(Dependency); ok {
				deps.bootClasspath = append(deps.bootClasspath, dep.ImplementationJars()...)
			} else {
				panic(fmt.Errorf("unknown dependency %q for %q", otherName, ctx.ModuleName()))
			}
		case libTag:
			switch dep := module.(type) {
			case SdkLibraryDependency:
				deps.classpath = append(deps.classpath, dep.SdkImplementationJars(ctx, j.sdkVersion())...)
			case Dependency:
				deps.classpath = append(deps.classpath, dep.HeaderJars()...)
			case android.SourceFileProducer:
				checkProducesJars(ctx, dep)
				deps.classpath = append(deps.classpath, dep.Srcs()...)
			default:
				ctx.ModuleErrorf("depends on non-java module %q", otherName)
			}
		case srcsLibTag:
			switch dep := module.(type) {
			case Dependency:
				srcs := dep.(SrcDependency).CompiledSrcs()
				whitelistPathPrefixes := make(map[string]bool)
				j.genWhitelistPathPrefixes(whitelistPathPrefixes)
				for _, src := range srcs {
					if _, ok := src.(android.WritablePath); ok { // generated sources
						deps.srcs = append(deps.srcs, src)
					} else { // select source path for documentation based on whitelist path prefixs.
						for k := range whitelistPathPrefixes {
							if strings.HasPrefix(src.Rel(), k) {
								deps.srcs = append(deps.srcs, src)
								break
							}
						}
					}
				}
				deps.srcJars = append(deps.srcJars, dep.(SrcDependency).CompiledSrcJars()...)
			default:
				ctx.ModuleErrorf("depends on non-java module %q", otherName)
			}
		case systemModulesTag:
			if deps.systemModules != nil {
				panic("Found two system module dependencies")
			}
			sm := module.(*SystemModules)
			if sm.outputFile == nil {
				panic("Missing directory for system module dependency")
			}
			deps.systemModules = sm.outputFile
		}
	})
	// do not pass exclude_srcs directly when expanding srcFiles since exclude_srcs
	// may contain filegroup or genrule.
	srcFiles := android.PathsForModuleSrcExcludes(ctx, j.properties.Srcs, j.properties.Exclude_srcs)
	flags := j.collectAidlFlags(ctx, deps)
	srcFiles = j.genSources(ctx, srcFiles, flags)

	// srcs may depend on some genrule output.
	j.srcJars = srcFiles.FilterByExt(".srcjar")
	j.srcJars = append(j.srcJars, deps.srcJars...)

	j.srcFiles = srcFiles.FilterOutByExt(".srcjar")
	j.srcFiles = append(j.srcFiles, deps.srcs...)

	j.docZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"docs.zip")
	j.stubsSrcJar = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"stubs.srcjar")

	if j.properties.Local_sourcepaths == nil && len(j.srcFiles) > 0 {
		j.properties.Local_sourcepaths = append(j.properties.Local_sourcepaths, ".")
	}
	j.sourcepaths = android.PathsForModuleSrc(ctx, j.properties.Local_sourcepaths)

	j.argFiles = android.PathsForModuleSrc(ctx, j.properties.Arg_files)
	argFilesMap := map[string]string{}
	argFileLabels := []string{}

	for _, label := range j.properties.Arg_files {
		var paths = android.PathsForModuleSrc(ctx, []string{label})
		if _, exists := argFilesMap[label]; !exists {
			argFilesMap[label] = strings.Join(paths.Strings(), " ")
			argFileLabels = append(argFileLabels, label)
		} else {
			ctx.ModuleErrorf("multiple arg_files for %q, %q and %q",
				label, argFilesMap[label], paths)
		}
	}

	var err error
	j.args, err = android.Expand(String(j.properties.Args), func(name string) (string, error) {
		if strings.HasPrefix(name, "location ") {
			label := strings.TrimSpace(strings.TrimPrefix(name, "location "))
			if paths, ok := argFilesMap[label]; ok {
				return paths, nil
			} else {
				return "", fmt.Errorf("unknown location label %q, expecting one of %q",
					label, strings.Join(argFileLabels, ", "))
			}
		} else if name == "genDir" {
			return android.PathForModuleGen(ctx).String(), nil
		}
		return "", fmt.Errorf("unknown variable '$(%s)'", name)
	})

	if err != nil {
		ctx.PropertyErrorf("args", "%s", err.Error())
	}

	return deps
}

func (j *Javadoc) DepsMutator(ctx android.BottomUpMutatorContext) {
	j.addDeps(ctx)
}

func (j *Javadoc) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	deps := j.collectDeps(ctx)

	var implicits android.Paths
	implicits = append(implicits, deps.bootClasspath...)
	implicits = append(implicits, deps.classpath...)

	var bootClasspathArgs, classpathArgs, sourcepathArgs string

	javaVersion := getJavaVersion(ctx, String(j.properties.Java_version), sdkContext(j))
	if len(deps.bootClasspath) > 0 {
		var systemModules classpath
		if deps.systemModules != nil {
			systemModules = append(systemModules, deps.systemModules)
		}
		bootClasspathArgs = systemModules.FormJavaSystemModulesPath("--system ", ctx.Device())
		bootClasspathArgs = bootClasspathArgs + " --patch-module java.base=."
	}
	if len(deps.classpath.Strings()) > 0 {
		classpathArgs = "-classpath " + strings.Join(deps.classpath.Strings(), ":")
	}

	implicits = append(implicits, j.srcJars...)
	implicits = append(implicits, j.argFiles...)

	opts := "-source " + javaVersion + " -J-Xmx1024m -XDignore.symbol.file -Xdoclint:none"

	sourcepathArgs = "-sourcepath " + strings.Join(j.sourcepaths.Strings(), ":")

	ctx.Build(pctx, android.BuildParams{
		Rule:           javadoc,
		Description:    "Javadoc",
		Output:         j.stubsSrcJar,
		ImplicitOutput: j.docZip,
		Inputs:         j.srcFiles,
		Implicits:      implicits,
		Args: map[string]string{
			"outDir":            android.PathForModuleOut(ctx, "out").String(),
			"srcJarDir":         android.PathForModuleOut(ctx, "srcjars").String(),
			"stubsDir":          android.PathForModuleOut(ctx, "stubsDir").String(),
			"srcJars":           strings.Join(j.srcJars.Strings(), " "),
			"opts":              opts,
			"bootclasspathArgs": bootClasspathArgs,
			"classpathArgs":     classpathArgs,
			"sourcepathArgs":    sourcepathArgs,
			"docZip":            j.docZip.String(),
		},
	})
}

//
// Droiddoc
//
type Droiddoc struct {
	Javadoc

	properties        DroiddocProperties
	apiFile           android.WritablePath
	dexApiFile        android.WritablePath
	privateApiFile    android.WritablePath
	privateDexApiFile android.WritablePath
	removedApiFile    android.WritablePath
	removedDexApiFile android.WritablePath
	exactApiFile      android.WritablePath
	apiMappingFile    android.WritablePath
	proguardFile      android.WritablePath

	checkCurrentApiTimestamp      android.WritablePath
	updateCurrentApiTimestamp     android.WritablePath
	checkLastReleasedApiTimestamp android.WritablePath

	apiFilePath android.Path
}

func DroiddocFactory() android.Module {
	module := &Droiddoc{}

	module.AddProperties(&module.properties,
		&module.Javadoc.properties)

	InitDroiddocModule(module, android.HostAndDeviceSupported)
	return module
}

func DroiddocHostFactory() android.Module {
	module := &Droiddoc{}

	module.AddProperties(&module.properties,
		&module.Javadoc.properties)

	InitDroiddocModule(module, android.HostSupported)
	return module
}

func (d *Droiddoc) ApiFilePath() android.Path {
	return d.apiFilePath
}

func (d *Droiddoc) DepsMutator(ctx android.BottomUpMutatorContext) {
	d.Javadoc.addDeps(ctx)

	if Bool(d.properties.Check_api.Ignore_missing_latest_api) {
		ignoreMissingModules(ctx, &d.properties.Check_api.Last_released)
	}

	if String(d.properties.Custom_template) != "" {
		ctx.AddDependency(ctx.Module(), droiddocTemplateTag, String(d.properties.Custom_template))
	}
}

func (d *Droiddoc) initBuilderFlags(ctx android.ModuleContext, implicits *android.Paths,
	deps deps) (droiddocBuilderFlags, error) {
	var flags droiddocBuilderFlags

	*implicits = append(*implicits, deps.bootClasspath...)
	*implicits = append(*implicits, deps.classpath...)

	if len(deps.bootClasspath.Strings()) > 0 {
		// For OpenJDK 8 we can use -bootclasspath to define the core libraries code.
		flags.bootClasspathArgs = deps.bootClasspath.FormJavaClassPath("-bootclasspath")
	}
	flags.classpathArgs = deps.classpath.FormJavaClassPath("-classpath")
	// Dokka doesn't support bootClasspath, so combine these two classpath vars for Dokka.
	dokkaClasspath := classpath{}
	dokkaClasspath = append(dokkaClasspath, deps.bootClasspath...)
	dokkaClasspath = append(dokkaClasspath, deps.classpath...)
	flags.dokkaClasspathArgs = dokkaClasspath.FormJavaClassPath("-classpath")

	// TODO(nanzhang): Remove this if- statement once we finish migration for all Doclava
	// based stubs generation.
	// In the future, all the docs generation depends on Metalava stubs (droidstubs) srcjar
	// dir. We need add the srcjar dir to -sourcepath arg, so that Javadoc can figure out
	// the correct package name base path.
	if len(d.Javadoc.properties.Local_sourcepaths) > 0 {
		flags.sourcepathArgs = "-sourcepath " + strings.Join(d.Javadoc.sourcepaths.Strings(), ":")
	} else {
		flags.sourcepathArgs = "-sourcepath " + android.PathForModuleOut(ctx, "srcjars").String()
	}

	return flags, nil
}

func (d *Droiddoc) collectDoclavaDocsFlags(ctx android.ModuleContext, implicits *android.Paths,
	jsilver, doclava android.Path) string {

	*implicits = append(*implicits, jsilver)
	*implicits = append(*implicits, doclava)

	var date string
	if runtime.GOOS == "darwin" {
		date = `date -r`
	} else {
		date = `date -d`
	}

	// Droiddoc always gets "-source 1.8" because it doesn't support 1.9 sources.  For modules with 1.9
	// sources, droiddoc will get sources produced by metalava which will have already stripped out the
	// 1.9 language features.
	args := " -source 1.8 -J-Xmx1600m -J-XX:-OmitStackTraceInFastThrow -XDignore.symbol.file " +
		"-doclet com.google.doclava.Doclava -docletpath " + jsilver.String() + ":" + doclava.String() + " " +
		"-hdf page.build " + ctx.Config().BuildId() + "-" + ctx.Config().BuildNumberFromFile() + " " +
		`-hdf page.now "$$(` + date + ` @$$(cat ` + ctx.Config().Getenv("BUILD_DATETIME_FILE") + `) "+%d %b %Y %k:%M")" `

	if String(d.properties.Custom_template) == "" {
		// TODO: This is almost always droiddoc-templates-sdk
		ctx.PropertyErrorf("custom_template", "must specify a template")
	}

	ctx.VisitDirectDepsWithTag(droiddocTemplateTag, func(m android.Module) {
		if t, ok := m.(*ExportedDroiddocDir); ok {
			*implicits = append(*implicits, t.deps...)
			args = args + " -templatedir " + t.dir.String()
		} else {
			ctx.PropertyErrorf("custom_template", "module %q is not a droiddoc_template", ctx.OtherModuleName(m))
		}
	})

	if len(d.properties.Html_dirs) > 0 {
		htmlDir := d.properties.Html_dirs[0]
		*implicits = append(*implicits, android.PathsForModuleSrc(ctx, []string{filepath.Join(d.properties.Html_dirs[0], "**/*")})...)
		args = args + " -htmldir " + htmlDir
	}

	if len(d.properties.Html_dirs) > 1 {
		htmlDir2 := d.properties.Html_dirs[1]
		*implicits = append(*implicits, android.PathsForModuleSrc(ctx, []string{filepath.Join(htmlDir2, "**/*")})...)
		args = args + " -htmldir2 " + htmlDir2
	}

	if len(d.properties.Html_dirs) > 2 {
		ctx.PropertyErrorf("html_dirs", "Droiddoc only supports up to 2 html dirs")
	}

	knownTags := android.PathsForModuleSrc(ctx, d.properties.Knowntags)
	*implicits = append(*implicits, knownTags...)

	for _, kt := range knownTags {
		args = args + " -knowntags " + kt.String()
	}

	for _, hdf := range d.properties.Hdf {
		args = args + " -hdf " + hdf
	}

	if String(d.properties.Proofread_file) != "" {
		proofreadFile := android.PathForModuleOut(ctx, String(d.properties.Proofread_file))
		args = args + " -proofread " + proofreadFile.String()
	}

	if String(d.properties.Todo_file) != "" {
		// tricky part:
		// we should not compute full path for todo_file through PathForModuleOut().
		// the non-standard doclet will get the full path relative to "-o".
		args = args + " -todo " + String(d.properties.Todo_file)
	}

	if String(d.properties.Resourcesdir) != "" {
		// TODO: should we add files under resourcesDir to the implicits? It seems that
		// resourcesDir is one sub dir of htmlDir
		resourcesDir := android.PathForModuleSrc(ctx, String(d.properties.Resourcesdir))
		args = args + " -resourcesdir " + resourcesDir.String()
	}

	if String(d.properties.Resourcesoutdir) != "" {
		// TODO: it seems -resourceoutdir reference/android/images/ didn't get generated anywhere.
		args = args + " -resourcesoutdir " + String(d.properties.Resourcesoutdir)
	}
	return args
}

func (d *Droiddoc) collectStubsFlags(ctx android.ModuleContext,
	implicitOutputs *android.WritablePaths) string {
	var doclavaFlags string
	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") ||
		apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") ||
		String(d.properties.Api_filename) != "" {
		d.apiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_api.txt")
		doclavaFlags += " -api " + d.apiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.apiFile)
		d.apiFilePath = d.apiFile
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") ||
		apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") ||
		String(d.properties.Removed_api_filename) != "" {
		d.removedApiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_removed.txt")
		doclavaFlags += " -removedApi " + d.removedApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.removedApiFile)
	}

	if String(d.properties.Private_api_filename) != "" {
		d.privateApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_api_filename))
		doclavaFlags += " -privateApi " + d.privateApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.privateApiFile)
	}

	if String(d.properties.Dex_api_filename) != "" {
		d.dexApiFile = android.PathForModuleOut(ctx, String(d.properties.Dex_api_filename))
		doclavaFlags += " -dexApi " + d.dexApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.dexApiFile)
	}

	if String(d.properties.Private_dex_api_filename) != "" {
		d.privateDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_dex_api_filename))
		doclavaFlags += " -privateDexApi " + d.privateDexApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.privateDexApiFile)
	}

	if String(d.properties.Removed_dex_api_filename) != "" {
		d.removedDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Removed_dex_api_filename))
		doclavaFlags += " -removedDexApi " + d.removedDexApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.removedDexApiFile)
	}

	if String(d.properties.Exact_api_filename) != "" {
		d.exactApiFile = android.PathForModuleOut(ctx, String(d.properties.Exact_api_filename))
		doclavaFlags += " -exactApi " + d.exactApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.exactApiFile)
	}

	if String(d.properties.Dex_mapping_filename) != "" {
		d.apiMappingFile = android.PathForModuleOut(ctx, String(d.properties.Dex_mapping_filename))
		doclavaFlags += " -apiMapping " + d.apiMappingFile.String()
		*implicitOutputs = append(*implicitOutputs, d.apiMappingFile)
	}

	if String(d.properties.Proguard_filename) != "" {
		d.proguardFile = android.PathForModuleOut(ctx, String(d.properties.Proguard_filename))
		doclavaFlags += " -proguard " + d.proguardFile.String()
		*implicitOutputs = append(*implicitOutputs, d.proguardFile)
	}

	if BoolDefault(d.properties.Create_stubs, true) {
		doclavaFlags += " -stubs " + android.PathForModuleOut(ctx, "stubsDir").String()
	}

	if Bool(d.properties.Write_sdk_values) {
		doclavaFlags += " -sdkvalues " + android.PathForModuleOut(ctx, "out").String()
	}

	return doclavaFlags
}

func (d *Droiddoc) getPostDoclavaCmds(ctx android.ModuleContext, implicits *android.Paths) string {
	var cmds string
	if String(d.properties.Static_doc_index_redirect) != "" {
		static_doc_index_redirect := ctx.ExpandSource(String(d.properties.Static_doc_index_redirect),
			"static_doc_index_redirect")
		*implicits = append(*implicits, static_doc_index_redirect)
		cmds = cmds + " && cp " + static_doc_index_redirect.String() + " " +
			android.PathForModuleOut(ctx, "out", "index.html").String()
	}

	if String(d.properties.Static_doc_properties) != "" {
		static_doc_properties := ctx.ExpandSource(String(d.properties.Static_doc_properties),
			"static_doc_properties")
		*implicits = append(*implicits, static_doc_properties)
		cmds = cmds + " && cp " + static_doc_properties.String() + " " +
			android.PathForModuleOut(ctx, "out", "source.properties").String()
	}
	return cmds
}

func (d *Droiddoc) transformDoclava(ctx android.ModuleContext, implicits android.Paths,
	implicitOutputs android.WritablePaths,
	bootclasspathArgs, classpathArgs, sourcepathArgs, opts, postDoclavaCmds string) {
	ctx.Build(pctx, android.BuildParams{
		Rule:            javadoc,
		Description:     "Doclava",
		Output:          d.Javadoc.stubsSrcJar,
		Inputs:          d.Javadoc.srcFiles,
		Implicits:       implicits,
		ImplicitOutputs: implicitOutputs,
		Args: map[string]string{
			"outDir":            android.PathForModuleOut(ctx, "out").String(),
			"srcJarDir":         android.PathForModuleOut(ctx, "srcjars").String(),
			"stubsDir":          android.PathForModuleOut(ctx, "stubsDir").String(),
			"srcJars":           strings.Join(d.Javadoc.srcJars.Strings(), " "),
			"opts":              opts,
			"bootclasspathArgs": bootclasspathArgs,
			"classpathArgs":     classpathArgs,
			"sourcepathArgs":    sourcepathArgs,
			"docZip":            d.Javadoc.docZip.String(),
			"postDoclavaCmds":   postDoclavaCmds,
		},
	})
}

func (d *Droiddoc) transformCheckApi(ctx android.ModuleContext, apiFile, removedApiFile android.Path,
	checkApiClasspath classpath, msg, opts string, output android.WritablePath) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        apiCheck,
		Description: "Doclava Check API",
		Output:      output,
		Inputs:      nil,
		Implicits: append(android.Paths{apiFile, removedApiFile, d.apiFile, d.removedApiFile},
			checkApiClasspath...),
		Args: map[string]string{
			"msg":                   msg,
			"classpath":             checkApiClasspath.FormJavaClassPath(""),
			"opts":                  opts,
			"apiFile":               apiFile.String(),
			"apiFileToCheck":        d.apiFile.String(),
			"removedApiFile":        removedApiFile.String(),
			"removedApiFileToCheck": d.removedApiFile.String(),
		},
	})
}

func (d *Droiddoc) transformDokka(ctx android.ModuleContext, implicits android.Paths,
	classpathArgs, opts string) {
	ctx.Build(pctx, android.BuildParams{
		Rule:        dokka,
		Description: "Dokka",
		Output:      d.Javadoc.stubsSrcJar,
		Inputs:      d.Javadoc.srcFiles,
		Implicits:   implicits,
		Args: map[string]string{
			"outDir":        android.PathForModuleOut(ctx, "dokka-out").String(),
			"srcJarDir":     android.PathForModuleOut(ctx, "dokka-srcjars").String(),
			"stubsDir":      android.PathForModuleOut(ctx, "dokka-stubsDir").String(),
			"srcJars":       strings.Join(d.Javadoc.srcJars.Strings(), " "),
			"classpathArgs": classpathArgs,
			"opts":          opts,
			"docZip":        d.Javadoc.docZip.String(),
		},
	})
}

func (d *Droiddoc) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	deps := d.Javadoc.collectDeps(ctx)

	jsilver := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "jsilver.jar")
	doclava := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "doclava.jar")
	java8Home := ctx.Config().Getenv("ANDROID_JAVA8_HOME")
	checkApiClasspath := classpath{jsilver, doclava, android.PathForSource(ctx, java8Home, "lib/tools.jar")}

	var implicits android.Paths
	implicits = append(implicits, d.Javadoc.srcJars...)
	implicits = append(implicits, d.Javadoc.argFiles...)

	var implicitOutputs android.WritablePaths
	implicitOutputs = append(implicitOutputs, d.Javadoc.docZip)
	for _, o := range d.Javadoc.properties.Out {
		implicitOutputs = append(implicitOutputs, android.PathForModuleGen(ctx, o))
	}

	flags, err := d.initBuilderFlags(ctx, &implicits, deps)
	if err != nil {
		return
	}

	flags.doclavaStubsFlags = d.collectStubsFlags(ctx, &implicitOutputs)
	if Bool(d.properties.Dokka_enabled) {
		d.transformDokka(ctx, implicits, flags.classpathArgs, d.Javadoc.args)
	} else {
		flags.doclavaDocsFlags = d.collectDoclavaDocsFlags(ctx, &implicits, jsilver, doclava)
		flags.postDoclavaCmds = d.getPostDoclavaCmds(ctx, &implicits)
		d.transformDoclava(ctx, implicits, implicitOutputs, flags.bootClasspathArgs, flags.classpathArgs,
			flags.sourcepathArgs, flags.doclavaDocsFlags+flags.doclavaStubsFlags+" "+d.Javadoc.args,
			flags.postDoclavaCmds)
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") &&
		!ctx.Config().IsPdkBuild() {
		apiFile := ctx.ExpandSource(String(d.properties.Check_api.Current.Api_file),
			"check_api.current.api_file")
		removedApiFile := ctx.ExpandSource(String(d.properties.Check_api.Current.Removed_api_file),
			"check_api.current_removed_api_file")

		d.checkCurrentApiTimestamp = android.PathForModuleOut(ctx, "check_current_api.timestamp")
		d.transformCheckApi(ctx, apiFile, removedApiFile, checkApiClasspath,
			fmt.Sprintf(`\n******************************\n`+
				`You have tried to change the API from what has been previously approved.\n\n`+
				`To make these errors go away, you have two choices:\n`+
				`   1. You can add '@hide' javadoc comments to the methods, etc. listed in the\n`+
				`      errors above.\n\n`+
				`   2. You can update current.txt by executing the following command:\n`+
				`         make %s-update-current-api\n\n`+
				`      To submit the revised current.txt to the main Android repository,\n`+
				`      you will need approval.\n`+
				`******************************\n`, ctx.ModuleName()), String(d.properties.Check_api.Current.Args),
			d.checkCurrentApiTimestamp)

		d.updateCurrentApiTimestamp = android.PathForModuleOut(ctx, "update_current_api.timestamp")
		transformUpdateApi(ctx, apiFile, removedApiFile, d.apiFile, d.removedApiFile,
			d.updateCurrentApiTimestamp)
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") &&
		!ctx.Config().IsPdkBuild() {
		apiFile := ctx.ExpandSource(String(d.properties.Check_api.Last_released.Api_file),
			"check_api.last_released.api_file")
		removedApiFile := ctx.ExpandSource(String(d.properties.Check_api.Last_released.Removed_api_file),
			"check_api.last_released.removed_api_file")

		d.checkLastReleasedApiTimestamp = android.PathForModuleOut(ctx, "check_last_released_api.timestamp")
		d.transformCheckApi(ctx, apiFile, removedApiFile, checkApiClasspath,
			`\n******************************\n`+
				`You have tried to change the API from what has been previously released in\n`+
				`an SDK.  Please fix the errors listed above.\n`+
				`******************************\n`, String(d.properties.Check_api.Last_released.Args),
			d.checkLastReleasedApiTimestamp)
	}
}

//
// Droidstubs
//
type Droidstubs struct {
	Javadoc

	properties              DroidstubsProperties
	apiFile                 android.WritablePath
	apiXmlFile              android.WritablePath
	lastReleasedApiXmlFile  android.WritablePath
	dexApiFile              android.WritablePath
	privateApiFile          android.WritablePath
	privateDexApiFile       android.WritablePath
	removedApiFile          android.WritablePath
	removedDexApiFile       android.WritablePath
	apiMappingFile          android.WritablePath
	exactApiFile            android.WritablePath
	proguardFile            android.WritablePath
	nullabilityWarningsFile android.WritablePath

	checkCurrentApiTimestamp      android.WritablePath
	updateCurrentApiTimestamp     android.WritablePath
	checkLastReleasedApiTimestamp android.WritablePath

	checkNullabilityWarningsTimestamp android.WritablePath

	annotationsZip android.WritablePath
	apiVersionsXml android.WritablePath

	apiFilePath android.Path

	jdiffDocZip      android.WritablePath
	jdiffStubsSrcJar android.WritablePath

	metadataZip android.WritablePath
	metadataDir android.WritablePath
}

func DroidstubsFactory() android.Module {
	module := &Droidstubs{}

	module.AddProperties(&module.properties,
		&module.Javadoc.properties)

	InitDroiddocModule(module, android.HostAndDeviceSupported)
	return module
}

func DroidstubsHostFactory() android.Module {
	module := &Droidstubs{}

	module.AddProperties(&module.properties,
		&module.Javadoc.properties)

	InitDroiddocModule(module, android.HostSupported)
	return module
}

func (d *Droidstubs) ApiFilePath() android.Path {
	return d.apiFilePath
}

func (d *Droidstubs) DepsMutator(ctx android.BottomUpMutatorContext) {
	d.Javadoc.addDeps(ctx)

	if Bool(d.properties.Check_api.Ignore_missing_latest_api) {
		ignoreMissingModules(ctx, &d.properties.Check_api.Last_released)
	}

	if len(d.properties.Merge_annotations_dirs) != 0 {
		for _, mergeAnnotationsDir := range d.properties.Merge_annotations_dirs {
			ctx.AddDependency(ctx.Module(), metalavaMergeAnnotationsDirTag, mergeAnnotationsDir)
		}
	}

	if len(d.properties.Merge_inclusion_annotations_dirs) != 0 {
		for _, mergeInclusionAnnotationsDir := range d.properties.Merge_inclusion_annotations_dirs {
			ctx.AddDependency(ctx.Module(), metalavaMergeInclusionAnnotationsDirTag, mergeInclusionAnnotationsDir)
		}
	}

	if len(d.properties.Api_levels_annotations_dirs) != 0 {
		for _, apiLevelsAnnotationsDir := range d.properties.Api_levels_annotations_dirs {
			ctx.AddDependency(ctx.Module(), metalavaAPILevelsAnnotationsDirTag, apiLevelsAnnotationsDir)
		}
	}
}

func (d *Droidstubs) initBuilderFlags(ctx android.ModuleContext, implicits *android.Paths,
	deps deps) (droiddocBuilderFlags, error) {
	var flags droiddocBuilderFlags

	*implicits = append(*implicits, deps.bootClasspath...)
	*implicits = append(*implicits, deps.classpath...)

	// continue to use -bootclasspath even if Metalava under -source 1.9 is enabled
	// since it doesn't support system modules yet.
	if len(deps.bootClasspath.Strings()) > 0 {
		// For OpenJDK 8 we can use -bootclasspath to define the core libraries code.
		flags.bootClasspathArgs = deps.bootClasspath.FormJavaClassPath("-bootclasspath")
	}
	flags.classpathArgs = deps.classpath.FormJavaClassPath("-classpath")

	flags.sourcepathArgs = "-sourcepath \"" + strings.Join(d.Javadoc.sourcepaths.Strings(), ":") + "\""
	return flags, nil
}

func (d *Droidstubs) collectStubsFlags(ctx android.ModuleContext,
	implicitOutputs *android.WritablePaths) string {
	var metalavaFlags string
	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") ||
		apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") ||
		String(d.properties.Api_filename) != "" {
		d.apiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_api.txt")
		metalavaFlags = metalavaFlags + " --api " + d.apiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.apiFile)
		d.apiFilePath = d.apiFile
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") ||
		apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") ||
		String(d.properties.Removed_api_filename) != "" {
		d.removedApiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_removed.txt")
		metalavaFlags = metalavaFlags + " --removed-api " + d.removedApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.removedApiFile)
	}

	if String(d.properties.Private_api_filename) != "" {
		d.privateApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_api_filename))
		metalavaFlags = metalavaFlags + " --private-api " + d.privateApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.privateApiFile)
	}

	if String(d.properties.Dex_api_filename) != "" {
		d.dexApiFile = android.PathForModuleOut(ctx, String(d.properties.Dex_api_filename))
		metalavaFlags += " --dex-api " + d.dexApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.dexApiFile)
	}

	if String(d.properties.Private_dex_api_filename) != "" {
		d.privateDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_dex_api_filename))
		metalavaFlags = metalavaFlags + " --private-dex-api " + d.privateDexApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.privateDexApiFile)
	}

	if String(d.properties.Removed_dex_api_filename) != "" {
		d.removedDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Removed_dex_api_filename))
		metalavaFlags = metalavaFlags + " --removed-dex-api " + d.removedDexApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.removedDexApiFile)
	}

	if String(d.properties.Exact_api_filename) != "" {
		d.exactApiFile = android.PathForModuleOut(ctx, String(d.properties.Exact_api_filename))
		metalavaFlags = metalavaFlags + " --exact-api " + d.exactApiFile.String()
		*implicitOutputs = append(*implicitOutputs, d.exactApiFile)
	}

	if String(d.properties.Dex_mapping_filename) != "" {
		d.apiMappingFile = android.PathForModuleOut(ctx, String(d.properties.Dex_mapping_filename))
		metalavaFlags = metalavaFlags + " --dex-api-mapping " + d.apiMappingFile.String()
		*implicitOutputs = append(*implicitOutputs, d.apiMappingFile)
	}

	if String(d.properties.Proguard_filename) != "" {
		d.proguardFile = android.PathForModuleOut(ctx, String(d.properties.Proguard_filename))
		metalavaFlags += " --proguard " + d.proguardFile.String()
		*implicitOutputs = append(*implicitOutputs, d.proguardFile)
	}

	if Bool(d.properties.Write_sdk_values) {
		d.metadataDir = android.PathForModuleOut(ctx, "metadata")
		metalavaFlags = metalavaFlags + " --sdk-values " + d.metadataDir.String()
	}

	if Bool(d.properties.Create_doc_stubs) {
		metalavaFlags += " --doc-stubs " + android.PathForModuleOut(ctx, "stubsDir").String()
	} else {
		metalavaFlags += " --stubs " + android.PathForModuleOut(ctx, "stubsDir").String()
	}
	return metalavaFlags
}

func (d *Droidstubs) collectAnnotationsFlags(ctx android.ModuleContext,
	implicits *android.Paths, implicitOutputs *android.WritablePaths) (string, string) {
	var flags, mergeAnnoDirFlags string
	if Bool(d.properties.Annotations_enabled) {
		flags += " --include-annotations"
		validatingNullability :=
			strings.Contains(d.Javadoc.args, "--validate-nullability-from-merged-stubs") ||
				String(d.properties.Validate_nullability_from_list) != ""
		migratingNullability := String(d.properties.Previous_api) != ""
		if !(migratingNullability || validatingNullability) {
			ctx.PropertyErrorf("previous_api",
				"has to be non-empty if annotations was enabled (unless validating nullability)")
		}
		if migratingNullability {
			previousApi := android.PathForModuleSrc(ctx, String(d.properties.Previous_api))
			*implicits = append(*implicits, previousApi)
			flags += " --migrate-nullness " + previousApi.String()
		}
		if s := String(d.properties.Validate_nullability_from_list); s != "" {
			flags += " --validate-nullability-from-list " + android.PathForModuleSrc(ctx, s).String()
		}
		if validatingNullability {
			d.nullabilityWarningsFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_nullability_warnings.txt")
			*implicitOutputs = append(*implicitOutputs, d.nullabilityWarningsFile)
			flags += " --nullability-warnings-txt " + d.nullabilityWarningsFile.String()
		}

		d.annotationsZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"_annotations.zip")
		*implicitOutputs = append(*implicitOutputs, d.annotationsZip)

		flags += " --extract-annotations " + d.annotationsZip.String()

		if len(d.properties.Merge_annotations_dirs) == 0 {
			ctx.PropertyErrorf("merge_annotations_dirs",
				"has to be non-empty if annotations was enabled!")
		}
		ctx.VisitDirectDepsWithTag(metalavaMergeAnnotationsDirTag, func(m android.Module) {
			if t, ok := m.(*ExportedDroiddocDir); ok {
				*implicits = append(*implicits, t.deps...)
				mergeAnnoDirFlags += " --merge-qualifier-annotations " + t.dir.String()
			} else {
				ctx.PropertyErrorf("merge_annotations_dirs",
					"module %q is not a metalava merge-annotations dir", ctx.OtherModuleName(m))
			}
		})
		flags += mergeAnnoDirFlags
		// TODO(tnorbye): find owners to fix these warnings when annotation was enabled.
		flags += " --hide HiddenTypedefConstant --hide SuperfluousPrefix --hide AnnotationExtraction"
	}

	return flags, mergeAnnoDirFlags
}

func (d *Droidstubs) collectInclusionAnnotationsFlags(ctx android.ModuleContext,
	implicits *android.Paths, implicitOutputs *android.WritablePaths) string {
	var flags string
	ctx.VisitDirectDepsWithTag(metalavaMergeInclusionAnnotationsDirTag, func(m android.Module) {
		if t, ok := m.(*ExportedDroiddocDir); ok {
			*implicits = append(*implicits, t.deps...)
			flags += " --merge-inclusion-annotations " + t.dir.String()
		} else {
			ctx.PropertyErrorf("merge_inclusion_annotations_dirs",
				"module %q is not a metalava merge-annotations dir", ctx.OtherModuleName(m))
		}
	})

	return flags
}

func (d *Droidstubs) collectAPILevelsAnnotationsFlags(ctx android.ModuleContext,
	implicits *android.Paths, implicitOutputs *android.WritablePaths) string {
	var flags string
	if Bool(d.properties.Api_levels_annotations_enabled) {
		d.apiVersionsXml = android.PathForModuleOut(ctx, "api-versions.xml")
		*implicitOutputs = append(*implicitOutputs, d.apiVersionsXml)

		if len(d.properties.Api_levels_annotations_dirs) == 0 {
			ctx.PropertyErrorf("api_levels_annotations_dirs",
				"has to be non-empty if api levels annotations was enabled!")
		}

		flags = " --generate-api-levels " + d.apiVersionsXml.String() + " --apply-api-levels " +
			d.apiVersionsXml.String() + " --current-version " + ctx.Config().PlatformSdkVersion() +
			" --current-codename " + ctx.Config().PlatformSdkCodename() + " "

		ctx.VisitDirectDepsWithTag(metalavaAPILevelsAnnotationsDirTag, func(m android.Module) {
			if t, ok := m.(*ExportedDroiddocDir); ok {
				var androidJars android.Paths
				for _, dep := range t.deps {
					if strings.HasSuffix(dep.String(), "android.jar") {
						androidJars = append(androidJars, dep)
					}
				}
				*implicits = append(*implicits, androidJars...)
				flags += " --android-jar-pattern " + t.dir.String() + "/%/public/android.jar "
			} else {
				ctx.PropertyErrorf("api_levels_annotations_dirs",
					"module %q is not a metalava api-levels-annotations dir", ctx.OtherModuleName(m))
			}
		})

	}

	return flags
}

func (d *Droidstubs) collectApiToXmlFlags(ctx android.ModuleContext, implicits *android.Paths,
	implicitOutputs *android.WritablePaths) string {
	var flags string
	if Bool(d.properties.Jdiff_enabled) && !ctx.Config().IsPdkBuild() {
		if d.apiFile.String() == "" {
			ctx.ModuleErrorf("API signature file has to be specified in Metalava when jdiff is enabled.")
		}

		d.apiXmlFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_api.xml")
		*implicitOutputs = append(*implicitOutputs, d.apiXmlFile)

		flags = " --api-xml " + d.apiXmlFile.String()

		if String(d.properties.Check_api.Last_released.Api_file) == "" {
			ctx.PropertyErrorf("check_api.last_released.api_file",
				"has to be non-empty if jdiff was enabled!")
		}
		lastReleasedApi := ctx.ExpandSource(String(d.properties.Check_api.Last_released.Api_file),
			"check_api.last_released.api_file")
		*implicits = append(*implicits, lastReleasedApi)

		d.lastReleasedApiXmlFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_last_released_api.xml")
		*implicitOutputs = append(*implicitOutputs, d.lastReleasedApiXmlFile)

		flags += " --convert-to-jdiff " + lastReleasedApi.String() + " " +
			d.lastReleasedApiXmlFile.String()
	}

	return flags
}

func (d *Droidstubs) transformMetalava(ctx android.ModuleContext, implicits android.Paths,
	implicitOutputs android.WritablePaths, javaVersion,
	bootclasspathArgs, classpathArgs, sourcepathArgs, opts string) {

	var writeSdkValues, metadataZip, metadataDir string
	if Bool(d.properties.Write_sdk_values) {
		writeSdkValues = "true"
		d.metadataZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"-metadata.zip")
		metadataZip = d.metadataZip.String()
		metadataDir = d.metadataDir.String()
		implicitOutputs = append(implicitOutputs, d.metadataZip)
	} else {
		writeSdkValues = "false"
		metadataZip = ""
		metadataDir = ""
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:            metalava,
		Description:     "Metalava",
		Output:          d.Javadoc.stubsSrcJar,
		Inputs:          d.Javadoc.srcFiles,
		Implicits:       implicits,
		ImplicitOutputs: implicitOutputs,
		Args: map[string]string{
			"outDir":            android.PathForModuleOut(ctx, "out").String(),
			"srcJarDir":         android.PathForModuleOut(ctx, "srcjars").String(),
			"stubsDir":          android.PathForModuleOut(ctx, "stubsDir").String(),
			"srcJars":           strings.Join(d.Javadoc.srcJars.Strings(), " "),
			"javaVersion":       javaVersion,
			"bootclasspathArgs": bootclasspathArgs,
			"classpathArgs":     classpathArgs,
			"sourcepathArgs":    sourcepathArgs,
			"opts":              opts,
			"writeSdkValues":    writeSdkValues,
			"metadataZip":       metadataZip,
			"metadataDir":       metadataDir,
		},
	})
}

func (d *Droidstubs) transformCheckApi(ctx android.ModuleContext,
	apiFile, removedApiFile android.Path, baselineFile android.OptionalPath, updatedBaselineOut android.WritablePath, implicits android.Paths,
	javaVersion, bootclasspathArgs, classpathArgs, sourcepathArgs, opts, subdir, msg string,
	output android.WritablePath) {

	implicits = append(android.Paths{apiFile, removedApiFile, d.apiFile, d.removedApiFile}, implicits...)
	var implicitOutputs android.WritablePaths

	if baselineFile.Valid() {
		implicits = append(implicits, baselineFile.Path())
		implicitOutputs = append(implicitOutputs, updatedBaselineOut)
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:            metalavaApiCheck,
		Description:     "Metalava Check API",
		Output:          output,
		Inputs:          d.Javadoc.srcFiles,
		Implicits:       implicits,
		ImplicitOutputs: implicitOutputs,
		Args: map[string]string{
			"srcJarDir":         android.PathForModuleOut(ctx, subdir, "srcjars").String(),
			"srcJars":           strings.Join(d.Javadoc.srcJars.Strings(), " "),
			"javaVersion":       javaVersion,
			"bootclasspathArgs": bootclasspathArgs,
			"classpathArgs":     classpathArgs,
			"sourcepathArgs":    sourcepathArgs,
			"opts":              opts,
			"msg":               msg,
		},
	})
}

func (d *Droidstubs) transformJdiff(ctx android.ModuleContext, implicits android.Paths,
	implicitOutputs android.WritablePaths,
	bootclasspathArgs, classpathArgs, sourcepathArgs, opts string) {
	ctx.Build(pctx, android.BuildParams{
		Rule:            javadoc,
		Description:     "Jdiff",
		Output:          d.jdiffStubsSrcJar,
		Inputs:          d.Javadoc.srcFiles,
		Implicits:       implicits,
		ImplicitOutputs: implicitOutputs,
		Args: map[string]string{
			"outDir":            android.PathForModuleOut(ctx, "jdiff-out").String(),
			"srcJarDir":         android.PathForModuleOut(ctx, "jdiff-srcjars").String(),
			"stubsDir":          android.PathForModuleOut(ctx, "jdiff-stubsDir").String(),
			"srcJars":           strings.Join(d.Javadoc.srcJars.Strings(), " "),
			"opts":              opts,
			"bootclasspathArgs": bootclasspathArgs,
			"classpathArgs":     classpathArgs,
			"sourcepathArgs":    sourcepathArgs,
			"docZip":            d.jdiffDocZip.String(),
		},
	})
}

func (d *Droidstubs) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	deps := d.Javadoc.collectDeps(ctx)

	javaVersion := getJavaVersion(ctx, String(d.Javadoc.properties.Java_version), sdkContext(d))

	var implicits android.Paths
	implicits = append(implicits, d.Javadoc.srcJars...)
	implicits = append(implicits, d.Javadoc.argFiles...)

	var implicitOutputs android.WritablePaths
	for _, o := range d.Javadoc.properties.Out {
		implicitOutputs = append(implicitOutputs, android.PathForModuleGen(ctx, o))
	}

	flags, err := d.initBuilderFlags(ctx, &implicits, deps)
	metalavaCheckApiImplicits := implicits
	jdiffImplicits := implicits

	if err != nil {
		return
	}

	flags.metalavaStubsFlags = d.collectStubsFlags(ctx, &implicitOutputs)
	flags.metalavaAnnotationsFlags, flags.metalavaMergeAnnoDirFlags =
		d.collectAnnotationsFlags(ctx, &implicits, &implicitOutputs)
	flags.metalavaInclusionAnnotationsFlags = d.collectInclusionAnnotationsFlags(ctx, &implicits, &implicitOutputs)
	flags.metalavaApiLevelsAnnotationsFlags = d.collectAPILevelsAnnotationsFlags(ctx, &implicits, &implicitOutputs)
	flags.metalavaApiToXmlFlags = d.collectApiToXmlFlags(ctx, &implicits, &implicitOutputs)

	if strings.Contains(d.Javadoc.args, "--generate-documentation") {
		// Currently Metalava have the ability to invoke Javadoc in a seperate process.
		// Pass "-nodocs" to suppress the Javadoc invocation when Metalava receives
		// "--generate-documentation" arg. This is not needed when Metalava removes this feature.
		d.Javadoc.args = d.Javadoc.args + " -nodocs "
	}
	d.transformMetalava(ctx, implicits, implicitOutputs, javaVersion,
		flags.bootClasspathArgs, flags.classpathArgs, flags.sourcepathArgs,
		flags.metalavaStubsFlags+flags.metalavaAnnotationsFlags+flags.metalavaInclusionAnnotationsFlags+
			flags.metalavaApiLevelsAnnotationsFlags+flags.metalavaApiToXmlFlags+" "+d.Javadoc.args)

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") &&
		!ctx.Config().IsPdkBuild() {
		apiFile := ctx.ExpandSource(String(d.properties.Check_api.Current.Api_file),
			"check_api.current.api_file")
		removedApiFile := ctx.ExpandSource(String(d.properties.Check_api.Current.Removed_api_file),
			"check_api.current_removed_api_file")
		baselineFile := ctx.ExpandOptionalSource(d.properties.Check_api.Current.Baseline_file,
			"check_api.current.baseline_file")

		d.checkCurrentApiTimestamp = android.PathForModuleOut(ctx, "check_current_api.timestamp")
		opts := " " + d.Javadoc.args + " --check-compatibility:api:current " + apiFile.String() +
			" --check-compatibility:removed:current " + removedApiFile.String() +
			flags.metalavaInclusionAnnotationsFlags + flags.metalavaMergeAnnoDirFlags + " "
		baselineOut := android.PathForModuleOut(ctx, "current_baseline.txt")
		if baselineFile.Valid() {
			opts = opts + "--baseline " + baselineFile.String() + " --update-baseline " + baselineOut.String() + " "
		}

		d.transformCheckApi(ctx, apiFile, removedApiFile, baselineFile, baselineOut, metalavaCheckApiImplicits,
			javaVersion, flags.bootClasspathArgs, flags.classpathArgs, flags.sourcepathArgs, opts, "current-apicheck",
			fmt.Sprintf(`\n******************************\n`+
				`You have tried to change the API from what has been previously approved.\n\n`+
				`To make these errors go away, you have two choices:\n`+
				`   1. You can add '@hide' javadoc comments to the methods, etc. listed in the\n`+
				`      errors above.\n\n`+
				`   2. You can update current.txt by executing the following command:\n`+
				`         make %s-update-current-api\n\n`+
				`      To submit the revised current.txt to the main Android repository,\n`+
				`      you will need approval.\n`+
				`******************************\n`, ctx.ModuleName()),
			d.checkCurrentApiTimestamp)

		d.updateCurrentApiTimestamp = android.PathForModuleOut(ctx, "update_current_api.timestamp")
		transformUpdateApi(ctx, apiFile, removedApiFile, d.apiFile, d.removedApiFile,
			d.updateCurrentApiTimestamp)
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") &&
		!ctx.Config().IsPdkBuild() {
		apiFile := ctx.ExpandSource(String(d.properties.Check_api.Last_released.Api_file),
			"check_api.last_released.api_file")
		removedApiFile := ctx.ExpandSource(String(d.properties.Check_api.Last_released.Removed_api_file),
			"check_api.last_released.removed_api_file")
		baselineFile := ctx.ExpandOptionalSource(d.properties.Check_api.Last_released.Baseline_file,
			"check_api.last_released.baseline_file")

		d.checkLastReleasedApiTimestamp = android.PathForModuleOut(ctx, "check_last_released_api.timestamp")
		opts := " " + d.Javadoc.args + " --check-compatibility:api:released " + apiFile.String() +
			flags.metalavaInclusionAnnotationsFlags + " --check-compatibility:removed:released " +
			removedApiFile.String() + flags.metalavaMergeAnnoDirFlags + " "
		baselineOut := android.PathForModuleOut(ctx, "last_released_baseline.txt")
		if baselineFile.Valid() {
			opts = opts + "--baseline " + baselineFile.String() + " --update-baseline " + baselineOut.String() + " "
		}

		d.transformCheckApi(ctx, apiFile, removedApiFile, baselineFile, baselineOut, metalavaCheckApiImplicits,
			javaVersion, flags.bootClasspathArgs, flags.classpathArgs, flags.sourcepathArgs, opts, "last-apicheck",
			`\n******************************\n`+
				`You have tried to change the API from what has been previously released in\n`+
				`an SDK.  Please fix the errors listed above.\n`+
				`******************************\n`,
			d.checkLastReleasedApiTimestamp)
	}

	if String(d.properties.Check_nullability_warnings) != "" {
		if d.nullabilityWarningsFile == nil {
			ctx.PropertyErrorf("check_nullability_warnings",
				"Cannot specify check_nullability_warnings unless validating nullability")
		}
		checkNullabilityWarnings := ctx.ExpandSource(String(d.properties.Check_nullability_warnings),
			"check_nullability_warnings")
		d.checkNullabilityWarningsTimestamp = android.PathForModuleOut(ctx, "check_nullability_warnings.timestamp")
		msg := fmt.Sprintf(`\n******************************\n`+
			`The warnings encountered during nullability annotation validation did\n`+
			`not match the checked in file of expected warnings. The diffs are shown\n`+
			`above. You have two options:\n`+
			`   1. Resolve the differences by editing the nullability annotations.\n`+
			`   2. Update the file of expected warnings by running:\n`+
			`         cp %s %s\n`+
			`       and submitting the updated file as part of your change.`,
			d.nullabilityWarningsFile, checkNullabilityWarnings)
		ctx.Build(pctx, android.BuildParams{
			Rule:        nullabilityWarningsCheck,
			Description: "Nullability Warnings Check",
			Output:      d.checkNullabilityWarningsTimestamp,
			Implicits:   android.Paths{checkNullabilityWarnings, d.nullabilityWarningsFile},
			Args: map[string]string{
				"expected": checkNullabilityWarnings.String(),
				"actual":   d.nullabilityWarningsFile.String(),
				"msg":      msg,
			},
		})
	}

	if Bool(d.properties.Jdiff_enabled) && !ctx.Config().IsPdkBuild() {

		// Please sync with android-api-council@ before making any changes for the name of jdiffDocZip below
		// since there's cron job downstream that fetch this .zip file periodically.
		// See b/116221385 for reference.
		d.jdiffDocZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"jdiff-docs.zip")
		d.jdiffStubsSrcJar = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"jdiff-stubs.srcjar")

		var jdiffImplicitOutputs android.WritablePaths
		jdiffImplicitOutputs = append(jdiffImplicitOutputs, d.jdiffDocZip)

		jdiff := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "jdiff.jar")
		jdiffImplicits = append(jdiffImplicits, android.Paths{jdiff, d.apiXmlFile, d.lastReleasedApiXmlFile}...)

		opts := " -encoding UTF-8 -source 1.8 -J-Xmx1600m -XDignore.symbol.file " +
			"-doclet jdiff.JDiff -docletpath " + jdiff.String() + " -quiet " +
			"-newapi " + strings.TrimSuffix(d.apiXmlFile.Base(), d.apiXmlFile.Ext()) +
			" -newapidir " + filepath.Dir(d.apiXmlFile.String()) +
			" -oldapi " + strings.TrimSuffix(d.lastReleasedApiXmlFile.Base(), d.lastReleasedApiXmlFile.Ext()) +
			" -oldapidir " + filepath.Dir(d.lastReleasedApiXmlFile.String())

		d.transformJdiff(ctx, jdiffImplicits, jdiffImplicitOutputs, flags.bootClasspathArgs, flags.classpathArgs,
			flags.sourcepathArgs, opts)
	}
}

//
// Exported Droiddoc Directory
//
var droiddocTemplateTag = dependencyTag{name: "droiddoc-template"}
var metalavaMergeAnnotationsDirTag = dependencyTag{name: "metalava-merge-annotations-dir"}
var metalavaMergeInclusionAnnotationsDirTag = dependencyTag{name: "metalava-merge-inclusion-annotations-dir"}
var metalavaAPILevelsAnnotationsDirTag = dependencyTag{name: "metalava-api-levels-annotations-dir"}

type ExportedDroiddocDirProperties struct {
	// path to the directory containing Droiddoc related files.
	Path *string
}

type ExportedDroiddocDir struct {
	android.ModuleBase

	properties ExportedDroiddocDirProperties

	deps android.Paths
	dir  android.Path
}

func ExportedDroiddocDirFactory() android.Module {
	module := &ExportedDroiddocDir{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

func (d *ExportedDroiddocDir) DepsMutator(android.BottomUpMutatorContext) {}

func (d *ExportedDroiddocDir) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	path := String(d.properties.Path)
	d.dir = android.PathForModuleSrc(ctx, path)
	d.deps = android.PathsForModuleSrc(ctx, []string{filepath.Join(path, "**/*")})
}

//
// Defaults
//
type DocDefaults struct {
	android.ModuleBase
	android.DefaultsModuleBase
}

func (*DocDefaults) GenerateAndroidBuildActions(ctx android.ModuleContext) {
}

func DocDefaultsFactory() android.Module {
	module := &DocDefaults{}

	module.AddProperties(
		&JavadocProperties{},
		&DroiddocProperties{},
	)

	android.InitDefaultsModule(module)

	return module
}

func StubsDefaultsFactory() android.Module {
	module := &DocDefaults{}

	module.AddProperties(
		&JavadocProperties{},
		&DroidstubsProperties{},
	)

	android.InitDefaultsModule(module)

	return module
}
