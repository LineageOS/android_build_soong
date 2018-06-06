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
				`${config.JavadocCmd} -encoding UTF-8 @$out.rsp @$srcJarDir/list ` +
				`$opts $bootclasspathArgs $classpathArgs -sourcepath $sourcepath ` +
				`-d $outDir -quiet  && ` +
				`${config.SoongZipCmd} -write_if_changed -d -o $docZip -C $outDir -D $outDir && ` +
				`${config.SoongZipCmd} -write_if_changed -jar -o $out -C $stubsDir -D $stubsDir`,
			CommandDeps: []string{
				"${config.ZipSyncCmd}",
				"${config.JavadocCmd}",
				"${config.SoongZipCmd}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
			Restat:         true,
		},
		"outDir", "srcJarDir", "stubsDir", "srcJars", "opts",
		"bootclasspathArgs", "classpathArgs", "sourcepath", "docZip")

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
			Command: `( ( cp -f $apiFileToCheck $apiFile && cp -f $removedApiFileToCheck $removedApiFile ) ` +
				`&& touch $out ) || (echo failed to update public API ; exit 38)`,
		},
		"apiFile", "apiFileToCheck", "removedApiFile", "removedApiFileToCheck")

	metalava = pctx.AndroidStaticRule("metalava",
		blueprint.RuleParams{
			Command: `rm -rf "$outDir" "$srcJarDir" "$stubsDir" && mkdir -p "$outDir" "$srcJarDir" "$stubsDir" && ` +
				`${config.ZipSyncCmd} -d $srcJarDir -l $srcJarDir/list -f "*.java" $srcJars && ` +
				`${config.JavaCmd} -jar ${config.MetalavaJar} -encoding UTF-8 -source 1.8 @$out.rsp @$srcJarDir/list ` +
				`$bootclasspathArgs $classpathArgs -sourcepath $sourcepath --no-banner --color ` +
				`--stubs $stubsDir --quiet --write-stubs-source-list $outDir/stubs_src_list $opts && ` +
				`${config.SoongZipCmd} -write_if_changed -d -o $docZip -C $outDir -D $outDir && ` +
				`${config.SoongZipCmd} -write_if_changed -jar -o $out -C $stubsDir -D $stubsDir`,
			CommandDeps: []string{
				"${config.ZipSyncCmd}",
				"${config.JavaCmd}",
				"${config.MetalavaJar}",
				"${config.JavadocCmd}",
				"${config.SoongZipCmd}",
			},
			Rspfile:        "$out.rsp",
			RspfileContent: "$in",
			Restat:         true,
		},
		"outDir", "srcJarDir", "stubsDir", "srcJars", "bootclasspathArgs", "classpathArgs", "sourcepath", "opts", "docZip")
)

func init() {
	android.RegisterModuleType("doc_defaults", DocDefaultsFactory)

	android.RegisterModuleType("droiddoc", DroiddocFactory)
	android.RegisterModuleType("droiddoc_host", DroiddocHostFactory)
	android.RegisterModuleType("droiddoc_template", DroiddocTemplateFactory)
	android.RegisterModuleType("javadoc", JavadocFactory)
	android.RegisterModuleType("javadoc_host", JavadocHostFactory)
}

type JavadocProperties struct {
	// list of source files used to compile the Java module.  May be .java, .logtags, .proto,
	// or .aidl files.
	Srcs []string `android:"arch_variant"`

	// list of directories rooted at the Android.bp file that will
	// be added to the search paths for finding source files when passing package names.
	Local_sourcepaths []string

	// list of source files that should not be used to build the Java module.
	// This is most useful in the arch/multilib variants to remove non-common files
	// filegroup or genrule can be included within this property.
	Exclude_srcs []string `android:"arch_variant"`

	// list of java libraries that will be in the classpath.
	Libs []string `android:"arch_variant"`

	// don't build against the framework libraries (legacy-test, core-junit,
	// ext, and framework for device targets)
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
}

type ApiToCheck struct {
	// path to the API txt file that the new API extracted from source code is checked
	// against. The path can be local to the module or from other module (via :module syntax).
	Api_file *string

	// path to the API txt file that the new @removed API extractd from source code is
	// checked against. The path can be local to the module or from other module (via
	// :module syntax).
	Removed_api_file *string

	// Arguments to the apicheck tool.
	Args *string
}

