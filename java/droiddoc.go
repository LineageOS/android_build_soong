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

	// the generated exact API filename by Doclava.
	Exact_api_filename *string

	// if set to false, don't allow droiddoc to generate stubs source files. Defaults to true.
	Create_stubs *bool
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
	exactApiFile      android.WritablePath
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
			ctx.AddDependency(ctx.Module(), bootClasspathTag, sdkDep.module)
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

func (j *Javadoc) collectDeps(ctx android.ModuleContext) deps {
	var deps deps

	sdkDep := decodeSdkDep(ctx, String(j.properties.Sdk_version))
	if sdkDep.invalidVersion {
		ctx.AddMissingDependencies([]string{sdkDep.module})
	} else if sdkDep.useFiles {
		deps.bootClasspath = append(deps.bootClasspath, sdkDep.jar)
	}

	ctx.VisitDirectDeps(func(module android.Module) {
		otherName := ctx.OtherModuleName(module)
		tag := ctx.OtherModuleDependencyTag(module)

		switch dep := module.(type) {
		case Dependency:
			switch tag {
			case bootClasspathTag:
				deps.bootClasspath = append(deps.bootClasspath, dep.ImplementationJars()...)
			case libTag:
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
			default:
				panic(fmt.Errorf("unknown dependency %q for %q", otherName, ctx.ModuleName()))
			}
		case SdkLibraryDependency:
			switch tag {
			case libTag:
				sdkVersion := String(j.properties.Sdk_version)
				linkType := javaSdk
				if strings.HasPrefix(sdkVersion, "system_") || strings.HasPrefix(sdkVersion, "test_") {
					linkType = javaSystem
				} else if sdkVersion == "" {
					linkType = javaPlatform
				}
				deps.classpath = append(deps.classpath, dep.HeaderJars(linkType)...)
			default:
				ctx.ModuleErrorf("dependency on java_sdk_library %q can only be in libs", otherName)
			}
		case android.SourceFileProducer:
			switch tag {
			case libTag:
				checkProducesJars(ctx, dep)
				deps.classpath = append(deps.classpath, dep.Srcs()...)
			case android.DefaultsDepTag, android.SourceDepTag:
				// Nothing to do
			default:
				ctx.ModuleErrorf("dependency on genrule %q may only be in srcs, libs", otherName)
			}
		default:
			switch tag {
			case android.DefaultsDepTag, android.SourceDepTag, droiddocTemplateTag:
				// Nothing to do
			default:
				ctx.ModuleErrorf("depends on non-java module %q", otherName)
			}
		}
	})
	// do not pass exclude_srcs directly when expanding srcFiles since exclude_srcs
	// may contain filegroup or genrule.
	srcFiles := ctx.ExpandSources(j.properties.Srcs, j.properties.Exclude_srcs)

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

func (d *Droiddoc) DepsMutator(ctx android.BottomUpMutatorContext) {
	d.Javadoc.addDeps(ctx)

	if String(d.properties.Custom_template) == "" {
		// TODO: This is almost always droiddoc-templates-sdk
		ctx.PropertyErrorf("custom_template", "must specify a template")
	} else {
		ctx.AddDependency(ctx.Module(), droiddocTemplateTag, String(d.properties.Custom_template))
	}

	// extra_arg_files may contains filegroup or genrule.
	android.ExtractSourcesDeps(ctx, d.properties.Arg_files)

	// knowntags may contain filegroup or genrule.
	android.ExtractSourcesDeps(ctx, d.properties.Knowntags)
}

func (d *Droiddoc) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	deps := d.Javadoc.collectDeps(ctx)

	var implicits android.Paths
	implicits = append(implicits, deps.bootClasspath...)
	implicits = append(implicits, deps.classpath...)

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
		ctx.PropertyErrorf("extra_args", "%s", err.Error())
		return
	}

	var bootClasspathArgs, classpathArgs string
	if len(deps.bootClasspath.Strings()) > 0 {
		bootClasspathArgs = "-bootclasspath " + strings.Join(deps.bootClasspath.Strings(), ":")
	}
	if len(deps.classpath.Strings()) > 0 {
		classpathArgs = "-classpath " + strings.Join(deps.classpath.Strings(), ":")
	}

	var templateDir string
	ctx.VisitDirectDepsWithTag(droiddocTemplateTag, func(m android.Module) {
		if t, ok := m.(*DroiddocTemplate); ok {
			implicits = append(implicits, t.deps...)
			templateDir = t.dir.String()
		} else {
			ctx.PropertyErrorf("custom_template", "module %q is not a droiddoc_template", ctx.OtherModuleName(m))
		}
	})

	var htmlDirArgs string
	if len(d.properties.Html_dirs) > 0 {
		htmlDir := android.PathForModuleSrc(ctx, d.properties.Html_dirs[0])
		implicits = append(implicits, ctx.Glob(htmlDir.Join(ctx, "**/*").String(), nil)...)
		htmlDirArgs = "-htmldir " + htmlDir.String()
	}

	var htmlDir2Args string
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

	var implicitOutputs android.WritablePaths
	if String(d.properties.Api_filename) != "" {
		d.apiFile = android.PathForModuleOut(ctx, String(d.properties.Api_filename))
		args = args + " -api " + d.apiFile.String()
		implicitOutputs = append(implicitOutputs, d.apiFile)
	}

	if String(d.properties.Private_api_filename) != "" {
		d.privateApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_api_filename))
		args = args + " -privateApi " + d.privateApiFile.String()
		implicitOutputs = append(implicitOutputs, d.privateApiFile)
	}

	if String(d.properties.Private_dex_api_filename) != "" {
		d.privateDexApiFile = android.PathForModuleOut(ctx, String(d.properties.Private_dex_api_filename))
		args = args + " -privateDexApi " + d.privateDexApiFile.String()
		implicitOutputs = append(implicitOutputs, d.privateDexApiFile)
	}

	if String(d.properties.Removed_api_filename) != "" {
		d.removedApiFile = android.PathForModuleOut(ctx, String(d.properties.Removed_api_filename))
		args = args + " -removedApi " + d.removedApiFile.String()
		implicitOutputs = append(implicitOutputs, d.removedApiFile)
	}

	if String(d.properties.Exact_api_filename) != "" {
		d.exactApiFile = android.PathForModuleOut(ctx, String(d.properties.Exact_api_filename))
		args = args + " -exactApi " + d.exactApiFile.String()
		implicitOutputs = append(implicitOutputs, d.exactApiFile)
	}

	implicits = append(implicits, d.Javadoc.srcJars...)

	jsilver := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "jsilver.jar")
	doclava := android.PathForOutput(ctx, "host", ctx.Config().PrebuiltOS(), "framework", "doclava.jar")
	implicits = append(implicits, jsilver)
	implicits = append(implicits, doclava)

	opts := "-source 1.8 -J-Xmx1600m -J-XX:-OmitStackTraceInFastThrow -XDignore.symbol.file " +
		"-doclet com.google.doclava.Doclava -docletpath " + jsilver.String() + ":" + doclava.String() + " " +
		"-templatedir " + templateDir + " " + htmlDirArgs + " " + htmlDir2Args + " " +
		"-hdf page.build " + ctx.Config().BuildId() + "-" + ctx.Config().BuildNumberFromFile() + " " +
		"-hdf page.now " + `"$$(date -d @$$(cat ` + ctx.Config().Getenv("BUILD_DATETIME_FILE") + `) "+%d %b %Y %k:%M")"` +
		" " + args
	if BoolDefault(d.properties.Create_stubs, true) {
		opts += " -stubs " + android.PathForModuleOut(ctx, "docs", "stubsDir").String()
	}

	implicitOutputs = append(implicitOutputs, d.Javadoc.docZip)
	for _, o := range d.properties.Out {
		implicitOutputs = append(implicitOutputs, android.PathForModuleGen(ctx, o))
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
