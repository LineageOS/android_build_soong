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

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/java/config"
)

func init() {
	RegisterDocsBuildComponents(android.InitRegistrationContext)
}

func RegisterDocsBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("doc_defaults", DocDefaultsFactory)

	ctx.RegisterModuleType("droiddoc", DroiddocFactory)
	ctx.RegisterModuleType("droiddoc_host", DroiddocHostFactory)
	ctx.RegisterModuleType("droiddoc_exported_dir", ExportedDroiddocDirFactory)
	ctx.RegisterModuleType("javadoc", JavadocFactory)
	ctx.RegisterModuleType("javadoc_host", JavadocHostFactory)
}

type JavadocProperties struct {
	// list of source files used to compile the Java module.  May be .java, .logtags, .proto,
	// or .aidl files.
	Srcs []string `android:"path,arch_variant"`

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

	// user customized droiddoc args. Deprecated, use flags instead.
	// Available variables for substitution:
	//
	//  $(location <label>): the path to the arg_files with name <label>
	//  $$: a literal $
	Args *string

	// user customized droiddoc args. Not compatible with property args.
	// Available variables for substitution:
	//
	//  $(location <label>): the path to the arg_files with name <label>
	//  $$: a literal $
	Flags []string

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

	// A file containing a baseline for allowed lint errors.
	Lint_baseline *string `android:"path"`

	// directory under current module source that provide additional resources (images).
	Resourcesdir *string

	// resources output directory under out/soong/.intermediates.
	Resourcesoutdir *string

	// index.html under current module will be copied to docs out dir, if not null.
	Static_doc_index_redirect *string `android:"path"`

	// source.properties under current module will be copied to docs out dir, if not null.
	Static_doc_properties *string `android:"path"`

	// a list of files under current module source dir which contains known tags in Java sources.
	// filegroup or genrule can be included within this property.
	Knowntags []string `android:"path"`

	// if set to true, generate docs through Dokka instead of Doclava.
	Dokka_enabled *bool

	// Compat config XML. Generates compat change documentation if set.
	Compat_config *string `android:"path"`
}

// Common flags passed down to build rule
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

// Javadoc
type Javadoc struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties JavadocProperties

	srcJars     android.Paths
	srcFiles    android.Paths
	sourcepaths android.Paths
	implicits   android.Paths

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

func (j *Javadoc) SdkVersion(ctx android.EarlyModuleContext) android.SdkSpec {
	return android.SdkSpecFrom(ctx, String(j.properties.Sdk_version))
}

func (j *Javadoc) SystemModules() string {
	return proptools.String(j.properties.System_modules)
}

func (j *Javadoc) MinSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel {
	return j.SdkVersion(ctx).ApiLevel
}

func (j *Javadoc) ReplaceMaxSdkVersionPlaceholder(ctx android.EarlyModuleContext) android.ApiLevel {
	return j.SdkVersion(ctx).ApiLevel
}

func (j *Javadoc) TargetSdkVersion(ctx android.EarlyModuleContext) android.ApiLevel {
	return j.SdkVersion(ctx).ApiLevel
}

