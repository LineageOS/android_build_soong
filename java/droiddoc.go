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
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/java/config"
)

func init() {
	RegisterDocsBuildComponents(android.InitRegistrationContext)
	RegisterStubsBuildComponents(android.InitRegistrationContext)

	// Register sdk member type.
	android.RegisterSdkMemberType(&droidStubsSdkMemberType{
		SdkMemberTypeBase: android.SdkMemberTypeBase{
			PropertyName: "stubs_sources",
			// stubs_sources can be used with sdk to provide the source stubs for APIs provided by
			// the APEX.
			SupportsSdk: true,
		},
	})
}

func RegisterDocsBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("doc_defaults", DocDefaultsFactory)

	ctx.RegisterModuleType("droiddoc", DroiddocFactory)
	ctx.RegisterModuleType("droiddoc_host", DroiddocHostFactory)
	ctx.RegisterModuleType("droiddoc_exported_dir", ExportedDroiddocDirFactory)
	ctx.RegisterModuleType("javadoc", JavadocFactory)
	ctx.RegisterModuleType("javadoc_host", JavadocHostFactory)
}

func RegisterStubsBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("stubs_defaults", StubsDefaultsFactory)

	ctx.RegisterModuleType("droidstubs", DroidstubsFactory)
	ctx.RegisterModuleType("droidstubs_host", DroidstubsHostFactory)

	ctx.RegisterModuleType("prebuilt_stubs_sources", PrebuiltStubsSourcesFactory)
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

	// list of package names that should actually be used. If this property is left unspecified,
	// all the sources from the srcs property is used.
	Filter_packages []string

	// list of java libraries that will be in the classpath.
	Libs []string `android:"arch_variant"`

	// If set to false, don't allow this module(-docs.zip) to be exported. Defaults to true.
	Installable *bool

	// if not blank, set to the version of the sdk to compile against.
	// Defaults to compiling against the current platform.
	Sdk_version *string `android:"arch_variant"`

	// When targeting 1.9 and above, override the modules to use with --system,
	// otherwise provides defaults libraries to add to the bootclasspath.
	// Defaults to "none"
	System_modules *string

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
	//  $$: a literal $
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
	Proofread_file *string

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

	// Compat config XML. Generates compat change documentation if set.
	Compat_config *string `android:"path"`
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

		Api_lint struct {
			Enabled *bool

			// If set, performs api_lint on any new APIs not found in the given signature file
			New_since *string `android:"path"`

			// If not blank, path to the baseline txt file for approved API lint violations.
			Baseline_file *string `android:"path"`
		}
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

func (j *Javadoc) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{j.stubsSrcJar}, nil
	case ".docs.zip":
		return android.Paths{j.docZip}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

// javadoc converts .java source files to documentation using javadoc.
func JavadocFactory() android.Module {
	module := &Javadoc{}

	module.AddProperties(&module.properties)

	InitDroiddocModule(module, android.HostAndDeviceSupported)
	return module
}

// javadoc_host converts .java source files to documentation using javadoc.
func JavadocHostFactory() android.Module {
	module := &Javadoc{}

	module.AddProperties(&module.properties)

	InitDroiddocModule(module, android.HostSupported)
	return module
}

var _ android.OutputFileProducer = (*Javadoc)(nil)

func (j *Javadoc) sdkVersion() sdkSpec {
	return sdkSpecFrom(String(j.properties.Sdk_version))
}

func (j *Javadoc) systemModules() string {
	return proptools.String(j.properties.System_modules)
}

func (j *Javadoc) minSdkVersion() sdkSpec {
	return j.sdkVersion()
}

func (j *Javadoc) targetSdkVersion() sdkSpec {
	return j.sdkVersion()
}