type DroiddocProperties struct {
	// directory relative to top of the source tree that contains doc templates files.
	Custom_template *string

	// directories relative to top of the source tree which contains html/jd files.
	Html_dirs []string

	// set a value in the Clearsilver hdf namespace.
	Hdf []string

	// proofread file contains all of the text content of the javadocs concatenated into one file,
	// suitable for spell-checking and other goodness.
	Proofread_file *string

	// a todo file lists the program elements that are missing documentation.
	// At some point, this might be improved to show more warnings.
	Todo_file *string

	// directory under current module source that provide additional resources (images).
	Resourcesdir *string

	// resources output directory under out/soong/.intermediates.
	Resourcesoutdir *string

	// local files that are used within user customized droiddoc options.
	Arg_files []string

	// user customized droiddoc args.
	// Available variables for substitution:
	//
	//  $(location <label>): the path to the arg_files with name <label>
	Args *string

	// names of the output files used in args that will be generated
	Out []string

	// a list of files under current module source dir which contains known tags in Java sources.
	// filegroup or genrule can be included within this property.
	Knowntags []string

	// the tag name used to distinguish if the API files belong to public/system/test.
	Api_tag_name *string

	// the generated public API filename by Doclava.
	Api_filename *string

	// the generated private API filename by Doclava.
	Private_api_filename *string

	// the generated private Dex API filename by Doclava.
	Private_dex_api_filename *string

	// the generated removed API filename by Doclava.
	Removed_api_filename *string

	// the generated removed Dex API filename by Doclava.
	Removed_dex_api_filename *string

	// the generated exact API filename by Doclava.
	Exact_api_filename *string

	// if set to false, don't allow droiddoc to generate stubs source files. Defaults to true.
	Create_stubs *bool

	Check_api struct {
		Last_released ApiToCheck

		Current ApiToCheck
	}

	// if set to true, create stubs through Metalava instead of Doclava. Javadoc/Doclava is
	// currently still used for documentation generation, and will be replaced by Dokka soon.
	Metalava_enabled *bool

	// user can specify the version of previous released API file in order to do compatibility check.
	Metalava_previous_api *string

	// is set to true, Metalava will allow framework SDK to contain annotations.
	Metalava_annotations_enabled *bool

	// a XML files set to merge annotations.
	Metalava_merge_annotations_dir *string
}

type Javadoc struct {
	android.ModuleBase
	android.DefaultableModuleBase

	properties JavadocProperties

	srcJars     android.Paths
	srcFiles    android.Paths
	sourcepaths android.Paths

	docZip      android.WritablePath
	stubsSrcJar android.WritablePath
}

func (j *Javadoc) Srcs() android.Paths {
	return android.Paths{j.stubsSrcJar}
}

var _ android.SourceFileProducer = (*Javadoc)(nil)

type Droiddoc struct {
	Javadoc

	properties        DroiddocProperties
	apiFile           android.WritablePath
	privateApiFile    android.WritablePath
	privateDexApiFile android.WritablePath
	removedApiFile    android.WritablePath
	removedDexApiFile android.WritablePath
	exactApiFile      android.WritablePath

	checkCurrentApiTimestamp      android.WritablePath
	updateCurrentApiTimestamp     android.WritablePath
	checkLastReleasedApiTimestamp android.WritablePath
}