func (j *Javadoc) addDeps(ctx android.BottomUpMutatorContext) {
	if ctx.Device() {
		sdkDep := decodeSdkDep(ctx, android.SdkContext(j))
		if sdkDep.useModule {
			ctx.AddVariationDependencies(nil, bootClasspathTag, sdkDep.bootclasspath...)
			ctx.AddVariationDependencies(nil, systemModulesTag, sdkDep.systemModules)
			ctx.AddVariationDependencies(nil, java9LibTag, sdkDep.java9Classpath...)
			ctx.AddVariationDependencies(nil, sdkLibTag, sdkDep.classpath...)
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
	flags = append(flags, "-I"+ctx.ModuleDir())
	if src := android.ExistentPathForSource(ctx, ctx.ModuleDir(), "src"); src.Valid() {
		flags = append(flags, "-I"+src.String())
	}

	minSdkVersion := j.MinSdkVersion(ctx).FinalOrFutureInt()
	flags = append(flags, fmt.Sprintf("--min_sdk_version=%v", minSdkVersion))

	return strings.Join(flags, " "), deps
}

// TODO: remove the duplication between this and the one in gen.go
func (j *Javadoc) genSources(ctx android.ModuleContext, srcFiles android.Paths,
	flags droiddocBuilderFlags) android.Paths {

	outSrcFiles := make(android.Paths, 0, len(srcFiles))
	var aidlSrcs android.Paths

	aidlIncludeFlags := genAidlIncludeFlags(ctx, srcFiles, android.Paths{})

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
		srcJarFiles := genAidl(ctx, aidlSrcs, flags.aidlFlags+aidlIncludeFlags, nil, flags.aidlDeps)
		outSrcFiles = append(outSrcFiles, srcJarFiles...)
	}

	return outSrcFiles
}

func (j *Javadoc) collectDeps(ctx android.ModuleContext) deps {
	var deps deps

	sdkDep := decodeSdkDep(ctx, android.SdkContext(j))
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
			if ctx.OtherModuleHasProvider(module, JavaInfoProvider) {
				dep := ctx.OtherModuleProvider(module, JavaInfoProvider).(JavaInfo)
				deps.bootClasspath = append(deps.bootClasspath, dep.ImplementationJars...)
			} else if sm, ok := module.(SystemModulesProvider); ok {
				// A system modules dependency has been added to the bootclasspath
				// so add its libs to the bootclasspath.
				deps.bootClasspath = append(deps.bootClasspath, sm.HeaderJars()...)
			} else {
				panic(fmt.Errorf("unknown dependency %q for %q", otherName, ctx.ModuleName()))
			}
		case libTag, sdkLibTag:
			if dep, ok := module.(SdkLibraryDependency); ok {
				deps.classpath = append(deps.classpath, dep.SdkHeaderJars(ctx, j.SdkVersion(ctx))...)
			} else if ctx.OtherModuleHasProvider(module, JavaInfoProvider) {
				dep := ctx.OtherModuleProvider(module, JavaInfoProvider).(JavaInfo)
				deps.classpath = append(deps.classpath, dep.HeaderJars...)
				deps.aidlIncludeDirs = append(deps.aidlIncludeDirs, dep.AidlIncludeDirs...)
			} else if dep, ok := module.(android.SourceFileProducer); ok {
				checkProducesJars(ctx, dep)
				deps.classpath = append(deps.classpath, dep.Srcs()...)
			} else {
				ctx.ModuleErrorf("depends on non-java module %q", otherName)
			}
		case java9LibTag:
			if ctx.OtherModuleHasProvider(module, JavaInfoProvider) {
				dep := ctx.OtherModuleProvider(module, JavaInfoProvider).(JavaInfo)
				deps.java9Classpath = append(deps.java9Classpath, dep.HeaderJars...)
			} else {
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
	j.implicits = append(j.implicits, srcFiles...)

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

	aidlFlags := j.collectAidlFlags(ctx, deps)
	srcFiles = j.genSources(ctx, srcFiles, aidlFlags)

	// srcs may depend on some genrule output.
	j.srcJars = srcFiles.FilterByExt(".srcjar")
	j.srcJars = append(j.srcJars, deps.srcJars...)

	j.srcFiles = srcFiles.FilterOutByExt(".srcjar")
	j.srcFiles = append(j.srcFiles, deps.srcs...)

	if len(j.srcFiles) > 0 {
		j.sourcepaths = android.PathsForModuleSrc(ctx, []string{"."})
	}

	return deps
}

func (j *Javadoc) expandArgs(ctx android.ModuleContext, cmd *android.RuleBuilderCommand) {
	var argFiles android.Paths
	argFilesMap := map[string]string{}
	argFileLabels := []string{}

	for _, label := range j.properties.Arg_files {
		var paths = android.PathsForModuleSrc(ctx, []string{label})
		if _, exists := argFilesMap[label]; !exists {
			argFilesMap[label] = strings.Join(cmd.PathsForInputs(paths), " ")
			argFileLabels = append(argFileLabels, label)
			argFiles = append(argFiles, paths...)
		} else {
			ctx.ModuleErrorf("multiple arg_files for %q, %q and %q",
				label, argFilesMap[label], paths)
		}
	}

	var argsPropertyName string
	flags := make([]string, 0)
	if j.properties.Args != nil && j.properties.Flags != nil {
		ctx.PropertyErrorf("args", "flags is set. Cannot set args")
	} else if args := proptools.String(j.properties.Args); args != "" {
		flags = append(flags, args)
		argsPropertyName = "args"
	} else {
		flags = append(flags, j.properties.Flags...)
		argsPropertyName = "flags"
	}

	for _, flag := range flags {
		expanded, err := android.Expand(flag, func(name string) (string, error) {
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
			ctx.PropertyErrorf(argsPropertyName, "%s", err.Error())
		}
		cmd.Flag(expanded)
	}

	cmd.Implicits(argFiles)
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

	rule := android.NewRuleBuilder(pctx, ctx)

	rule.Command().Text("rm -rf").Text(outDir.String())
	rule.Command().Text("mkdir -p").Text(outDir.String())

	srcJarList := zipSyncCmd(ctx, rule, srcJarDir, j.srcJars)

	javaVersion := getJavaVersion(ctx, String(j.properties.Java_version), android.SdkContext(j))

	cmd := javadocSystemModulesCmd(ctx, rule, j.srcFiles, outDir, srcJarDir, srcJarList,
		deps.systemModules, deps.classpath, j.sourcepaths)

	cmd.FlagWithArg("-source ", javaVersion.String()).
		Flag("-J-Xmx1024m").
		Flag("-XDignore.symbol.file").
		Flag("-Xdoclint:none")

	j.expandArgs(ctx, cmd)

	rule.Command().
		BuiltTool("soong_zip").
		Flag("-write_if_changed").
		Flag("-d").
		FlagWithOutput("-o ", j.docZip).
		FlagWithArg("-C ", outDir.String()).
		FlagWithArg("-D ", outDir.String())

	rule.Restat()

	zipSyncCleanupCmd(rule, srcJarDir)

	rule.Build("javadoc", "javadoc")
}

// Droiddoc
type Droiddoc struct {
	Javadoc

	properties DroiddocProperties
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

func (d *Droiddoc) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	case "", ".docs.zip":
		return android.Paths{d.Javadoc.docZip}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (d *Droiddoc) DepsMutator(ctx android.BottomUpMutatorContext) {
	d.Javadoc.addDeps(ctx)

	if String(d.properties.Custom_template) != "" {
		ctx.AddDependency(ctx.Module(), droiddocTemplateTag, String(d.properties.Custom_template))
	}
}

func (d *Droiddoc) doclavaDocsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, docletPath classpath) {
	buildNumberFile := ctx.Config().BuildNumberFile(ctx)
	// Droiddoc always gets "-source 1.8" because it doesn't support 1.9 sources.  For modules with 1.9
	// sources, droiddoc will get sources produced by metalava which will have already stripped out the
	// 1.9 language features.
	cmd.FlagWithArg("-source ", getStubsJavaVersion().String()).
		Flag("-J-Xmx1600m").
		Flag("-J-XX:-OmitStackTraceInFastThrow").
		Flag("-XDignore.symbol.file").
		Flag("--ignore-source-errors").
		FlagWithArg("-doclet ", "com.google.doclava.Doclava").
		FlagWithInputList("-docletpath ", docletPath.Paths(), ":").
		FlagWithArg("-Xmaxerrs ", "10").
		FlagWithArg("-Xmaxwarns ", "10").
		Flag("-J--add-exports=jdk.javadoc/jdk.javadoc.internal.doclets.formats.html=ALL-UNNAMED").
		Flag("-J--add-exports=jdk.javadoc/jdk.javadoc.internal.tool=ALL-UNNAMED").
		Flag("-J--add-exports=jdk.compiler/com.sun.tools.javac.code=ALL-UNNAMED").
		Flag("-J--add-exports=jdk.compiler/com.sun.tools.javac.comp=ALL-UNNAMED").
		Flag("-J--add-exports=jdk.compiler/com.sun.tools.javac.model=ALL-UNNAMED").
		Flag("-J--add-exports=jdk.compiler/com.sun.tools.javac.tree=ALL-UNNAMED").
		Flag("-J--add-exports=jdk.compiler/com.sun.tools.javac.util=ALL-UNNAMED").
		FlagWithArg("-hdf page.build ", ctx.Config().BuildId()+"-$(cat "+buildNumberFile.String()+")").OrderOnly(buildNumberFile).
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

	if String(d.properties.Lint_baseline) != "" {
		cmd.FlagWithInput("-lintbaseline ", android.PathForModuleSrc(ctx, String(d.properties.Lint_baseline)))
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
		BuiltTool("soong_javac_wrapper").Tool(config.JavadocCmd(ctx)).
		Flag(config.JavacVmFlags).
		FlagWithRspFileInputList("@", android.PathForModuleOut(ctx, "javadoc.rsp"), srcs).
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
		BuiltTool("dokka").
		Flag(config.JavacVmFlags).
		Flag("-J--add-opens=java.base/java.lang=ALL-UNNAMED").
		Flag(srcJarDir.String()).
		FlagWithInputList("-classpath ", dokkaClasspath, ":").
		FlagWithArg("-format ", "dac").
		FlagWithArg("-dacRoot ", "/reference/kotlin").
		FlagWithArg("-output ", outDir.String())
}

func (d *Droiddoc) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	deps := d.Javadoc.collectDeps(ctx)

	d.Javadoc.docZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"docs.zip")

	jsilver := ctx.Config().HostJavaToolPath(ctx, "jsilver.jar")
	doclava := ctx.Config().HostJavaToolPath(ctx, "doclava.jar")

	outDir := android.PathForModuleOut(ctx, "out")
	srcJarDir := android.PathForModuleOut(ctx, "srcjars")

	rule := android.NewRuleBuilder(pctx, ctx)

	srcJarList := zipSyncCmd(ctx, rule, srcJarDir, d.Javadoc.srcJars)

	var cmd *android.RuleBuilderCommand
	if Bool(d.properties.Dokka_enabled) {
		cmd = dokkaCmd(ctx, rule, outDir, srcJarDir, deps.bootClasspath, deps.classpath)
	} else {
		cmd = javadocBootclasspathCmd(ctx, rule, d.Javadoc.srcFiles, outDir, srcJarDir, srcJarList,
			deps.bootClasspath, deps.classpath, d.Javadoc.sourcepaths)
	}

	d.expandArgs(ctx, cmd)

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
		BuiltTool("soong_zip").
		Flag("-write_if_changed").
		Flag("-d").
		FlagWithOutput("-o ", d.docZip).
		FlagWithArg("-C ", outDir.String()).
		FlagWithArg("-D ", outDir.String())

	rule.Restat()

	zipSyncCleanupCmd(rule, srcJarDir)

	rule.Build("javadoc", desc)
}

// Exported Droiddoc Directory
var droiddocTemplateTag = dependencyTag{name: "droiddoc-template"}

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

// Defaults
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

func zipSyncCmd(ctx android.ModuleContext, rule *android.RuleBuilder,
	srcJarDir android.ModuleOutPath, srcJars android.Paths) android.OutputPath {

	cmd := rule.Command()
	cmd.Text("rm -rf").Text(cmd.PathForOutput(srcJarDir))
	cmd = rule.Command()
	cmd.Text("mkdir -p").Text(cmd.PathForOutput(srcJarDir))
	srcJarList := srcJarDir.Join(ctx, "list")

	rule.Temporary(srcJarList)

	cmd = rule.Command()
	cmd.BuiltTool("zipsync").
		FlagWithArg("-d ", cmd.PathForOutput(srcJarDir)).
		FlagWithOutput("-l ", srcJarList).
		FlagWithArg("-f ", `"*.java"`).
		Inputs(srcJars)

	return srcJarList
}

func zipSyncCleanupCmd(rule *android.RuleBuilder, srcJarDir android.ModuleOutPath) {
	rule.Command().Text("rm -rf").Text(srcJarDir.String())
}