func (j *Javadoc) addDeps(ctx android.BottomUpMutatorContext) {
	if ctx.Device() {
		sdkDep := decodeSdkDep(ctx, sdkContext(j))
		if sdkDep.useDefaultLibs {
			ctx.AddVariationDependencies(nil, bootClasspathTag, config.DefaultBootclasspathLibraries...)
			ctx.AddVariationDependencies(nil, systemModulesTag, config.DefaultSystemModules)
			if sdkDep.hasFrameworkLibs() {
				ctx.AddVariationDependencies(nil, libTag, config.DefaultLibraries...)
			}
		} else if sdkDep.useModule {
			ctx.AddVariationDependencies(nil, bootClasspathTag, sdkDep.bootclasspath...)
			ctx.AddVariationDependencies(nil, systemModulesTag, sdkDep.systemModules)
			ctx.AddVariationDependencies(nil, java9LibTag, sdkDep.java9Classpath...)
		}
	}

	ctx.AddVariationDependencies(nil, libTag, j.properties.Libs...)
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

// TODO: remove the duplication between this and the one in gen.go
func (j *Javadoc) genSources(ctx android.ModuleContext, srcFiles android.Paths,
	flags droiddocBuilderFlags) android.Paths {

	outSrcFiles := make(android.Paths, 0, len(srcFiles))
	var aidlSrcs android.Paths

	aidlIncludeFlags := genAidlIncludeFlags(srcFiles)

	for _, srcFile := range srcFiles {
		switch srcFile.Ext() {
		case ".aidl":
			aidlSrcs = append(aidlSrcs, srcFile)
		case ".logtags":
			javaFile := genLogtags(ctx, srcFile)
			outSrcFiles = append(outSrcFiles, javaFile)
		default:
			outSrcFiles = append(outSrcFiles, srcFile)
		}
	}

	// Process all aidl files together to support sharding them into one or more rules that produce srcjars.
	if len(aidlSrcs) > 0 {
		srcJarFiles := genAidl(ctx, aidlSrcs, flags.aidlFlags+aidlIncludeFlags, flags.aidlDeps)
		outSrcFiles = append(outSrcFiles, srcJarFiles...)
	}

	return outSrcFiles
}

func (j *Javadoc) collectDeps(ctx android.ModuleContext) deps {
	var deps deps

	sdkDep := decodeSdkDep(ctx, sdkContext(j))
	if sdkDep.invalidVersion {
		ctx.AddMissingDependencies(sdkDep.bootclasspath)
		ctx.AddMissingDependencies(sdkDep.java9Classpath)
	} else if sdkDep.useFiles {
		deps.bootClasspath = append(deps.bootClasspath, sdkDep.jars...)
		deps.aidlPreprocess = sdkDep.aidl
	} else {
		deps.aidlPreprocess = sdkDep.aidl
	}

	ctx.VisitDirectDeps(func(module android.Module) {
		otherName := ctx.OtherModuleName(module)
		tag := ctx.OtherModuleDependencyTag(module)

		switch tag {
		case bootClasspathTag:
			if dep, ok := module.(Dependency); ok {
				deps.bootClasspath = append(deps.bootClasspath, dep.ImplementationJars()...)
			} else if sm, ok := module.(SystemModulesProvider); ok {
				// A system modules dependency has been added to the bootclasspath
				// so add its libs to the bootclasspath.
				deps.bootClasspath = append(deps.bootClasspath, sm.HeaderJars()...)
			} else {
				panic(fmt.Errorf("unknown dependency %q for %q", otherName, ctx.ModuleName()))
			}
		case libTag:
			switch dep := module.(type) {
			case SdkLibraryDependency:
				deps.classpath = append(deps.classpath, dep.SdkImplementationJars(ctx, j.sdkVersion())...)
			case Dependency:
				deps.classpath = append(deps.classpath, dep.HeaderJars()...)
				deps.aidlIncludeDirs = append(deps.aidlIncludeDirs, dep.AidlIncludeDirs()...)
			case android.SourceFileProducer:
				checkProducesJars(ctx, dep)
				deps.classpath = append(deps.classpath, dep.Srcs()...)
			default:
				ctx.ModuleErrorf("depends on non-java module %q", otherName)
			}
		case java9LibTag:
			switch dep := module.(type) {
			case Dependency:
				deps.java9Classpath = append(deps.java9Classpath, dep.HeaderJars()...)
			default:
				ctx.ModuleErrorf("depends on non-java module %q", otherName)
			}
		case systemModulesTag:
			if deps.systemModules != nil {
				panic("Found two system module dependencies")
			}
			sm := module.(SystemModulesProvider)
			outputDir, outputDeps := sm.OutputDirAndDeps()
			deps.systemModules = &systemModules{outputDir, outputDeps}
		}
	})
	// do not pass exclude_srcs directly when expanding srcFiles since exclude_srcs
	// may contain filegroup or genrule.
	srcFiles := android.PathsForModuleSrcExcludes(ctx, j.properties.Srcs, j.properties.Exclude_srcs)

	filterByPackage := func(srcs []android.Path, filterPackages []string) []android.Path {
		if filterPackages == nil {
			return srcs
		}
		filtered := []android.Path{}
		for _, src := range srcs {
			if src.Ext() != ".java" {
				// Don't filter-out non-Java (=generated sources) by package names. This is not ideal,
				// but otherwise metalava emits stub sources having references to the generated AIDL classes
				// in filtered-out pacages (e.g. com.android.internal.*).
				// TODO(b/141149570) We need to fix this by introducing default private constructors or
				// fixing metalava to not emit constructors having references to unknown classes.
				filtered = append(filtered, src)
				continue
			}
			packageName := strings.ReplaceAll(filepath.Dir(src.Rel()), "/", ".")
			if android.HasAnyPrefix(packageName, filterPackages) {
				filtered = append(filtered, src)
			}
		}
		return filtered
	}
	srcFiles = filterByPackage(srcFiles, j.properties.Filter_packages)

	flags := j.collectAidlFlags(ctx, deps)
	srcFiles = j.genSources(ctx, srcFiles, flags)

	// srcs may depend on some genrule output.
	j.srcJars = srcFiles.FilterByExt(".srcjar")
	j.srcJars = append(j.srcJars, deps.srcJars...)

	j.srcFiles = srcFiles.FilterOutByExt(".srcjar")
	j.srcFiles = append(j.srcFiles, deps.srcs...)

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

	j.docZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"docs.zip")

	outDir := android.PathForModuleOut(ctx, "out")
	srcJarDir := android.PathForModuleOut(ctx, "srcjars")

	j.stubsSrcJar = nil

	rule := android.NewRuleBuilder()

	rule.Command().Text("rm -rf").Text(outDir.String())
	rule.Command().Text("mkdir -p").Text(outDir.String())

	srcJarList := zipSyncCmd(ctx, rule, srcJarDir, j.srcJars)

	javaVersion := getJavaVersion(ctx, String(j.properties.Java_version), sdkContext(j))

	cmd := javadocSystemModulesCmd(ctx, rule, j.srcFiles, outDir, srcJarDir, srcJarList,
		deps.systemModules, deps.classpath, j.sourcepaths)

	cmd.FlagWithArg("-source ", javaVersion.String()).
		Flag("-J-Xmx1024m").
		Flag("-XDignore.symbol.file").
		Flag("-Xdoclint:none")

	rule.Command().
		BuiltTool(ctx, "soong_zip").
		Flag("-write_if_changed").
		Flag("-d").
		FlagWithOutput("-o ", j.docZip).
		FlagWithArg("-C ", outDir.String()).
		FlagWithArg("-D ", outDir.String())

	rule.Restat()

	zipSyncCleanupCmd(rule, srcJarDir)

	rule.Build(pctx, ctx, "javadoc", "javadoc")
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

// droiddoc converts .java source files to documentation using doclava or dokka.
func DroiddocFactory() android.Module {
	module := &Droiddoc{}

	module.AddProperties(&module.properties,
		&module.Javadoc.properties)

	InitDroiddocModule(module, android.HostAndDeviceSupported)
	return module
}

// droiddoc_host converts .java source files to documentation using doclava or dokka.
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

func (d *Droiddoc) doclavaDocsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, docletPath classpath) {
	// Droiddoc always gets "-source 1.8" because it doesn't support 1.9 sources.  For modules with 1.9
	// sources, droiddoc will get sources produced by metalava which will have already stripped out the
	// 1.9 language features.
	cmd.FlagWithArg("-source ", "1.8").
		Flag("-J-Xmx1600m").
		Flag("-J-XX:-OmitStackTraceInFastThrow").
		Flag("-XDignore.symbol.file").
		FlagWithArg("-doclet ", "com.google.doclava.Doclava").
		FlagWithInputList("-docletpath ", docletPath.Paths(), ":").
		FlagWithArg("-hdf page.build ", ctx.Config().BuildId()+"-"+ctx.Config().BuildNumberFromFile()).
		FlagWithArg("-hdf page.now ", `"$(date -d @$(cat `+ctx.Config().Getenv("BUILD_DATETIME_FILE")+`) "+%d %b %Y %k:%M")" `)

	if String(d.properties.Custom_template) == "" {
		// TODO: This is almost always droiddoc-templates-sdk
		ctx.PropertyErrorf("custom_template", "must specify a template")
	}

	ctx.VisitDirectDepsWithTag(droiddocTemplateTag, func(m android.Module) {
		if t, ok := m.(*ExportedDroiddocDir); ok {
			cmd.FlagWithArg("-templatedir ", t.dir.String()).Implicits(t.deps)
		} else {
			ctx.PropertyErrorf("custom_template", "module %q is not a droiddoc_exported_dir", ctx.OtherModuleName(m))
		}
	})

	if len(d.properties.Html_dirs) > 0 {
		htmlDir := android.PathForModuleSrc(ctx, d.properties.Html_dirs[0])
		cmd.FlagWithArg("-htmldir ", htmlDir.String()).
			Implicits(android.PathsForModuleSrc(ctx, []string{filepath.Join(d.properties.Html_dirs[0], "**/*")}))
	}

	if len(d.properties.Html_dirs) > 1 {
		htmlDir2 := android.PathForModuleSrc(ctx, d.properties.Html_dirs[1])
		cmd.FlagWithArg("-htmldir2 ", htmlDir2.String()).
			Implicits(android.PathsForModuleSrc(ctx, []string{filepath.Join(d.properties.Html_dirs[1], "**/*")}))
	}

	if len(d.properties.Html_dirs) > 2 {
		ctx.PropertyErrorf("html_dirs", "Droiddoc only supports up to 2 html dirs")
	}

	knownTags := android.PathsForModuleSrc(ctx, d.properties.Knowntags)
	cmd.FlagForEachInput("-knowntags ", knownTags)

	cmd.FlagForEachArg("-hdf ", d.properties.Hdf)

	if String(d.properties.Proofread_file) != "" {
		proofreadFile := android.PathForModuleOut(ctx, String(d.properties.Proofread_file))
		cmd.FlagWithOutput("-proofread ", proofreadFile)
	}

	if String(d.properties.Todo_file) != "" {
		// tricky part:
		// we should not compute full path for todo_file through PathForModuleOut().
		// the non-standard doclet will get the full path relative to "-o".
		cmd.FlagWithArg("-todo ", String(d.properties.Todo_file)).
			ImplicitOutput(android.PathForModuleOut(ctx, String(d.properties.Todo_file)))
	}

	if String(d.properties.Resourcesdir) != "" {
		// TODO: should we add files under resourcesDir to the implicits? It seems that
		// resourcesDir is one sub dir of htmlDir
		resourcesDir := android.PathForModuleSrc(ctx, String(d.properties.Resourcesdir))
		cmd.FlagWithArg("-resourcesdir ", resourcesDir.String())
	}

	if String(d.properties.Resourcesoutdir) != "" {
		// TODO: it seems -resourceoutdir reference/android/images/ didn't get generated anywhere.
		cmd.FlagWithArg("-resourcesoutdir ", String(d.properties.Resourcesoutdir))
	}
}