func InitDroiddocModule(module android.DefaultableModule, hod android.HostOrDeviceSupported) {
	android.InitAndroidArchModule(module, hod, android.MultilibCommon)
	android.InitDefaultableModule(module)
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

func (j *Javadoc) addDeps(ctx android.BottomUpMutatorContext) {
	if ctx.Device() {
		sdkDep := decodeSdkDep(ctx, String(j.properties.Sdk_version))
		if sdkDep.useDefaultLibs {
			ctx.AddDependency(ctx.Module(), bootClasspathTag, config.DefaultBootclasspathLibraries...)
			if !Bool(j.properties.No_framework_libs) {
				ctx.AddDependency(ctx.Module(), libTag, []string{"ext", "framework"}...)
			}
		} else if sdkDep.useModule {
			ctx.AddDependency(ctx.Module(), bootClasspathTag, sdkDep.modules...)
		}
	}

	ctx.AddDependency(ctx.Module(), libTag, j.properties.Libs...)

	android.ExtractSourcesDeps(ctx, j.properties.Srcs)

	// exclude_srcs may contain filegroup or genrule.
	android.ExtractSourcesDeps(ctx, j.properties.Exclude_srcs)
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

func (j *Javadoc) collectBuilderFlags(ctx android.ModuleContext, deps deps) javaBuilderFlags {
	var flags javaBuilderFlags

	// aidl flags.
	aidlFlags := j.aidlFlags(ctx, deps.aidlPreprocess, deps.aidlIncludeDirs)
	if len(aidlFlags) > 0 {
		// optimization.
		ctx.Variable(pctx, "aidlFlags", strings.Join(aidlFlags, " "))
		flags.aidlFlags = "$aidlFlags"
	}

	return flags
}

func (j *Javadoc) aidlFlags(ctx android.ModuleContext, aidlPreprocess android.OptionalPath,
	aidlIncludeDirs android.Paths) []string {

	aidlIncludes := android.PathsForModuleSrc(ctx, j.properties.Aidl.Local_include_dirs)
	aidlIncludes = append(aidlIncludes, android.PathsForSource(ctx, j.properties.Aidl.Include_dirs)...)

	var flags []string
	if aidlPreprocess.Valid() {
		flags = append(flags, "-p"+aidlPreprocess.String())
	} else {
		flags = append(flags, android.JoinWithPrefix(aidlIncludeDirs.Strings(), "-I"))
	}

	flags = append(flags, android.JoinWithPrefix(aidlIncludes.Strings(), "-I"))
	flags = append(flags, "-I"+android.PathForModuleSrc(ctx).String())
	if src := android.ExistentPathForSource(ctx, ctx.ModuleDir(), "src"); src.Valid() {
		flags = append(flags, "-I"+src.String())
	}

	return flags
}

func (j *Javadoc) genSources(ctx android.ModuleContext, srcFiles android.Paths,
	flags javaBuilderFlags) android.Paths {

	outSrcFiles := make(android.Paths, 0, len(srcFiles))

	for _, srcFile := range srcFiles {
		switch srcFile.Ext() {
		case ".aidl":
			javaFile := genAidl(ctx, srcFile, flags.aidlFlags)
			outSrcFiles = append(outSrcFiles, javaFile)
		default:
			outSrcFiles = append(outSrcFiles, srcFile)
		}
	}

	return outSrcFiles
}

func (j *Javadoc) collectDeps(ctx android.ModuleContext) deps {
	var deps deps

	sdkDep := decodeSdkDep(ctx, String(j.properties.Sdk_version))
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
			case Dependency:
				deps.classpath = append(deps.classpath, dep.ImplementationJars()...)
				if otherName == String(j.properties.Srcs_lib) {
					srcs := dep.(SrcDependency).CompiledSrcs()
					whitelistPathPrefixes := make(map[string]bool)
					j.genWhitelistPathPrefixes(whitelistPathPrefixes)
					for _, src := range srcs {
						if _, ok := src.(android.WritablePath); ok { // generated sources
							deps.srcs = append(deps.srcs, src)
						} else { // select source path for documentation based on whitelist path prefixs.
							for k, _ := range whitelistPathPrefixes {
								if strings.HasPrefix(src.Rel(), k) {
									deps.srcs = append(deps.srcs, src)
									break
								}
							}
						}
					}
					deps.srcJars = append(deps.srcJars, dep.(SrcDependency).CompiledSrcJars()...)
				}
			case SdkLibraryDependency:
				sdkVersion := String(j.properties.Sdk_version)
				linkType := javaSdk
				if strings.HasPrefix(sdkVersion, "system_") || strings.HasPrefix(sdkVersion, "test_") {
					linkType = javaSystem
				} else if sdkVersion == "" {
					linkType = javaPlatform
				}
				deps.classpath = append(deps.classpath, dep.HeaderJars(linkType)...)
			case android.SourceFileProducer:
				checkProducesJars(ctx, dep)
				deps.classpath = append(deps.classpath, dep.Srcs()...)
			default:
				ctx.ModuleErrorf("depends on non-java module %q", otherName)
			}
		}
	})
	// do not pass exclude_srcs directly when expanding srcFiles since exclude_srcs
	// may contain filegroup or genrule.
	srcFiles := ctx.ExpandSources(j.properties.Srcs, j.properties.Exclude_srcs)
	flags := j.collectBuilderFlags(ctx, deps)
	srcFiles = j.genSources(ctx, srcFiles, flags)

	// srcs may depend on some genrule output.
	j.srcJars = srcFiles.FilterByExt(".srcjar")
	j.srcJars = append(j.srcJars, deps.srcJars...)

	j.srcFiles = srcFiles.FilterOutByExt(".srcjar")
	j.srcFiles = append(j.srcFiles, deps.srcs...)

	j.docZip = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"docs.zip")
	j.stubsSrcJar = android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"stubs.srcjar")

	if j.properties.Local_sourcepaths == nil {
		j.properties.Local_sourcepaths = append(j.properties.Local_sourcepaths, ".")
	}
	j.sourcepaths = android.PathsForModuleSrc(ctx, j.properties.Local_sourcepaths)

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

	var bootClasspathArgs, classpathArgs string
	if ctx.Config().UseOpenJDK9() {
		if len(deps.bootClasspath) > 0 {
			// For OpenJDK 9 we use --patch-module to define the core libraries code.
			// TODO(tobiast): Reorganize this when adding proper support for OpenJDK 9
			// modules. Here we treat all code in core libraries as being in java.base
			// to work around the OpenJDK 9 module system. http://b/62049770
			bootClasspathArgs = "--patch-module=java.base=" + strings.Join(deps.bootClasspath.Strings(), ":")
		}
	} else {
		if len(deps.bootClasspath.Strings()) > 0 {
			// For OpenJDK 8 we can use -bootclasspath to define the core libraries code.
			bootClasspathArgs = deps.bootClasspath.FormJavaClassPath("-bootclasspath")
		}
	}
	if len(deps.classpath.Strings()) > 0 {
		classpathArgs = "-classpath " + strings.Join(deps.classpath.Strings(), ":")
	}

	implicits = append(implicits, j.srcJars...)

	opts := "-J-Xmx1024m -XDignore.symbol.file -Xdoclint:none"

	ctx.Build(pctx, android.BuildParams{
		Rule:           javadoc,
		Description:    "Javadoc",
		Output:         j.stubsSrcJar,
		ImplicitOutput: j.docZip,
		Inputs:         j.srcFiles,
		Implicits:      implicits,
		Args: map[string]string{
			"outDir":            android.PathForModuleOut(ctx, "docs", "out").String(),
			"srcJarDir":         android.PathForModuleOut(ctx, "docs", "srcjars").String(),
			"stubsDir":          android.PathForModuleOut(ctx, "docs", "stubsDir").String(),
			"srcJars":           strings.Join(j.srcJars.Strings(), " "),
			"opts":              opts,
			"bootclasspathArgs": bootClasspathArgs,
			"classpathArgs":     classpathArgs,
			"sourcepath":        strings.Join(j.sourcepaths.Strings(), ":"),
			"docZip":            j.docZip.String(),
		},
	})
}