func (d *Droiddoc) stubsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, stubsDir android.WritablePath) {
	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") ||
		apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") ||
		String(d.properties.Api_filename) != "" {

		d.apiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_api.txt")
		cmd.FlagWithOutput("-api ", d.apiFile)
		d.apiFilePath = d.apiFile
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") ||
		apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") ||
		String(d.properties.Removed_api_filename) != "" {
		d.removedApiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_removed.txt")
		cmd.FlagWithOutput("-removedApi ", d.removedApiFile)
	}

	if String(d.properties.Private_api_filename) != "" {
		d.privateApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_api_filename))
		cmd.FlagWithOutput("-privateApi ", d.privateApiFile)
	}

	if String(d.properties.Dex_api_filename) != "" {
		d.dexApiFile = android.PathForModuleOut(ctx, String(d.properties.Dex_api_filename))
		cmd.FlagWithOutput("-dexApi ", d.dexApiFile)
	}

	if String(d.properties.Private_dex_api_filename) != "" {
		d.privateDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_dex_api_filename))
		cmd.FlagWithOutput("-privateDexApi ", d.privateDexApiFile)
	}

	if String(d.properties.Removed_dex_api_filename) != "" {
		d.removedDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Removed_dex_api_filename))
		cmd.FlagWithOutput("-removedDexApi ", d.removedDexApiFile)
	}

	if String(d.properties.Exact_api_filename) != "" {
		d.exactApiFile = android.PathForModuleOut(ctx, String(d.properties.Exact_api_filename))
		cmd.FlagWithOutput("-exactApi ", d.exactApiFile)
	}

	if String(d.properties.Dex_mapping_filename) != "" {
		d.apiMappingFile = android.PathForModuleOut(ctx, String(d.properties.Dex_mapping_filename))
		cmd.FlagWithOutput("-apiMapping ", d.apiMappingFile)
	}

	if String(d.properties.Proguard_filename) != "" {
		d.proguardFile = android.PathForModuleOut(ctx, String(d.properties.Proguard_filename))
		cmd.FlagWithOutput("-proguard ", d.proguardFile)
	}

	if BoolDefault(d.properties.Create_stubs, true) {
		cmd.FlagWithArg("-stubs ", stubsDir.String())
	}

	if Bool(d.properties.Write_sdk_values) {
		cmd.FlagWithArg("-sdkvalues ", android.PathForModuleOut(ctx, "out").String())
	}
}

func (d *Droiddoc) postDoclavaCmds(ctx android.ModuleContext, rule *android.RuleBuilder) {
	if String(d.properties.Static_doc_index_redirect) != "" {
		staticDocIndexRedirect := android.PathForModuleSrc(ctx, String(d.properties.Static_doc_index_redirect))
		rule.Command().Text("cp").
			Input(staticDocIndexRedirect).
			Output(android.PathForModuleOut(ctx, "out", "index.html"))
	}

	if String(d.properties.Static_doc_properties) != "" {
		staticDocProperties := android.PathForModuleSrc(ctx, String(d.properties.Static_doc_properties))
		rule.Command().Text("cp").
			Input(staticDocProperties).
			Output(android.PathForModuleOut(ctx, "out", "source.properties"))
	}
}

func javadocCmd(ctx android.ModuleContext, rule *android.RuleBuilder, srcs android.Paths,
	outDir, srcJarDir, srcJarList android.Path, sourcepaths android.Paths) *android.RuleBuilderCommand {

	cmd := rule.Command().
		BuiltTool(ctx, "soong_javac_wrapper").Tool(config.JavadocCmd(ctx)).
		Flag(config.JavacVmFlags).
		FlagWithArg("-encoding ", "UTF-8").
		FlagWithRspFileInputList("@", srcs).
		FlagWithInput("@", srcJarList)

	// TODO(ccross): Remove this if- statement once we finish migration for all Doclava
	// based stubs generation.
	// In the future, all the docs generation depends on Metalava stubs (droidstubs) srcjar
	// dir. We need add the srcjar dir to -sourcepath arg, so that Javadoc can figure out
	// the correct package name base path.
	if len(sourcepaths) > 0 {
		cmd.FlagWithList("-sourcepath ", sourcepaths.Strings(), ":")
	} else {
		cmd.FlagWithArg("-sourcepath ", srcJarDir.String())
	}

	cmd.FlagWithArg("-d ", outDir.String()).
		Flag("-quiet")

	return cmd
}

func javadocSystemModulesCmd(ctx android.ModuleContext, rule *android.RuleBuilder, srcs android.Paths,
	outDir, srcJarDir, srcJarList android.Path, systemModules *systemModules,
	classpath classpath, sourcepaths android.Paths) *android.RuleBuilderCommand {

	cmd := javadocCmd(ctx, rule, srcs, outDir, srcJarDir, srcJarList, sourcepaths)

	flag, deps := systemModules.FormJavaSystemModulesPath(ctx.Device())
	cmd.Flag(flag).Implicits(deps)

	cmd.FlagWithArg("--patch-module ", "java.base=.")

	if len(classpath) > 0 {
		cmd.FlagWithInputList("-classpath ", classpath.Paths(), ":")
	}

	return cmd
}

func javadocBootclasspathCmd(ctx android.ModuleContext, rule *android.RuleBuilder, srcs android.Paths,
	outDir, srcJarDir, srcJarList android.Path, bootclasspath, classpath classpath,
	sourcepaths android.Paths) *android.RuleBuilderCommand {

	cmd := javadocCmd(ctx, rule, srcs, outDir, srcJarDir, srcJarList, sourcepaths)

	if len(bootclasspath) == 0 && ctx.Device() {
		// explicitly specify -bootclasspath "" if the bootclasspath is empty to
		// ensure java does not fall back to the default bootclasspath.
		cmd.FlagWithArg("-bootclasspath ", `""`)
	} else if len(bootclasspath) > 0 {
		cmd.FlagWithInputList("-bootclasspath ", bootclasspath.Paths(), ":")
	}

	if len(classpath) > 0 {
		cmd.FlagWithInputList("-classpath ", classpath.Paths(), ":")
	}

	return cmd
}

func dokkaCmd(ctx android.ModuleContext, rule *android.RuleBuilder,
	outDir, srcJarDir android.Path, bootclasspath, classpath classpath) *android.RuleBuilderCommand {

	// Dokka doesn't support bootClasspath, so combine these two classpath vars for Dokka.
	dokkaClasspath := append(bootclasspath.Paths(), classpath.Paths()...)

	return rule.Command().
		BuiltTool(ctx, "dokka").
		Flag(config.JavacVmFlags).
		Flag(srcJarDir.String()).
		FlagWithInputList("-classpath ", dokkaClasspath, ":").
		FlagWithArg("-format ", "dac").
		FlagWithArg("-dacRoot ", "/reference/kotlin").
		FlagWithArg("-output ", outDir.String())
}

func (d *Droiddoc) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	deps := d.Javadoc.collectDeps(ctx)

	d.Javadoc.docZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"docs.zip")
	d.Javadoc.stubsSrcJar = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"stubs.srcjar")

	jsilver := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "jsilver.jar")
	doclava := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "doclava.jar")
	java8Home := ctx.Config().Getenv("ANDROID_JAVA8_HOME")
	checkApiClasspath := classpath{jsilver, doclava, android.PathForSource(ctx, java8Home, "lib/tools.jar")}

	outDir := android.PathForModuleOut(ctx, "out")
	srcJarDir := android.PathForModuleOut(ctx, "srcjars")
	stubsDir := android.PathForModuleOut(ctx, "stubsDir")

	rule := android.NewRuleBuilder()

	rule.Command().Text("rm -rf").Text(outDir.String()).Text(stubsDir.String())
	rule.Command().Text("mkdir -p").Text(outDir.String()).Text(stubsDir.String())

	srcJarList := zipSyncCmd(ctx, rule, srcJarDir, d.Javadoc.srcJars)

	var cmd *android.RuleBuilderCommand
	if Bool(d.properties.Dokka_enabled) {
		cmd = dokkaCmd(ctx, rule, outDir, srcJarDir, deps.bootClasspath, deps.classpath)
	} else {
		cmd = javadocBootclasspathCmd(ctx, rule, d.Javadoc.srcFiles, outDir, srcJarDir, srcJarList,
			deps.bootClasspath, deps.classpath, d.Javadoc.sourcepaths)
	}

	d.stubsFlags(ctx, cmd, stubsDir)

	cmd.Flag(d.Javadoc.args).Implicits(d.Javadoc.argFiles)

	if d.properties.Compat_config != nil {
		compatConfig := android.PathForModuleSrc(ctx, String(d.properties.Compat_config))
		cmd.FlagWithInput("-compatconfig ", compatConfig)
	}

	var desc string
	if Bool(d.properties.Dokka_enabled) {
		desc = "dokka"
	} else {
		d.doclavaDocsFlags(ctx, cmd, classpath{jsilver, doclava})

		for _, o := range d.Javadoc.properties.Out {
			cmd.ImplicitOutput(android.PathForModuleGen(ctx, o))
		}

		d.postDoclavaCmds(ctx, rule)
		desc = "doclava"
	}

	rule.Command().
		BuiltTool(ctx, "soong_zip").
		Flag("-write_if_changed").
		Flag("-d").
		FlagWithOutput("-o ", d.docZip).
		FlagWithArg("-C ", outDir.String()).
		FlagWithArg("-D ", outDir.String())

	rule.Command().
		BuiltTool(ctx, "soong_zip").
		Flag("-write_if_changed").
		Flag("-jar").
		FlagWithOutput("-o ", d.stubsSrcJar).
		FlagWithArg("-C ", stubsDir.String()).
		FlagWithArg("-D ", stubsDir.String())

	rule.Restat()

	zipSyncCleanupCmd(rule, srcJarDir)

	rule.Build(pctx, ctx, "javadoc", desc)

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") &&
		!ctx.Config().IsPdkBuild() {

		apiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Current.Api_file))
		removedApiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Current.Removed_api_file))

		d.checkCurrentApiTimestamp = android.PathForModuleOut(ctx, "check_current_api.timestamp")

		rule := android.NewRuleBuilder()

		rule.Command().Text("( true")

		rule.Command().
			BuiltTool(ctx, "apicheck").
			Flag("-JXmx1024m").
			FlagWithInputList("-Jclasspath\\ ", checkApiClasspath.Paths(), ":").
			OptionalFlag(d.properties.Check_api.Current.Args).
			Input(apiFile).
			Input(d.apiFile).
			Input(removedApiFile).
			Input(d.removedApiFile)

		msg := fmt.Sprintf(`\n******************************\n`+
			`You have tried to change the API from what has been previously approved.\n\n`+
			`To make these errors go away, you have two choices:\n`+
			`   1. You can add '@hide' javadoc comments to the methods, etc. listed in the\n`+
			`      errors above.\n\n`+
			`   2. You can update current.txt by executing the following command:\n`+
			`         make %s-update-current-api\n\n`+
			`      To submit the revised current.txt to the main Android repository,\n`+
			`      you will need approval.\n`+
			`******************************\n`, ctx.ModuleName())

		rule.Command().
			Text("touch").Output(d.checkCurrentApiTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build(pctx, ctx, "doclavaCurrentApiCheck", "check current API")

		d.updateCurrentApiTimestamp = android.PathForModuleOut(ctx, "update_current_api.timestamp")

		// update API rule
		rule = android.NewRuleBuilder()

		rule.Command().Text("( true")

		rule.Command().
			Text("cp").Flag("-f").
			Input(d.apiFile).Flag(apiFile.String())

		rule.Command().
			Text("cp").Flag("-f").
			Input(d.removedApiFile).Flag(removedApiFile.String())

		msg = "failed to update public API"

		rule.Command().
			Text("touch").Output(d.updateCurrentApiTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build(pctx, ctx, "doclavaCurrentApiUpdate", "update current API")
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") &&
		!ctx.Config().IsPdkBuild() {

		apiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Last_released.Api_file))
		removedApiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Last_released.Removed_api_file))

		d.checkLastReleasedApiTimestamp = android.PathForModuleOut(ctx, "check_last_released_api.timestamp")

		rule := android.NewRuleBuilder()

		rule.Command().
			Text("(").
			BuiltTool(ctx, "apicheck").
			Flag("-JXmx1024m").
			FlagWithInputList("-Jclasspath\\ ", checkApiClasspath.Paths(), ":").
			OptionalFlag(d.properties.Check_api.Last_released.Args).
			Input(apiFile).
			Input(d.apiFile).
			Input(removedApiFile).
			Input(d.removedApiFile)

		msg := `\n******************************\n` +
			`You have tried to change the API from what has been previously released in\n` +
			`an SDK.  Please fix the errors listed above.\n` +
			`******************************\n`

		rule.Command().
			Text("touch").Output(d.checkLastReleasedApiTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build(pctx, ctx, "doclavaLastApiCheck", "check last API")
	}
}