func (d *Droiddoc) checkCurrentApi() bool {
	if String(d.properties.Check_api.Current.Api_file) != "" &&
		String(d.properties.Check_api.Current.Removed_api_file) != "" {
		return true
	} else if String(d.properties.Check_api.Current.Api_file) != "" {
		panic("check_api.current.removed_api_file: has to be non empty!")
	} else if String(d.properties.Check_api.Current.Removed_api_file) != "" {
		panic("check_api.current.api_file: has to be non empty!")
	}

	return false
}

func (d *Droiddoc) checkLastReleasedApi() bool {
	if String(d.properties.Check_api.Last_released.Api_file) != "" &&
		String(d.properties.Check_api.Last_released.Removed_api_file) != "" {
		return true
	} else if String(d.properties.Check_api.Last_released.Api_file) != "" {
		panic("check_api.last_released.removed_api_file: has to be non empty!")
	} else if String(d.properties.Check_api.Last_released.Removed_api_file) != "" {
		panic("check_api.last_released.api_file: has to be non empty!")
	}

	return false
}

func (d *Droiddoc) DepsMutator(ctx android.BottomUpMutatorContext) {
	d.Javadoc.addDeps(ctx)

	if String(d.properties.Custom_template) != "" {
		ctx.AddDependency(ctx.Module(), droiddocTemplateTag, String(d.properties.Custom_template))
	}

	// extra_arg_files may contains filegroup or genrule.
	android.ExtractSourcesDeps(ctx, d.properties.Arg_files)

	// knowntags may contain filegroup or genrule.
	android.ExtractSourcesDeps(ctx, d.properties.Knowntags)

	if d.checkCurrentApi() {
		android.ExtractSourceDeps(ctx, d.properties.Check_api.Current.Api_file)
		android.ExtractSourceDeps(ctx, d.properties.Check_api.Current.Removed_api_file)
	}

	if d.checkLastReleasedApi() {
		android.ExtractSourceDeps(ctx, d.properties.Check_api.Last_released.Api_file)
		android.ExtractSourceDeps(ctx, d.properties.Check_api.Last_released.Removed_api_file)
	}

	if String(d.properties.Metalava_previous_api) != "" {
		android.ExtractSourceDeps(ctx, d.properties.Metalava_previous_api)
	}
}