//
// Droidstubs
//
type Droidstubs struct {
	Javadoc
	android.SdkBase

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
	apiLintTimestamp              android.WritablePath
	apiLintReport                 android.WritablePath

	checkNullabilityWarningsTimestamp android.WritablePath

	annotationsZip android.WritablePath
	apiVersionsXml android.WritablePath

	apiFilePath android.Path

	jdiffDocZip      android.WritablePath
	jdiffStubsSrcJar android.WritablePath

	metadataZip android.WritablePath
	metadataDir android.WritablePath
}

// droidstubs passes sources files through Metalava to generate stub .java files that only contain the API to be
// documented, filtering out hidden classes and methods.  The resulting .java files are intended to be passed to
// a droiddoc module to generate documentation.
func DroidstubsFactory() android.Module {
	module := &Droidstubs{}

	module.AddProperties(&module.properties,
		&module.Javadoc.properties)

	InitDroiddocModule(module, android.HostAndDeviceSupported)
	android.InitSdkAwareModule(module)
	return module
}

// droidstubs_host passes sources files through Metalava to generate stub .java files that only contain the API
// to be documented, filtering out hidden classes and methods.  The resulting .java files are intended to be
// passed to a droiddoc_host module to generate documentation.  Use a droidstubs_host instead of a droidstubs
// module when symbols needed by the source files are provided by java_library_host modules.
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

func (d *Droidstubs) stubsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, stubsDir android.WritablePath) {
	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") ||
		apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") ||
		String(d.properties.Api_filename) != "" {
		d.apiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_api.txt")
		cmd.FlagWithOutput("--api ", d.apiFile)
		d.apiFilePath = d.apiFile
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") ||
		apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") ||
		String(d.properties.Removed_api_filename) != "" {
		d.removedApiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_removed.txt")
		cmd.FlagWithOutput("--removed-api ", d.removedApiFile)
	}

	if String(d.properties.Private_api_filename) != "" {
		d.privateApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_api_filename))
		cmd.FlagWithOutput("--private-api ", d.privateApiFile)
	}

	if String(d.properties.Dex_api_filename) != "" {
		d.dexApiFile = android.PathForModuleOut(ctx, String(d.properties.Dex_api_filename))
		cmd.FlagWithOutput("--dex-api ", d.dexApiFile)
	}

	if String(d.properties.Private_dex_api_filename) != "" {
		d.privateDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_dex_api_filename))
		cmd.FlagWithOutput("--private-dex-api ", d.privateDexApiFile)
	}

	if String(d.properties.Removed_dex_api_filename) != "" {
		d.removedDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Removed_dex_api_filename))
		cmd.FlagWithOutput("--removed-dex-api ", d.removedDexApiFile)
	}

	if String(d.properties.Exact_api_filename) != "" {
		d.exactApiFile = android.PathForModuleOut(ctx, String(d.properties.Exact_api_filename))
		cmd.FlagWithOutput("--exact-api ", d.exactApiFile)
	}

	if String(d.properties.Dex_mapping_filename) != "" {
		d.apiMappingFile = android.PathForModuleOut(ctx, String(d.properties.Dex_mapping_filename))
		cmd.FlagWithOutput("--dex-api-mapping ", d.apiMappingFile)
	}

	if String(d.properties.Proguard_filename) != "" {
		d.proguardFile = android.PathForModuleOut(ctx, String(d.properties.Proguard_filename))
		cmd.FlagWithOutput("--proguard ", d.proguardFile)
	}

	if Bool(d.properties.Write_sdk_values) {
		d.metadataDir = android.PathForModuleOut(ctx, "metadata")
		cmd.FlagWithArg("--sdk-values ", d.metadataDir.String())
	}

	if Bool(d.properties.Create_doc_stubs) {
		cmd.FlagWithArg("--doc-stubs ", stubsDir.String())
	} else {
		cmd.FlagWithArg("--stubs ", stubsDir.String())
		cmd.Flag("--exclude-documentation-from-stubs")
	}
}

func (d *Droidstubs) annotationsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand) {
	if Bool(d.properties.Annotations_enabled) {
		cmd.Flag("--include-annotations")

		validatingNullability :=
			strings.Contains(d.Javadoc.args, "--validate-nullability-from-merged-stubs") ||
				String(d.properties.Validate_nullability_from_list) != ""

		migratingNullability := String(d.properties.Previous_api) != ""
		if migratingNullability {
			previousApi := android.PathForModuleSrc(ctx, String(d.properties.Previous_api))
			cmd.FlagWithInput("--migrate-nullness ", previousApi)
		}

		if s := String(d.properties.Validate_nullability_from_list); s != "" {
			cmd.FlagWithInput("--validate-nullability-from-list ", android.PathForModuleSrc(ctx, s))
		}

		if validatingNullability {
			d.nullabilityWarningsFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_nullability_warnings.txt")
			cmd.FlagWithOutput("--nullability-warnings-txt ", d.nullabilityWarningsFile)
		}

		d.annotationsZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"_annotations.zip")
		cmd.FlagWithOutput("--extract-annotations ", d.annotationsZip)

		if len(d.properties.Merge_annotations_dirs) == 0 {
			ctx.PropertyErrorf("merge_annotations_dirs",
				"has to be non-empty if annotations was enabled!")
		}

		d.mergeAnnoDirFlags(ctx, cmd)

		// TODO(tnorbye): find owners to fix these warnings when annotation was enabled.
		cmd.FlagWithArg("--hide ", "HiddenTypedefConstant").
			FlagWithArg("--hide ", "SuperfluousPrefix").
			FlagWithArg("--hide ", "AnnotationExtraction")
	}
}

func (d *Droidstubs) mergeAnnoDirFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand) {
	ctx.VisitDirectDepsWithTag(metalavaMergeAnnotationsDirTag, func(m android.Module) {
		if t, ok := m.(*ExportedDroiddocDir); ok {
			cmd.FlagWithArg("--merge-qualifier-annotations ", t.dir.String()).Implicits(t.deps)
		} else {
			ctx.PropertyErrorf("merge_annotations_dirs",
				"module %q is not a metalava merge-annotations dir", ctx.OtherModuleName(m))
		}
	})
}

func (d *Droidstubs) inclusionAnnotationsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand) {
	ctx.VisitDirectDepsWithTag(metalavaMergeInclusionAnnotationsDirTag, func(m android.Module) {
		if t, ok := m.(*ExportedDroiddocDir); ok {
			cmd.FlagWithArg("--merge-inclusion-annotations ", t.dir.String()).Implicits(t.deps)
		} else {
			ctx.PropertyErrorf("merge_inclusion_annotations_dirs",
				"module %q is not a metalava merge-annotations dir", ctx.OtherModuleName(m))
		}
	})
}

func (d *Droidstubs) apiLevelsAnnotationsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand) {
	if Bool(d.properties.Api_levels_annotations_enabled) {
		d.apiVersionsXml = android.PathForModuleOut(ctx, "api-versions.xml")

		if len(d.properties.Api_levels_annotations_dirs) == 0 {
			ctx.PropertyErrorf("api_levels_annotations_dirs",
				"has to be non-empty if api levels annotations was enabled!")
		}

		cmd.FlagWithOutput("--generate-api-levels ", d.apiVersionsXml)
		cmd.FlagWithInput("--apply-api-levels ", d.apiVersionsXml)
		cmd.FlagWithArg("--current-version ", ctx.Config().PlatformSdkVersion())
		cmd.FlagWithArg("--current-codename ", ctx.Config().PlatformSdkCodename())

		ctx.VisitDirectDepsWithTag(metalavaAPILevelsAnnotationsDirTag, func(m android.Module) {
			if t, ok := m.(*ExportedDroiddocDir); ok {
				for _, dep := range t.deps {
					if strings.HasSuffix(dep.String(), "android.jar") {
						cmd.Implicit(dep)
					}
				}
				cmd.FlagWithArg("--android-jar-pattern ", t.dir.String()+"/%/public/android.jar")
			} else {
				ctx.PropertyErrorf("api_levels_annotations_dirs",
					"module %q is not a metalava api-levels-annotations dir", ctx.OtherModuleName(m))
			}
		})

	}
}

func (d *Droidstubs) apiToXmlFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand) {
	if Bool(d.properties.Jdiff_enabled) && !ctx.Config().IsPdkBuild() {
		if d.apiFile.String() == "" {
			ctx.ModuleErrorf("API signature file has to be specified in Metalava when jdiff is enabled.")
		}

		d.apiXmlFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_api.xml")
		cmd.FlagWithOutput("--api-xml ", d.apiXmlFile)

		if String(d.properties.Check_api.Last_released.Api_file) == "" {
			ctx.PropertyErrorf("check_api.last_released.api_file",
				"has to be non-empty if jdiff was enabled!")
		}

		lastReleasedApi := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Last_released.Api_file))
		d.lastReleasedApiXmlFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_last_released_api.xml")
		cmd.FlagWithInput("--convert-to-jdiff ", lastReleasedApi).Output(d.lastReleasedApiXmlFile)
	}
}

func metalavaCmd(ctx android.ModuleContext, rule *android.RuleBuilder, javaVersion javaVersion, srcs android.Paths,
	srcJarList android.Path, bootclasspath, classpath classpath, sourcepaths android.Paths) *android.RuleBuilderCommand {
	// Metalava uses lots of memory, restrict the number of metalava jobs that can run in parallel.
	rule.HighMem()
	cmd := rule.Command().BuiltTool(ctx, "metalava").
		Flag(config.JavacVmFlags).
		FlagWithArg("-encoding ", "UTF-8").
		FlagWithArg("-source ", javaVersion.String()).
		FlagWithRspFileInputList("@", srcs).
		FlagWithInput("@", srcJarList)

	if len(bootclasspath) > 0 {
		cmd.FlagWithInputList("-bootclasspath ", bootclasspath.Paths(), ":")
	}

	if len(classpath) > 0 {
		cmd.FlagWithInputList("-classpath ", classpath.Paths(), ":")
	}

	if len(sourcepaths) > 0 {
		cmd.FlagWithList("-sourcepath ", sourcepaths.Strings(), ":")
	} else {
		cmd.FlagWithArg("-sourcepath ", `""`)
	}

	cmd.Flag("--no-banner").
		Flag("--color").
		Flag("--quiet").
		Flag("--format=v2")

	return cmd
}