func (d *Droiddoc) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	deps := d.Javadoc.collectDeps(ctx)

	var implicits android.Paths
	implicits = append(implicits, deps.bootClasspath...)
	implicits = append(implicits, deps.classpath...)

	bootClasspathArgs := deps.bootClasspath.FormJavaClassPath("-bootclasspath")
	classpathArgs := deps.classpath.FormJavaClassPath("-classpath")

	argFiles := ctx.ExpandSources(d.properties.Arg_files, nil)
	argFilesMap := map[string]android.Path{}

	for _, f := range argFiles {
		implicits = append(implicits, f)
		if _, exists := argFilesMap[f.Rel()]; !exists {
			argFilesMap[f.Rel()] = f
		} else {
			ctx.ModuleErrorf("multiple arg_files for %q, %q and %q",
				f, argFilesMap[f.Rel()], f.Rel())
		}
	}

	args, err := android.Expand(String(d.properties.Args), func(name string) (string, error) {
		if strings.HasPrefix(name, "location ") {
			label := strings.TrimSpace(strings.TrimPrefix(name, "location "))
			if f, ok := argFilesMap[label]; ok {
				return f.String(), nil
			} else {
				return "", fmt.Errorf("unknown location label %q", label)
			}
		} else if name == "genDir" {
			return android.PathForModuleGen(ctx).String(), nil
		}
		return "", fmt.Errorf("unknown variable '$(%s)'", name)
	})

	if err != nil {
		ctx.PropertyErrorf("args", "%s", err.Error())
		return
	}

	genDocsForMetalava := false
	var metalavaArgs string
	if Bool(d.properties.Metalava_enabled) {
		if strings.Contains(args, "--generate-documentation") {
			if !strings.Contains(args, "-nodocs") {
				genDocsForMetalava = true
			}
			// TODO(nanzhang): Add a Soong property to handle documentation args.
			metalavaArgs = strings.Split(args, "--generate-documentation")[0]
		} else {
			metalavaArgs = args
		}
	}

	var templateDir, htmlDirArgs, htmlDir2Args string
	if !Bool(d.properties.Metalava_enabled) || genDocsForMetalava {
		if String(d.properties.Custom_template) == "" {
			// TODO: This is almost always droiddoc-templates-sdk
			ctx.PropertyErrorf("custom_template", "must specify a template")
		}

		ctx.VisitDirectDepsWithTag(droiddocTemplateTag, func(m android.Module) {
			if t, ok := m.(*DroiddocTemplate); ok {
				implicits = append(implicits, t.deps...)
				templateDir = t.dir.String()
			} else {
				ctx.PropertyErrorf("custom_template", "module %q is not a droiddoc_template", ctx.OtherModuleName(m))
			}
		})

		if len(d.properties.Html_dirs) > 0 {
			htmlDir := android.PathForModuleSrc(ctx, d.properties.Html_dirs[0])
			implicits = append(implicits, ctx.Glob(htmlDir.Join(ctx, "**/*").String(), nil)...)
			htmlDirArgs = "-htmldir " + htmlDir.String()
		}

		if len(d.properties.Html_dirs) > 1 {
			htmlDir2 := android.PathForModuleSrc(ctx, d.properties.Html_dirs[1])
			implicits = append(implicits, ctx.Glob(htmlDir2.Join(ctx, "**/*").String(), nil)...)
			htmlDir2Args = "-htmldir2 " + htmlDir2.String()
		}

		if len(d.properties.Html_dirs) > 2 {
			ctx.PropertyErrorf("html_dirs", "Droiddoc only supports up to 2 html dirs")
		}

		knownTags := ctx.ExpandSources(d.properties.Knowntags, nil)
		implicits = append(implicits, knownTags...)

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
	}

	var docArgsForMetalava string
	if Bool(d.properties.Metalava_enabled) && genDocsForMetalava {
		docArgsForMetalava = strings.Split(args, "--generate-documentation")[1]
	}

	var implicitOutputs android.WritablePaths

	if d.checkCurrentApi() || d.checkLastReleasedApi() || String(d.properties.Api_filename) != "" {
		d.apiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_api.txt")
		args = args + " -api " + d.apiFile.String()
		metalavaArgs = metalavaArgs + " --api " + d.apiFile.String()
		implicitOutputs = append(implicitOutputs, d.apiFile)
	}

	if d.checkCurrentApi() || d.checkLastReleasedApi() || String(d.properties.Removed_api_filename) != "" {
		d.removedApiFile = android.PathForModuleOut(ctx, ctx.ModuleName()+"_removed.txt")
		args = args + " -removedApi " + d.removedApiFile.String()
		metalavaArgs = metalavaArgs + " --removed-api " + d.removedApiFile.String()
		implicitOutputs = append(implicitOutputs, d.removedApiFile)
	}

	if String(d.properties.Private_api_filename) != "" {
		d.privateApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_api_filename))
		args = args + " -privateApi " + d.privateApiFile.String()
		metalavaArgs = metalavaArgs + " --private-api " + d.privateApiFile.String()
		implicitOutputs = append(implicitOutputs, d.privateApiFile)
	}

	if String(d.properties.Private_dex_api_filename) != "" {
		d.privateDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_dex_api_filename))
		args = args + " -privateDexApi " + d.privateDexApiFile.String()
		metalavaArgs = metalavaArgs + " --private-dex-api " + d.privateDexApiFile.String()
		implicitOutputs = append(implicitOutputs, d.privateDexApiFile)
	}

	if String(d.properties.Removed_dex_api_filename) != "" {
		d.removedDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Removed_dex_api_filename))
		args = args + " -removedDexApi " + d.removedDexApiFile.String()
		metalavaArgs = metalavaArgs + " --removed-dex-api " + d.removedDexApiFile.String()
		implicitOutputs = append(implicitOutputs, d.removedDexApiFile)
	}

	if String(d.properties.Exact_api_filename) != "" {
		d.exactApiFile = android.PathForModuleOut(ctx, String(d.properties.Exact_api_filename))
		args = args + " -exactApi " + d.exactApiFile.String()
		metalavaArgs = metalavaArgs + " --exact-api " + d.exactApiFile.String()
		implicitOutputs = append(implicitOutputs, d.exactApiFile)
	}

	implicits = append(implicits, d.Javadoc.srcJars...)

	implicitOutputs = append(implicitOutputs, d.Javadoc.docZip)
	for _, o := range d.properties.Out {
		implicitOutputs = append(implicitOutputs, android.PathForModuleGen(ctx, o))
	}

	jsilver := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "jsilver.jar")
	doclava := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "doclava.jar")

	var date string
	if runtime.GOOS == "darwin" {
		date = `date -r`
	} else {
		date = `date -d`
	}

	doclavaOpts := "-source 1.8 -J-Xmx1600m -J-XX:-OmitStackTraceInFastThrow -XDignore.symbol.file " +
		"-doclet com.google.doclava.Doclava -docletpath " + jsilver.String() + ":" + doclava.String() + " " +
		"-templatedir " + templateDir + " " + htmlDirArgs + " " + htmlDir2Args + " " +
		"-hdf page.build " + ctx.Config().BuildId() + "-" + ctx.Config().BuildNumberFromFile() + " " +
		`-hdf page.now "$$(` + date + ` @$$(cat ` + ctx.Config().Getenv("BUILD_DATETIME_FILE") + `) "+%d %b %Y %k:%M")" `

	if !Bool(d.properties.Metalava_enabled) {
		opts := doclavaOpts + args

		implicits = append(implicits, jsilver)
		implicits = append(implicits, doclava)

		if BoolDefault(d.properties.Create_stubs, true) {
			opts += " -stubs " + android.PathForModuleOut(ctx, "docs", "stubsDir").String()
		}

		ctx.Build(pctx, android.BuildParams{
			Rule:            javadoc,
			Description:     "Droiddoc",
			Output:          d.Javadoc.stubsSrcJar,
			Inputs:          d.Javadoc.srcFiles,
			Implicits:       implicits,
			ImplicitOutputs: implicitOutputs,
			Args: map[string]string{
				"outDir":            android.PathForModuleOut(ctx, "docs", "out").String(),
				"srcJarDir":         android.PathForModuleOut(ctx, "docs", "srcjars").String(),
				"stubsDir":          android.PathForModuleOut(ctx, "docs", "stubsDir").String(),
				"srcJars":           strings.Join(d.Javadoc.srcJars.Strings(), " "),
				"opts":              opts,
				"bootclasspathArgs": bootClasspathArgs,
				"classpathArgs":     classpathArgs,
				"sourcepath":        strings.Join(d.Javadoc.sourcepaths.Strings(), ":"),
				"docZip":            d.Javadoc.docZip.String(),
			},
		})
	} else {
		opts := metalavaArgs

		buildArgs := map[string]string{
			"outDir":            android.PathForModuleOut(ctx, "docs", "out").String(),
			"srcJarDir":         android.PathForModuleOut(ctx, "docs", "srcjars").String(),
			"stubsDir":          android.PathForModuleOut(ctx, "docs", "stubsDir").String(),
			"srcJars":           strings.Join(d.Javadoc.srcJars.Strings(), " "),
			"bootclasspathArgs": bootClasspathArgs,
			"classpathArgs":     classpathArgs,
			"sourcepath":        strings.Join(d.Javadoc.sourcepaths.Strings(), ":"),
			"docZip":            d.Javadoc.docZip.String(),
		}

		var previousApi android.Path
		if String(d.properties.Metalava_previous_api) != "" {
			previousApi = ctx.ExpandSource(String(d.properties.Metalava_previous_api),
				"metalava_previous_api")
			opts += " --check-compatibility  --previous-api " + previousApi.String()
			implicits = append(implicits, previousApi)
		}

		if Bool(d.properties.Metalava_annotations_enabled) {
			if String(d.properties.Metalava_previous_api) == "" {
				ctx.PropertyErrorf("metalava_previous_api",
					"has to be non-empty if annotations was enabled!")
			}
			opts += " --include-annotations --migrate-nullness"

			annotationsZip := android.PathForModuleOut(ctx, ctx.ModuleName()+"_annotations.zip")
			implicitOutputs = append(implicitOutputs, annotationsZip)

			if String(d.properties.Metalava_merge_annotations_dir) == "" {
				ctx.PropertyErrorf("metalava_merge_annotations",
					"has to be non-empty if annotations was enabled!")
			}

			mergeAnnotationsDir := android.PathForModuleSrc(ctx,
				String(d.properties.Metalava_merge_annotations_dir))
			implicits = append(implicits, ctx.Glob(mergeAnnotationsDir.Join(ctx, "**/*").String(), nil)...)

			opts += " --extract-annotations " + annotationsZip.String() + " --merge-annotations " + mergeAnnotationsDir.String()
			// TODO(tnorbye): find owners to fix these warnings when annotation was enabled.
			opts += "--hide HiddenTypedefConstant --hide SuperfluousPrefix --hide AnnotationExtraction"
		}

		if genDocsForMetalava {
			opts += " --generate-documentation ${config.JavadocCmd} -encoding UTF-8 STUBS_SOURCE_LIST " +
				doclavaOpts + docArgsForMetalava + bootClasspathArgs + " " + classpathArgs + " " + " -sourcepath " +
				strings.Join(d.Javadoc.sourcepaths.Strings(), ":") + " -quiet -d $outDir "
			implicits = append(implicits, jsilver)
			implicits = append(implicits, doclava)
		}

		buildArgs["opts"] = opts
		ctx.Build(pctx, android.BuildParams{
			Rule:            metalava,
			Description:     "Metalava",
			Output:          d.Javadoc.stubsSrcJar,
			Inputs:          d.Javadoc.srcFiles,
			Implicits:       implicits,
			ImplicitOutputs: implicitOutputs,
			Args:            buildArgs,
		})
	}

	java8Home := ctx.Config().Getenv("ANDROID_JAVA8_HOME")

	checkApiClasspath := classpath{jsilver, doclava, android.PathForSource(ctx, java8Home, "lib/tools.jar")}

	if d.checkCurrentApi() && !ctx.Config().IsPdkBuild() {
		d.checkCurrentApiTimestamp = android.PathForModuleOut(ctx, "check_current_api.timestamp")

		apiFile := ctx.ExpandSource(String(d.properties.Check_api.Current.Api_file),
			"check_api.current.api_file")
		removedApiFile := ctx.ExpandSource(String(d.properties.Check_api.Current.Removed_api_file),
			"check_api.current_removed_api_file")

		ctx.Build(pctx, android.BuildParams{
			Rule:        apiCheck,
			Description: "Current API check",
			Output:      d.checkCurrentApiTimestamp,
			Inputs:      nil,
			Implicits: append(android.Paths{apiFile, removedApiFile, d.apiFile, d.removedApiFile},
				checkApiClasspath...),
			Args: map[string]string{
				"classpath":             checkApiClasspath.FormJavaClassPath(""),
				"opts":                  String(d.properties.Check_api.Current.Args),
				"apiFile":               apiFile.String(),
				"apiFileToCheck":        d.apiFile.String(),
				"removedApiFile":        removedApiFile.String(),
				"removedApiFileToCheck": d.removedApiFile.String(),
				"msg": fmt.Sprintf(`\n******************************\n`+
					`You have tried to change the API from what has been previously approved.\n\n`+
					`To make these errors go away, you have two choices:\n`+
					`   1. You can add '@hide' javadoc comments to the methods, etc. listed in the\n`+
					`      errors above.\n\n`+
					`   2. You can update current.txt by executing the following command:\n`+
					`         make %s-update-current-api\n\n`+
					`      To submit the revised current.txt to the main Android repository,\n`+
					`      you will need approval.\n`+
					`******************************\n`, ctx.ModuleName()),
			},
		})

		d.updateCurrentApiTimestamp = android.PathForModuleOut(ctx, "update_current_api.timestamp")

		ctx.Build(pctx, android.BuildParams{
			Rule:        updateApi,
			Description: "update current API",
			Output:      d.updateCurrentApiTimestamp,
			Implicits:   append(android.Paths{}, apiFile, removedApiFile, d.apiFile, d.removedApiFile),
			Args: map[string]string{
				"apiFile":               apiFile.String(),
				"apiFileToCheck":        d.apiFile.String(),
				"removedApiFile":        removedApiFile.String(),
				"removedApiFileToCheck": d.removedApiFile.String(),
			},
		})
	}
	if d.checkLastReleasedApi() && !ctx.Config().IsPdkBuild() {
		d.checkLastReleasedApiTimestamp = android.PathForModuleOut(ctx, "check_last_released_api.timestamp")

		apiFile := ctx.ExpandSource(String(d.properties.Check_api.Last_released.Api_file),
			"check_api.last_released.api_file")
		removedApiFile := ctx.ExpandSource(String(d.properties.Check_api.Last_released.Removed_api_file),
			"check_api.last_released.removed_api_file")

		ctx.Build(pctx, android.BuildParams{
			Rule:        apiCheck,
			Description: "Last Released API check",
			Output:      d.checkLastReleasedApiTimestamp,
			Inputs:      nil,
			Implicits: append(android.Paths{apiFile, removedApiFile, d.apiFile, d.removedApiFile},
				checkApiClasspath...),
			Args: map[string]string{
				"classpath":             checkApiClasspath.FormJavaClassPath(""),
				"opts":                  String(d.properties.Check_api.Last_released.Args),
				"apiFile":               apiFile.String(),
				"apiFileToCheck":        d.apiFile.String(),
				"removedApiFile":        removedApiFile.String(),
				"removedApiFileToCheck": d.removedApiFile.String(),
				"msg": `\n******************************\n` +
					`You have tried to change the API from what has been previously released in\n` +
					`an SDK.  Please fix the errors listed above.\n` +
					`******************************\n`,
			},
		})
	}
}

var droiddocTemplateTag = dependencyTag{name: "droiddoc-template"}

type DroiddocTemplateProperties struct {
	// path to the directory containing the droiddoc templates.
	Path *string
}

type DroiddocTemplate struct {
	android.ModuleBase

	properties DroiddocTemplateProperties

	deps android.Paths
	dir  android.Path
}

func DroiddocTemplateFactory() android.Module {
	module := &DroiddocTemplate{}
	module.AddProperties(&module.properties)
	android.InitAndroidModule(module)
	return module
}

func (d *DroiddocTemplate) DepsMutator(android.BottomUpMutatorContext) {}

func (d *DroiddocTemplate) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	path := android.PathForModuleSrc(ctx, String(d.properties.Path))
	d.dir = path
	d.deps = ctx.Glob(path.Join(ctx, "**/*").String(), nil)
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

func (d *DocDefaults) DepsMutator(ctx android.BottomUpMutatorContext) {
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