func (d *Droidstubs) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	deps := d.Javadoc.collectDeps(ctx)

	javaVersion := getJavaVersion(ctx, String(d.Javadoc.properties.Java_version), sdkContext(d))

	// Create rule for metalava

	d.Javadoc.stubsSrcJar = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"stubs.srcjar")

	srcJarDir := android.PathForModuleOut(ctx, "srcjars")
	stubsDir := android.PathForModuleOut(ctx, "stubsDir")

	rule := android.NewRuleBuilder()

	rule.Command().Text("rm -rf").Text(stubsDir.String())
	rule.Command().Text("mkdir -p").Text(stubsDir.String())

	srcJarList := zipSyncCmd(ctx, rule, srcJarDir, d.Javadoc.srcJars)

	cmd := metalavaCmd(ctx, rule, javaVersion, d.Javadoc.srcFiles, srcJarList,
		deps.bootClasspath, deps.classpath, d.Javadoc.sourcepaths)

	d.stubsFlags(ctx, cmd, stubsDir)

	d.annotationsFlags(ctx, cmd)
	d.inclusionAnnotationsFlags(ctx, cmd)
	d.apiLevelsAnnotationsFlags(ctx, cmd)
	d.apiToXmlFlags(ctx, cmd)

	if strings.Contains(d.Javadoc.args, "--generate-documentation") {
		// Currently Metalava have the ability to invoke Javadoc in a seperate process.
		// Pass "-nodocs" to suppress the Javadoc invocation when Metalava receives
		// "--generate-documentation" arg. This is not needed when Metalava removes this feature.
		d.Javadoc.args = d.Javadoc.args + " -nodocs "
	}

	cmd.Flag(d.Javadoc.args).Implicits(d.Javadoc.argFiles)
	for _, o := range d.Javadoc.properties.Out {
		cmd.ImplicitOutput(android.PathForModuleGen(ctx, o))
	}

	rule.Command().
		BuiltTool(ctx, "soong_zip").
		Flag("-write_if_changed").
		Flag("-jar").
		FlagWithOutput("-o ", d.Javadoc.stubsSrcJar).
		FlagWithArg("-C ", stubsDir.String()).
		FlagWithArg("-D ", stubsDir.String())

	if Bool(d.properties.Write_sdk_values) {
		d.metadataZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"-metadata.zip")
		rule.Command().
			BuiltTool(ctx, "soong_zip").
			Flag("-write_if_changed").
			Flag("-d").
			FlagWithOutput("-o ", d.metadataZip).
			FlagWithArg("-C ", d.metadataDir.String()).
			FlagWithArg("-D ", d.metadataDir.String())
	}

	rule.Restat()

	zipSyncCleanupCmd(rule, srcJarDir)

	rule.Build(pctx, ctx, "metalava", "metalava")

	// Create rule for apicheck

	if BoolDefault(d.properties.Check_api.Api_lint.Enabled, false) && !ctx.Config().IsPdkBuild() {
		rule := android.NewRuleBuilder()
		rule.Command().Text("( true")

		srcJarDir := android.PathForModuleOut(ctx, "api_lint", "srcjars")
		srcJarList := zipSyncCmd(ctx, rule, srcJarDir, d.Javadoc.srcJars)

		cmd := metalavaCmd(ctx, rule, javaVersion, d.Javadoc.srcFiles, srcJarList,
			deps.bootClasspath, deps.classpath, d.Javadoc.sourcepaths)

		cmd.Flag(d.Javadoc.args).Implicits(d.Javadoc.argFiles)

		newSince := android.OptionalPathForModuleSrc(ctx, d.properties.Check_api.Api_lint.New_since)
		if newSince.Valid() {
			cmd.FlagWithInput("--api-lint ", newSince.Path())
		} else {
			cmd.Flag("--api-lint")
		}
		d.apiLintReport = android.PathForModuleOut(ctx, "api_lint_report.txt")
		cmd.FlagWithOutput("--report-even-if-suppressed ", d.apiLintReport)

		d.inclusionAnnotationsFlags(ctx, cmd)
		d.mergeAnnoDirFlags(ctx, cmd)

		baselineFile := android.OptionalPathForModuleSrc(ctx, d.properties.Check_api.Api_lint.Baseline_file)
		updatedBaselineOutput := android.PathForModuleOut(ctx, "api_lint_baseline.txt")
		d.apiLintTimestamp = android.PathForModuleOut(ctx, "api_lint.timestamp")

		if baselineFile.Valid() {
			cmd.FlagWithInput("--baseline ", baselineFile.Path())
			cmd.FlagWithOutput("--update-baseline ", updatedBaselineOutput)
		}

		zipSyncCleanupCmd(rule, srcJarDir)

		msg := fmt.Sprintf(`\n******************************\n`+
			`Your API changes are triggering API Lint warnings or errors.\n\n`+
			`To make these errors go away, you have two choices:\n`+
			`   1. You can suppress the errors with @SuppressLint(\"<id>\").\n\n`+
			`   2. You can update the baseline by executing the following command:\n`+
			`         cp \"$PWD/%s\" \"$PWD/%s\"\n\n`+
			`******************************\n`, updatedBaselineOutput, baselineFile.Path())
		rule.Command().
			Text("touch").Output(d.apiLintTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build(pctx, ctx, "metalavaApiLint", "metalava API lint")

	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") &&
		!ctx.Config().IsPdkBuild() {

		if len(d.Javadoc.properties.Out) > 0 {
			ctx.PropertyErrorf("out", "out property may not be combined with check_api")
		}

		apiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Current.Api_file))
		removedApiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Current.Removed_api_file))
		baselineFile := android.OptionalPathForModuleSrc(ctx, d.properties.Check_api.Current.Baseline_file)
		updatedBaselineOutput := android.PathForModuleOut(ctx, "current_baseline.txt")

		d.checkCurrentApiTimestamp = android.PathForModuleOut(ctx, "check_current_api.timestamp")

		rule := android.NewRuleBuilder()

		rule.Command().Text("( true")

		srcJarDir := android.PathForModuleOut(ctx, "current-apicheck", "srcjars")
		srcJarList := zipSyncCmd(ctx, rule, srcJarDir, d.Javadoc.srcJars)

		cmd := metalavaCmd(ctx, rule, javaVersion, d.Javadoc.srcFiles, srcJarList,
			deps.bootClasspath, deps.classpath, d.Javadoc.sourcepaths)

		cmd.Flag(d.Javadoc.args).Implicits(d.Javadoc.argFiles).
			FlagWithInput("--check-compatibility:api:current ", apiFile).
			FlagWithInput("--check-compatibility:removed:current ", removedApiFile)

		d.inclusionAnnotationsFlags(ctx, cmd)
		d.mergeAnnoDirFlags(ctx, cmd)

		if baselineFile.Valid() {
			cmd.FlagWithInput("--baseline ", baselineFile.Path())
			cmd.FlagWithOutput("--update-baseline ", updatedBaselineOutput)
		}

		zipSyncCleanupCmd(rule, srcJarDir)

		msg := fmt.Sprintf(`\n******************************\n`+
			`You have tried to change the API from what has been previously approved.\n\n`+
			`To make these errors go away, you have two choices:\n`+
			`   1. You can add '@hide' javadoc comments to the methods, etc. listed in the\n`+
			`      errors above.\n\n`+
			`   2. You can update current.txt by executing the following command:\n`+
			`         make %s-update-current-api\n\n`+
			`      To submit the revised current.txt to the main Android repository,\n`+
			`      you will need approval.\n`+
			`******************************\n`, ctx.ModuleName())

		rule.Command().
			Text("touch").Output(d.checkCurrentApiTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build(pctx, ctx, "metalavaCurrentApiCheck", "metalava check current API")

		d.updateCurrentApiTimestamp = android.PathForModuleOut(ctx, "update_current_api.timestamp")

		// update API rule
		rule = android.NewRuleBuilder()

		rule.Command().Text("( true")

		rule.Command().
			Text("cp").Flag("-f").
			Input(d.apiFile).Flag(apiFile.String())

		rule.Command().
			Text("cp").Flag("-f").
			Input(d.removedApiFile).Flag(removedApiFile.String())

		msg = "failed to update public API"

		rule.Command().
			Text("touch").Output(d.updateCurrentApiTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build(pctx, ctx, "metalavaCurrentApiUpdate", "update current API")
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released") &&
		!ctx.Config().IsPdkBuild() {

		if len(d.Javadoc.properties.Out) > 0 {
			ctx.PropertyErrorf("out", "out property may not be combined with check_api")
		}

		apiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Last_released.Api_file))
		removedApiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Last_released.Removed_api_file))
		baselineFile := android.OptionalPathForModuleSrc(ctx, d.properties.Check_api.Last_released.Baseline_file)
		updatedBaselineOutput := android.PathForModuleOut(ctx, "last_released_baseline.txt")

		d.checkLastReleasedApiTimestamp = android.PathForModuleOut(ctx, "check_last_released_api.timestamp")

		rule := android.NewRuleBuilder()

		rule.Command().Text("( true")

		srcJarDir := android.PathForModuleOut(ctx, "last-apicheck", "srcjars")
		srcJarList := zipSyncCmd(ctx, rule, srcJarDir, d.Javadoc.srcJars)

		cmd := metalavaCmd(ctx, rule, javaVersion, d.Javadoc.srcFiles, srcJarList,
			deps.bootClasspath, deps.classpath, d.Javadoc.sourcepaths)

		cmd.Flag(d.Javadoc.args).Implicits(d.Javadoc.argFiles).
			FlagWithInput("--check-compatibility:api:released ", apiFile)

		d.inclusionAnnotationsFlags(ctx, cmd)

		cmd.FlagWithInput("--check-compatibility:removed:released ", removedApiFile)

		d.mergeAnnoDirFlags(ctx, cmd)

		if baselineFile.Valid() {
			cmd.FlagWithInput("--baseline ", baselineFile.Path())
			cmd.FlagWithOutput("--update-baseline ", updatedBaselineOutput)
		}

		zipSyncCleanupCmd(rule, srcJarDir)

		msg := `\n******************************\n` +
			`You have tried to change the API from what has been previously released in\n` +
			`an SDK.  Please fix the errors listed above.\n` +
			`******************************\n`
		rule.Command().
			Text("touch").Output(d.checkLastReleasedApiTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build(pctx, ctx, "metalavaLastApiCheck", "metalava check last API")
	}

	if String(d.properties.Check_nullability_warnings) != "" {
		if d.nullabilityWarningsFile == nil {
			ctx.PropertyErrorf("check_nullability_warnings",
				"Cannot specify check_nullability_warnings unless validating nullability")
		}

		checkNullabilityWarnings := android.PathForModuleSrc(ctx, String(d.properties.Check_nullability_warnings))

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

		rule := android.NewRuleBuilder()

		rule.Command().
			Text("(").
			Text("diff").Input(checkNullabilityWarnings).Input(d.nullabilityWarningsFile).
			Text("&&").
			Text("touch").Output(d.checkNullabilityWarningsTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build(pctx, ctx, "nullabilityWarningsCheck", "nullability warnings check")
	}

	if Bool(d.properties.Jdiff_enabled) && !ctx.Config().IsPdkBuild() {
		if len(d.Javadoc.properties.Out) > 0 {
			ctx.PropertyErrorf("out", "out property may not be combined with jdiff")
		}

		outDir := android.PathForModuleOut(ctx, "jdiff-out")
		srcJarDir := android.PathForModuleOut(ctx, "jdiff-srcjars")
		stubsDir := android.PathForModuleOut(ctx, "jdiff-stubsDir")

		rule := android.NewRuleBuilder()

		// Please sync with android-api-council@ before making any changes for the name of jdiffDocZip below
		// since there's cron job downstream that fetch this .zip file periodically.
		// See b/116221385 for reference.
		d.jdiffDocZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"jdiff-docs.zip")
		d.jdiffStubsSrcJar = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"jdiff-stubs.srcjar")

		jdiff := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "jdiff.jar")

		rule.Command().Text("rm -rf").Text(outDir.String()).Text(stubsDir.String())
		rule.Command().Text("mkdir -p").Text(outDir.String()).Text(stubsDir.String())

		srcJarList := zipSyncCmd(ctx, rule, srcJarDir, d.Javadoc.srcJars)

		cmd := javadocBootclasspathCmd(ctx, rule, d.Javadoc.srcFiles, outDir, srcJarDir, srcJarList,
			deps.bootClasspath, deps.classpath, d.sourcepaths)

		cmd.Flag("-J-Xmx1600m").
			Flag("-XDignore.symbol.file").
			FlagWithArg("-doclet ", "jdiff.JDiff").
			FlagWithInput("-docletpath ", jdiff).
			Flag("-quiet").
			FlagWithArg("-newapi ", strings.TrimSuffix(d.apiXmlFile.Base(), d.apiXmlFile.Ext())).
			FlagWithArg("-newapidir ", filepath.Dir(d.apiXmlFile.String())).
			Implicit(d.apiXmlFile).
			FlagWithArg("-oldapi ", strings.TrimSuffix(d.lastReleasedApiXmlFile.Base(), d.lastReleasedApiXmlFile.Ext())).
			FlagWithArg("-oldapidir ", filepath.Dir(d.lastReleasedApiXmlFile.String())).
			Implicit(d.lastReleasedApiXmlFile)

		rule.Command().
			BuiltTool(ctx, "soong_zip").
			Flag("-write_if_changed").
			Flag("-d").
			FlagWithOutput("-o ", d.jdiffDocZip).
			FlagWithArg("-C ", outDir.String()).
			FlagWithArg("-D ", outDir.String())

		rule.Command().
			BuiltTool(ctx, "soong_zip").
			Flag("-write_if_changed").
			Flag("-jar").
			FlagWithOutput("-o ", d.jdiffStubsSrcJar).
			FlagWithArg("-C ", stubsDir.String()).
			FlagWithArg("-D ", stubsDir.String())

		rule.Restat()

		zipSyncCleanupCmd(rule, srcJarDir)

		rule.Build(pctx, ctx, "jdiff", "jdiff")
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

// droiddoc_exported_dir exports a directory of html templates or nullability annotations for use by doclava.
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

func zipSyncCmd(ctx android.ModuleContext, rule *android.RuleBuilder,
	srcJarDir android.ModuleOutPath, srcJars android.Paths) android.OutputPath {

	rule.Command().Text("rm -rf").Text(srcJarDir.String())
	rule.Command().Text("mkdir -p").Text(srcJarDir.String())
	srcJarList := srcJarDir.Join(ctx, "list")

	rule.Temporary(srcJarList)

	rule.Command().BuiltTool(ctx, "zipsync").
		FlagWithArg("-d ", srcJarDir.String()).
		FlagWithOutput("-l ", srcJarList).
		FlagWithArg("-f ", `"*.java"`).
		Inputs(srcJars)

	return srcJarList
}

func zipSyncCleanupCmd(rule *android.RuleBuilder, srcJarDir android.ModuleOutPath) {
	rule.Command().Text("rm -rf").Text(srcJarDir.String())
}

var _ android.PrebuiltInterface = (*PrebuiltStubsSources)(nil)

type PrebuiltStubsSourcesProperties struct {
	Srcs []string `android:"path"`
}

type PrebuiltStubsSources struct {
	android.ModuleBase
	android.DefaultableModuleBase
	prebuilt android.Prebuilt
	android.SdkBase

	properties PrebuiltStubsSourcesProperties

	// The source directories containing stubs source files.
	srcDirs     android.Paths
	stubsSrcJar android.ModuleOutPath
}

func (p *PrebuiltStubsSources) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "":
		return android.Paths{p.stubsSrcJar}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (p *PrebuiltStubsSources) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	p.stubsSrcJar = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"stubs.srcjar")

	p.srcDirs = android.PathsForModuleSrc(ctx, p.properties.Srcs)

	rule := android.NewRuleBuilder()
	command := rule.Command().
		BuiltTool(ctx, "soong_zip").
		Flag("-write_if_changed").
		Flag("-jar").
		FlagWithOutput("-o ", p.stubsSrcJar)

	for _, d := range p.srcDirs {
		dir := d.String()
		command.
			FlagWithArg("-C ", dir).
			FlagWithInput("-D ", d)
	}

	rule.Restat()

	rule.Build(pctx, ctx, "zip src", "Create srcjar from prebuilt source")
}

func (p *PrebuiltStubsSources) Prebuilt() *android.Prebuilt {
	return &p.prebuilt
}

func (p *PrebuiltStubsSources) Name() string {
	return p.prebuilt.Name(p.ModuleBase.Name())
}

// prebuilt_stubs_sources imports a set of java source files as if they were
// generated by droidstubs.
//
// By default, a prebuilt_stubs_sources has a single variant that expects a
// set of `.java` files generated by droidstubs.
//
// Specifying `host_supported: true` will produce two variants, one for use as a dependency of device modules and one
// for host modules.
//
// Intended only for use by sdk snapshots.
func PrebuiltStubsSourcesFactory() android.Module {
	module := &PrebuiltStubsSources{}

	module.AddProperties(&module.properties)

	android.InitPrebuiltModule(module, &module.properties.Srcs)
	android.InitSdkAwareModule(module)
	InitDroiddocModule(module, android.HostAndDeviceSupported)
	return module
}

type droidStubsSdkMemberType struct {
	android.SdkMemberTypeBase
}

func (mt *droidStubsSdkMemberType) AddDependencies(mctx android.BottomUpMutatorContext, dependencyTag blueprint.DependencyTag, names []string) {
	mctx.AddVariationDependencies(nil, dependencyTag, names...)
}

func (mt *droidStubsSdkMemberType) IsInstance(module android.Module) bool {
	_, ok := module.(*Droidstubs)
	return ok
}

func (mt *droidStubsSdkMemberType) BuildSnapshot(sdkModuleContext android.ModuleContext, builder android.SnapshotBuilder, member android.SdkMember) {
	variants := member.Variants()
	if len(variants) != 1 {
		sdkModuleContext.ModuleErrorf("sdk contains %d variants of member %q but only one is allowed", len(variants), member.Name())
	}
	variant := variants[0]
	d, _ := variant.(*Droidstubs)
	stubsSrcJar := d.stubsSrcJar

	snapshotRelativeDir := filepath.Join("java", d.Name()+"_stubs_sources")
	builder.UnzipToSnapshot(stubsSrcJar, snapshotRelativeDir)

	pbm := builder.AddPrebuiltModule(member, "prebuilt_stubs_sources")
	pbm.AddProperty("srcs", []string{snapshotRelativeDir})
}
