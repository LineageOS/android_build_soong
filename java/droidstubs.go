// Copyright 2021 Google Inc. All rights reserved.
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
	"regexp"
	"strings"

	"github.com/google/blueprint/proptools"

	"android/soong/android"
	"android/soong/java/config"
	"android/soong/remoteexec"
)

// The values allowed for Droidstubs' Api_levels_sdk_type
var allowedApiLevelSdkTypes = []string{"public", "system", "module-lib", "system-server"}

type StubsType int

const (
	Everything StubsType = iota
	Runtime
	Exportable
	Unavailable
)

func (s StubsType) String() string {
	switch s {
	case Everything:
		return "everything"
	case Runtime:
		return "runtime"
	case Exportable:
		return "exportable"
	default:
		return ""
	}
}

func StringToStubsType(s string) StubsType {
	switch strings.ToLower(s) {
	case Everything.String():
		return Everything
	case Runtime.String():
		return Runtime
	case Exportable.String():
		return Exportable
	default:
		return Unavailable
	}
}

func init() {
	RegisterStubsBuildComponents(android.InitRegistrationContext)
}

func RegisterStubsBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("stubs_defaults", StubsDefaultsFactory)

	ctx.RegisterModuleType("droidstubs", DroidstubsFactory)
	ctx.RegisterModuleType("droidstubs_host", DroidstubsHostFactory)

	ctx.RegisterModuleType("prebuilt_stubs_sources", PrebuiltStubsSourcesFactory)
}

type stubsArtifacts struct {
	nullabilityWarningsFile android.WritablePath
	annotationsZip          android.WritablePath
	apiVersionsXml          android.WritablePath
	metadataZip             android.WritablePath
	metadataDir             android.WritablePath
}

// Droidstubs
type Droidstubs struct {
	Javadoc
	embeddableInModuleAndImport

	properties     DroidstubsProperties
	apiFile        android.Path
	removedApiFile android.Path

	checkCurrentApiTimestamp      android.WritablePath
	updateCurrentApiTimestamp     android.WritablePath
	checkLastReleasedApiTimestamp android.WritablePath
	apiLintTimestamp              android.WritablePath
	apiLintReport                 android.WritablePath

	checkNullabilityWarningsTimestamp android.WritablePath

	everythingArtifacts stubsArtifacts
	exportableArtifacts stubsArtifacts

	// Single aconfig "cache file" merged from this module and all dependencies.
	mergedAconfigFiles map[string]android.Paths

	exportableApiFile        android.WritablePath
	exportableRemovedApiFile android.WritablePath
}

type DroidstubsProperties struct {
	// The generated public API filename by Metalava, defaults to <module>_api.txt
	Api_filename *string

	// the generated removed API filename by Metalava, defaults to <module>_removed.txt
	Removed_api_filename *string

	Check_api struct {
		Last_released ApiToCheck

		Current ApiToCheck

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

	// if set to true, cause Metalava to output Javadoc comments in the stubs source files. Defaults to false.
	// Has no effect if create_doc_stubs: true.
	Output_javadoc_comments *bool

	// if set to false then do not write out stubs. Defaults to true.
	//
	// TODO(b/146727827): Remove capability when we do not need to generate stubs and API separately.
	Generate_stubs *bool

	// if set to true, provides a hint to the build system that this rule uses a lot of memory,
	// which can be used for scheduling purposes
	High_mem *bool

	// if set to true, Metalava will allow framework SDK to contain API levels annotations.
	Api_levels_annotations_enabled *bool

	// Apply the api levels database created by this module rather than generating one in this droidstubs.
	Api_levels_module *string

	// the dirs which Metalava extracts API levels annotations from.
	Api_levels_annotations_dirs []string

	// the sdk kind which Metalava extracts API levels annotations from. Supports 'public', 'system', 'module-lib' and 'system-server'; defaults to public.
	Api_levels_sdk_type *string

	// the filename which Metalava extracts API levels annotations from. Defaults to android.jar.
	Api_levels_jar_filename *string

	// if set to true, collect the values used by the Dev tools and
	// write them in files packaged with the SDK. Defaults to false.
	Write_sdk_values *bool

	// path or filegroup to file defining extension an SDK name <-> numerical ID mapping and
	// what APIs exist in which SDKs; passed to metalava via --sdk-extensions-info
	Extensions_info_file *string `android:"path"`

	// API surface of this module. If set, the module contributes to an API surface.
	// For the full list of available API surfaces, refer to soong/android/sdk_version.go
	Api_surface *string

	// a list of aconfig_declarations module names that the stubs generated in this module
	// depend on.
	Aconfig_declarations []string
}

// Used by xsd_config
type ApiFilePath interface {
	ApiFilePath(StubsType) (android.Path, error)
}

type ApiStubsSrcProvider interface {
	StubsSrcJar(StubsType) (android.Path, error)
}

// Provider of information about API stubs, used by java_sdk_library.
type ApiStubsProvider interface {
	AnnotationsZip(StubsType) (android.Path, error)
	ApiFilePath
	RemovedApiFilePath(StubsType) (android.Path, error)

	ApiStubsSrcProvider
}

type currentApiTimestampProvider interface {
	CurrentApiTimestamp() android.Path
}

type annotationFlagsParams struct {
	migratingNullability    bool
	validatingNullability   bool
	nullabilityWarningsFile android.WritablePath
	annotationsZip          android.WritablePath
}
type stubsCommandParams struct {
	srcJarDir               android.ModuleOutPath
	stubsDir                android.OptionalPath
	stubsSrcJar             android.WritablePath
	metadataZip             android.WritablePath
	metadataDir             android.WritablePath
	apiVersionsXml          android.WritablePath
	nullabilityWarningsFile android.WritablePath
	annotationsZip          android.WritablePath
	stubConfig              stubsCommandConfigParams
}
type stubsCommandConfigParams struct {
	stubsType             StubsType
	javaVersion           javaVersion
	deps                  deps
	checkApi              bool
	generateStubs         bool
	doApiLint             bool
	doCheckReleased       bool
	writeSdkValues        bool
	migratingNullability  bool
	validatingNullability bool
}

// droidstubs passes sources files through Metalava to generate stub .java files that only contain the API to be
// documented, filtering out hidden classes and methods.  The resulting .java files are intended to be passed to
// a droiddoc module to generate documentation.
func DroidstubsFactory() android.Module {
	module := &Droidstubs{}

	module.AddProperties(&module.properties,
		&module.Javadoc.properties)
	module.initModuleAndImport(module)

	InitDroiddocModule(module, android.HostAndDeviceSupported)

	module.SetDefaultableHook(func(ctx android.DefaultableHookContext) {
		module.createApiContribution(ctx)
	})
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

func getStubsTypeAndTag(tag string) (StubsType, string, error) {
	if len(tag) == 0 {
		return Everything, "", nil
	}
	if tag[0] != '.' {
		return Unavailable, "", fmt.Errorf("tag must begin with \".\"")
	}

	stubsType := Everything
	// Check if the tag has a stubs type prefix (e.g. ".exportable")
	for st := Everything; st <= Exportable; st++ {
		if strings.HasPrefix(tag, "."+st.String()) {
			stubsType = st
		}
	}

	return stubsType, strings.TrimPrefix(tag, "."+stubsType.String()), nil
}

// Droidstubs' tag supports specifying with the stubs type.
// While supporting the pre-existing tags, it also supports tags with
// the stubs type prefix. Some examples are shown below:
// {.annotations.zip} - pre-existing behavior. Returns the path to the
// annotation zip.
// {.exportable} - Returns the path to the exportable stubs src jar.
// {.exportable.annotations.zip} - Returns the path to the exportable
// annotations zip file.
// {.runtime.api_versions.xml} - Runtime stubs does not generate api versions
// xml file. For unsupported combinations, the default everything output file
// is returned.
func (d *Droidstubs) OutputFiles(tag string) (android.Paths, error) {
	stubsType, prefixRemovedTag, err := getStubsTypeAndTag(tag)
	if err != nil {
		return nil, err
	}
	switch prefixRemovedTag {
	case "":
		stubsSrcJar, err := d.StubsSrcJar(stubsType)
		return android.Paths{stubsSrcJar}, err
	case ".docs.zip":
		docZip, err := d.DocZip(stubsType)
		return android.Paths{docZip}, err
	case ".api.txt", android.DefaultDistTag:
		// This is the default dist path for dist properties that have no tag property.
		apiFilePath, err := d.ApiFilePath(stubsType)
		return android.Paths{apiFilePath}, err
	case ".removed-api.txt":
		removedApiFilePath, err := d.RemovedApiFilePath(stubsType)
		return android.Paths{removedApiFilePath}, err
	case ".annotations.zip":
		annotationsZip, err := d.AnnotationsZip(stubsType)
		return android.Paths{annotationsZip}, err
	case ".api_versions.xml":
		apiVersionsXmlFilePath, err := d.ApiVersionsXmlFilePath(stubsType)
		return android.Paths{apiVersionsXmlFilePath}, err
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (d *Droidstubs) AnnotationsZip(stubsType StubsType) (ret android.Path, err error) {
	switch stubsType {
	case Everything:
		ret, err = d.everythingArtifacts.annotationsZip, nil
	case Exportable:
		ret, err = d.exportableArtifacts.annotationsZip, nil
	default:
		ret, err = nil, fmt.Errorf("annotations zip not supported for the stub type %s", stubsType.String())
	}
	return ret, err
}

func (d *Droidstubs) ApiFilePath(stubsType StubsType) (ret android.Path, err error) {
	switch stubsType {
	case Everything:
		ret, err = d.apiFile, nil
	case Exportable:
		ret, err = d.exportableApiFile, nil
	default:
		ret, err = nil, fmt.Errorf("api file path not supported for the stub type %s", stubsType.String())
	}
	if ret == nil && err == nil {
		err = fmt.Errorf("api file is null for the stub type %s", stubsType.String())
	}
	return ret, err
}

func (d *Droidstubs) ApiVersionsXmlFilePath(stubsType StubsType) (ret android.Path, err error) {
	switch stubsType {
	case Everything:
		ret, err = d.everythingArtifacts.apiVersionsXml, nil
	case Exportable:
		ret, err = d.exportableArtifacts.apiVersionsXml, nil
	default:
		ret, err = nil, fmt.Errorf("api versions xml file path not supported for the stub type %s", stubsType.String())
	}
	if ret == nil && err == nil {
		err = fmt.Errorf("api versions xml file is null for the stub type %s", stubsType.String())
	}
	return ret, err
}

func (d *Droidstubs) DocZip(stubsType StubsType) (ret android.Path, err error) {
	switch stubsType {
	case Everything:
		ret, err = d.docZip, nil
	default:
		ret, err = nil, fmt.Errorf("docs zip not supported for the stub type %s", stubsType.String())
	}
	if ret == nil && err == nil {
		err = fmt.Errorf("docs zip is null for the stub type %s", stubsType.String())
	}
	return ret, err
}

func (d *Droidstubs) RemovedApiFilePath(stubsType StubsType) (ret android.Path, err error) {
	switch stubsType {
	case Everything:
		ret, err = d.removedApiFile, nil
	case Exportable:
		ret, err = d.exportableRemovedApiFile, nil
	default:
		ret, err = nil, fmt.Errorf("removed api file path not supported for the stub type %s", stubsType.String())
	}
	if ret == nil && err == nil {
		err = fmt.Errorf("removed api file is null for the stub type %s", stubsType.String())
	}
	return ret, err
}

func (d *Droidstubs) StubsSrcJar(stubsType StubsType) (ret android.Path, err error) {
	switch stubsType {
	case Everything:
		ret, err = d.stubsSrcJar, nil
	case Exportable:
		ret, err = d.exportableStubsSrcJar, nil
	default:
		ret, err = nil, fmt.Errorf("stubs srcjar not supported for the stub type %s", stubsType.String())
	}
	if ret == nil && err == nil {
		err = fmt.Errorf("stubs srcjar is null for the stub type %s", stubsType.String())
	}
	return ret, err
}

func (d *Droidstubs) CurrentApiTimestamp() android.Path {
	return d.checkCurrentApiTimestamp
}

var metalavaMergeAnnotationsDirTag = dependencyTag{name: "metalava-merge-annotations-dir"}
var metalavaMergeInclusionAnnotationsDirTag = dependencyTag{name: "metalava-merge-inclusion-annotations-dir"}
var metalavaAPILevelsAnnotationsDirTag = dependencyTag{name: "metalava-api-levels-annotations-dir"}
var metalavaAPILevelsModuleTag = dependencyTag{name: "metalava-api-levels-module-tag"}
var metalavaCurrentApiTimestampTag = dependencyTag{name: "metalava-current-api-timestamp-tag"}

func (d *Droidstubs) DepsMutator(ctx android.BottomUpMutatorContext) {
	d.Javadoc.addDeps(ctx)

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

	if len(d.properties.Aconfig_declarations) != 0 {
		for _, aconfigDeclarationModuleName := range d.properties.Aconfig_declarations {
			ctx.AddDependency(ctx.Module(), aconfigDeclarationTag, aconfigDeclarationModuleName)
		}
	}

	if d.properties.Api_levels_module != nil {
		ctx.AddDependency(ctx.Module(), metalavaAPILevelsModuleTag, proptools.String(d.properties.Api_levels_module))
	}
}

func (d *Droidstubs) sdkValuesFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, metadataDir android.WritablePath) {
	cmd.FlagWithArg("--sdk-values ", metadataDir.String())
}

func (d *Droidstubs) stubsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, stubsDir android.OptionalPath, stubsType StubsType, checkApi bool) {

	apiFileName := proptools.StringDefault(d.properties.Api_filename, ctx.ModuleName()+"_api.txt")
	uncheckedApiFile := android.PathForModuleOut(ctx, stubsType.String(), apiFileName)
	cmd.FlagWithOutput("--api ", uncheckedApiFile)
	if checkApi || String(d.properties.Api_filename) != "" {
		if stubsType == Everything {
			d.apiFile = uncheckedApiFile
		} else if stubsType == Exportable {
			d.exportableApiFile = uncheckedApiFile
		}
	} else if sourceApiFile := proptools.String(d.properties.Check_api.Current.Api_file); sourceApiFile != "" {
		if stubsType == Everything {
			// If check api is disabled then make the source file available for export.
			d.apiFile = android.PathForModuleSrc(ctx, sourceApiFile)
		} else if stubsType == Exportable {
			d.exportableApiFile = uncheckedApiFile
		}
	}

	removedApiFileName := proptools.StringDefault(d.properties.Removed_api_filename, ctx.ModuleName()+"_removed.txt")
	uncheckedRemovedFile := android.PathForModuleOut(ctx, stubsType.String(), removedApiFileName)
	cmd.FlagWithOutput("--removed-api ", uncheckedRemovedFile)
	if checkApi || String(d.properties.Removed_api_filename) != "" {
		if stubsType == Everything {
			d.removedApiFile = uncheckedRemovedFile
		} else if stubsType == Exportable {
			d.exportableRemovedApiFile = uncheckedRemovedFile
		}
	} else if sourceRemovedApiFile := proptools.String(d.properties.Check_api.Current.Removed_api_file); sourceRemovedApiFile != "" {
		if stubsType == Everything {
			// If check api is disabled then make the source removed api file available for export.
			d.removedApiFile = android.PathForModuleSrc(ctx, sourceRemovedApiFile)
		} else if stubsType == Exportable {
			d.exportableRemovedApiFile = uncheckedRemovedFile
		}
	}

	if stubsDir.Valid() {
		if Bool(d.properties.Create_doc_stubs) {
			cmd.FlagWithArg("--doc-stubs ", stubsDir.String())
		} else {
			cmd.FlagWithArg("--stubs ", stubsDir.String())
			if !Bool(d.properties.Output_javadoc_comments) {
				cmd.Flag("--exclude-documentation-from-stubs")
			}
		}
	}
}

func (d *Droidstubs) annotationsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, params annotationFlagsParams) {
	if Bool(d.properties.Annotations_enabled) {
		cmd.Flag(config.MetalavaAnnotationsFlags)

		if params.migratingNullability {
			previousApi := android.PathForModuleSrc(ctx, String(d.properties.Previous_api))
			cmd.FlagWithInput("--migrate-nullness ", previousApi)
		}

		if s := String(d.properties.Validate_nullability_from_list); s != "" {
			cmd.FlagWithInput("--validate-nullability-from-list ", android.PathForModuleSrc(ctx, s))
		}

		if params.validatingNullability {
			cmd.FlagWithOutput("--nullability-warnings-txt ", params.nullabilityWarningsFile)
		}

		cmd.FlagWithOutput("--extract-annotations ", params.annotationsZip)

		if len(d.properties.Merge_annotations_dirs) != 0 {
			d.mergeAnnoDirFlags(ctx, cmd)
		}

		cmd.Flag(config.MetalavaAnnotationsWarningsFlags)
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

func (d *Droidstubs) apiLevelsAnnotationsFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, stubsType StubsType, apiVersionsXml android.WritablePath) {
	var apiVersions android.Path
	if proptools.Bool(d.properties.Api_levels_annotations_enabled) {
		d.apiLevelsGenerationFlags(ctx, cmd, stubsType, apiVersionsXml)
		apiVersions = apiVersionsXml
	} else {
		ctx.VisitDirectDepsWithTag(metalavaAPILevelsModuleTag, func(m android.Module) {
			if s, ok := m.(*Droidstubs); ok {
				if stubsType == Everything {
					apiVersions = s.everythingArtifacts.apiVersionsXml
				} else if stubsType == Exportable {
					apiVersions = s.exportableArtifacts.apiVersionsXml
				} else {
					ctx.ModuleErrorf("%s stubs type does not generate api-versions.xml file", stubsType.String())
				}
			} else {
				ctx.PropertyErrorf("api_levels_module",
					"module %q is not a droidstubs module", ctx.OtherModuleName(m))
			}
		})
	}
	if apiVersions != nil {
		cmd.FlagWithArg("--current-version ", ctx.Config().PlatformSdkVersion().String())
		cmd.FlagWithArg("--current-codename ", ctx.Config().PlatformSdkCodename())
		cmd.FlagWithInput("--apply-api-levels ", apiVersions)
	}
}

func (d *Droidstubs) apiLevelsGenerationFlags(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, stubsType StubsType, apiVersionsXml android.WritablePath) {
	if len(d.properties.Api_levels_annotations_dirs) == 0 {
		ctx.PropertyErrorf("api_levels_annotations_dirs",
			"has to be non-empty if api levels annotations was enabled!")
	}

	cmd.FlagWithOutput("--generate-api-levels ", apiVersionsXml)

	filename := proptools.StringDefault(d.properties.Api_levels_jar_filename, "android.jar")

	var dirs []string
	var extensions_dir string
	ctx.VisitDirectDepsWithTag(metalavaAPILevelsAnnotationsDirTag, func(m android.Module) {
		if t, ok := m.(*ExportedDroiddocDir); ok {
			extRegex := regexp.MustCompile(t.dir.String() + `/extensions/[0-9]+/public/.*\.jar`)

			// Grab the first extensions_dir and we find while scanning ExportedDroiddocDir.deps;
			// ideally this should be read from prebuiltApis.properties.Extensions_*
			for _, dep := range t.deps {
				if extRegex.MatchString(dep.String()) && d.properties.Extensions_info_file != nil {
					if extensions_dir == "" {
						extensions_dir = t.dir.String() + "/extensions"
					}
					cmd.Implicit(dep)
				}
				if dep.Base() == filename {
					cmd.Implicit(dep)
				}
				if filename != "android.jar" && dep.Base() == "android.jar" {
					// Metalava implicitly searches these patterns:
					//  prebuilts/tools/common/api-versions/android-%/android.jar
					//  prebuilts/sdk/%/public/android.jar
					// Add android.jar files from the api_levels_annotations_dirs directories to try
					// to satisfy these patterns.  If Metalava can't find a match for an API level
					// between 1 and 28 in at least one pattern it will fail.
					cmd.Implicit(dep)
				}
			}

			dirs = append(dirs, t.dir.String())
		} else {
			ctx.PropertyErrorf("api_levels_annotations_dirs",
				"module %q is not a metalava api-levels-annotations dir", ctx.OtherModuleName(m))
		}
	})

	// Add all relevant --android-jar-pattern patterns for Metalava.
	// When parsing a stub jar for a specific version, Metalava picks the first pattern that defines
	// an actual file present on disk (in the order the patterns were passed). For system APIs for
	// privileged apps that are only defined since API level 21 (Lollipop), fallback to public stubs
	// for older releases. Similarly, module-lib falls back to system API.
	var sdkDirs []string
	switch proptools.StringDefault(d.properties.Api_levels_sdk_type, "public") {
	case "system-server":
		sdkDirs = []string{"system-server", "module-lib", "system", "public"}
	case "module-lib":
		sdkDirs = []string{"module-lib", "system", "public"}
	case "system":
		sdkDirs = []string{"system", "public"}
	case "public":
		sdkDirs = []string{"public"}
	default:
		ctx.PropertyErrorf("api_levels_sdk_type", "needs to be one of %v", allowedApiLevelSdkTypes)
		return
	}

	for _, sdkDir := range sdkDirs {
		for _, dir := range dirs {
			cmd.FlagWithArg("--android-jar-pattern ", fmt.Sprintf("%s/%%/%s/%s", dir, sdkDir, filename))
		}
	}

	if d.properties.Extensions_info_file != nil {
		if extensions_dir == "" {
			ctx.ModuleErrorf("extensions_info_file set, but no SDK extension dirs found")
		}
		info_file := android.PathForModuleSrc(ctx, *d.properties.Extensions_info_file)
		cmd.Implicit(info_file)
		cmd.FlagWithArg("--sdk-extensions-root ", extensions_dir)
		cmd.FlagWithArg("--sdk-extensions-info ", info_file.String())
	}
}

func metalavaUseRbe(ctx android.ModuleContext) bool {
	return ctx.Config().UseRBE() && ctx.Config().IsEnvTrue("RBE_METALAVA")
}

func metalavaCmd(ctx android.ModuleContext, rule *android.RuleBuilder, javaVersion javaVersion, srcs android.Paths,
	srcJarList android.Path, bootclasspath, classpath classpath, homeDir android.WritablePath) *android.RuleBuilderCommand {
	rule.Command().Text("rm -rf").Flag(homeDir.String())
	rule.Command().Text("mkdir -p").Flag(homeDir.String())

	cmd := rule.Command()
	cmd.FlagWithArg("ANDROID_PREFS_ROOT=", homeDir.String())

	if metalavaUseRbe(ctx) {
		rule.Remoteable(android.RemoteRuleSupports{RBE: true})
		execStrategy := ctx.Config().GetenvWithDefault("RBE_METALAVA_EXEC_STRATEGY", remoteexec.LocalExecStrategy)
		compare := ctx.Config().IsEnvTrue("RBE_METALAVA_COMPARE")
		remoteUpdateCache := !ctx.Config().IsEnvFalse("RBE_METALAVA_REMOTE_UPDATE_CACHE")
		labels := map[string]string{"type": "tool", "name": "metalava"}
		// TODO: metalava pool rejects these jobs
		pool := ctx.Config().GetenvWithDefault("RBE_METALAVA_POOL", "java16")
		rule.Rewrapper(&remoteexec.REParams{
			Labels:              labels,
			ExecStrategy:        execStrategy,
			ToolchainInputs:     []string{config.JavaCmd(ctx).String()},
			Platform:            map[string]string{remoteexec.PoolKey: pool},
			Compare:             compare,
			NumLocalRuns:        1,
			NumRemoteRuns:       1,
			NoRemoteUpdateCache: !remoteUpdateCache,
		})
	}

	cmd.BuiltTool("metalava").ImplicitTool(ctx.Config().HostJavaToolPath(ctx, "metalava.jar")).
		Flag(config.JavacVmFlags).
		Flag(config.MetalavaAddOpens).
		FlagWithArg("--java-source ", javaVersion.String()).
		FlagWithRspFileInputList("@", android.PathForModuleOut(ctx, "metalava.rsp"), srcs).
		FlagWithInput("@", srcJarList)

	// Metalava does not differentiate between bootclasspath and classpath and has not done so for
	// years, so it is unlikely to change any time soon.
	combinedPaths := append(([]android.Path)(nil), bootclasspath.Paths()...)
	combinedPaths = append(combinedPaths, classpath.Paths()...)
	if len(combinedPaths) > 0 {
		cmd.FlagWithInputList("--classpath ", combinedPaths, ":")
	}

	cmd.Flag(config.MetalavaFlags)

	return cmd
}

// Pass flagged apis related flags to metalava. When aconfig_declarations property is not
// defined for a module, simply revert all flagged apis annotations. If aconfig_declarations
// property is defined, apply transformations and only revert the flagged apis that are not
// enabled via release configurations and are not specified in aconfig_declarations
func generateRevertAnnotationArgs(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, stubsType StubsType, aconfigFlagsPaths android.Paths) {

	if len(aconfigFlagsPaths) == 0 {
		cmd.Flag("--revert-annotation android.annotation.FlaggedApi")
		return
	}

	releasedFlaggedApisFile := android.PathForModuleOut(ctx, fmt.Sprintf("released-flagged-apis-%s.txt", stubsType.String()))
	revertAnnotationsFile := android.PathForModuleOut(ctx, fmt.Sprintf("revert-annotations-%s.txt", stubsType.String()))

	var filterArgs string
	switch stubsType {
	// No flagged apis specific flags need to be passed to metalava when generating
	// everything stubs
	case Everything:
		return

	case Runtime:
		filterArgs = "--filter='state:ENABLED+permission:READ_ONLY' --filter='permission:READ_WRITE'"

	case Exportable:
		// When the build flag RELEASE_EXPORT_RUNTIME_APIS is set to true, apis marked with
		// the flagged apis that have read_write permissions are exposed on top of the enabled
		// and read_only apis. This is to support local override of flag values at runtime.
		if ctx.Config().ReleaseExportRuntimeApis() {
			filterArgs = "--filter='state:ENABLED+permission:READ_ONLY' --filter='permission:READ_WRITE'"
		} else {
			filterArgs = "--filter='state:ENABLED+permission:READ_ONLY'"
		}
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        gatherReleasedFlaggedApisRule,
		Inputs:      aconfigFlagsPaths,
		Output:      releasedFlaggedApisFile,
		Description: fmt.Sprintf("%s gather aconfig flags", stubsType),
		Args: map[string]string{
			"flags_path":  android.JoinPathsWithPrefix(aconfigFlagsPaths, "--cache "),
			"filter_args": filterArgs,
		},
	})

	ctx.Build(pctx, android.BuildParams{
		Rule:        generateMetalavaRevertAnnotationsRule,
		Input:       releasedFlaggedApisFile,
		Output:      revertAnnotationsFile,
		Description: fmt.Sprintf("%s revert annotations", stubsType),
	})

	cmd.FlagWithInput("@", revertAnnotationsFile)
}

func (d *Droidstubs) commonMetalavaStubCmd(ctx android.ModuleContext, rule *android.RuleBuilder,
	params stubsCommandParams) *android.RuleBuilderCommand {
	if BoolDefault(d.properties.High_mem, false) {
		// This metalava run uses lots of memory, restrict the number of metalava jobs that can run in parallel.
		rule.HighMem()
	}

	if params.stubConfig.generateStubs {
		rule.Command().Text("rm -rf").Text(params.stubsDir.String())
		rule.Command().Text("mkdir -p").Text(params.stubsDir.String())
	}

	srcJarList := zipSyncCmd(ctx, rule, params.srcJarDir, d.Javadoc.srcJars)

	homeDir := android.PathForModuleOut(ctx, params.stubConfig.stubsType.String(), "home")
	cmd := metalavaCmd(ctx, rule, params.stubConfig.javaVersion, d.Javadoc.srcFiles, srcJarList,
		params.stubConfig.deps.bootClasspath, params.stubConfig.deps.classpath, homeDir)
	cmd.Implicits(d.Javadoc.implicits)

	d.stubsFlags(ctx, cmd, params.stubsDir, params.stubConfig.stubsType, params.stubConfig.checkApi)

	if params.stubConfig.writeSdkValues {
		d.sdkValuesFlags(ctx, cmd, params.metadataDir)
	}

	annotationParams := annotationFlagsParams{
		migratingNullability:    params.stubConfig.migratingNullability,
		validatingNullability:   params.stubConfig.validatingNullability,
		nullabilityWarningsFile: params.nullabilityWarningsFile,
		annotationsZip:          params.annotationsZip,
	}

	d.annotationsFlags(ctx, cmd, annotationParams)
	d.inclusionAnnotationsFlags(ctx, cmd)
	d.apiLevelsAnnotationsFlags(ctx, cmd, params.stubConfig.stubsType, params.apiVersionsXml)

	d.expandArgs(ctx, cmd)

	for _, o := range d.Javadoc.properties.Out {
		cmd.ImplicitOutput(android.PathForModuleGen(ctx, o))
	}

	return cmd
}

// Sandbox rule for generating the everything stubs and other artifacts
func (d *Droidstubs) everythingStubCmd(ctx android.ModuleContext, params stubsCommandConfigParams) {
	srcJarDir := android.PathForModuleOut(ctx, Everything.String(), "srcjars")
	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Sbox(android.PathForModuleOut(ctx, Everything.String()),
		android.PathForModuleOut(ctx, "metalava.sbox.textproto")).
		SandboxInputs()

	var stubsDir android.OptionalPath
	if params.generateStubs {
		stubsDir = android.OptionalPathForPath(android.PathForModuleOut(ctx, Everything.String(), "stubsDir"))
		d.Javadoc.stubsSrcJar = android.PathForModuleOut(ctx, Everything.String(), ctx.ModuleName()+"-"+"stubs.srcjar")
	}

	if params.writeSdkValues {
		d.everythingArtifacts.metadataDir = android.PathForModuleOut(ctx, Everything.String(), "metadata")
		d.everythingArtifacts.metadataZip = android.PathForModuleOut(ctx, Everything.String(), ctx.ModuleName()+"-metadata.zip")
	}

	if Bool(d.properties.Annotations_enabled) {
		if params.validatingNullability {
			d.everythingArtifacts.nullabilityWarningsFile = android.PathForModuleOut(ctx, Everything.String(), ctx.ModuleName()+"_nullability_warnings.txt")
		}
		d.everythingArtifacts.annotationsZip = android.PathForModuleOut(ctx, Everything.String(), ctx.ModuleName()+"_annotations.zip")
	}
	if Bool(d.properties.Api_levels_annotations_enabled) {
		d.everythingArtifacts.apiVersionsXml = android.PathForModuleOut(ctx, Everything.String(), "api-versions.xml")
	}

	commonCmdParams := stubsCommandParams{
		srcJarDir:               srcJarDir,
		stubsDir:                stubsDir,
		stubsSrcJar:             d.Javadoc.stubsSrcJar,
		metadataDir:             d.everythingArtifacts.metadataDir,
		apiVersionsXml:          d.everythingArtifacts.apiVersionsXml,
		nullabilityWarningsFile: d.everythingArtifacts.nullabilityWarningsFile,
		annotationsZip:          d.everythingArtifacts.annotationsZip,
		stubConfig:              params,
	}

	cmd := d.commonMetalavaStubCmd(ctx, rule, commonCmdParams)

	d.everythingOptionalCmd(ctx, cmd, params.doApiLint, params.doCheckReleased)

	if params.generateStubs {
		rule.Command().
			BuiltTool("soong_zip").
			Flag("-write_if_changed").
			Flag("-jar").
			FlagWithOutput("-o ", d.Javadoc.stubsSrcJar).
			FlagWithArg("-C ", stubsDir.String()).
			FlagWithArg("-D ", stubsDir.String())
	}

	if params.writeSdkValues {
		rule.Command().
			BuiltTool("soong_zip").
			Flag("-write_if_changed").
			Flag("-d").
			FlagWithOutput("-o ", d.everythingArtifacts.metadataZip).
			FlagWithArg("-C ", d.everythingArtifacts.metadataDir.String()).
			FlagWithArg("-D ", d.everythingArtifacts.metadataDir.String())
	}

	// TODO: We don't really need two separate API files, but this is a reminiscence of how
	// we used to run metalava separately for API lint and the "last_released" check. Unify them.
	if params.doApiLint {
		rule.Command().Text("touch").Output(d.apiLintTimestamp)
	}
	if params.doCheckReleased {
		rule.Command().Text("touch").Output(d.checkLastReleasedApiTimestamp)
	}

	// TODO(b/183630617): rewrapper doesn't support restat rules
	if !metalavaUseRbe(ctx) {
		rule.Restat()
	}

	zipSyncCleanupCmd(rule, srcJarDir)

	rule.Build("metalava", "metalava merged")
}

// Sandbox rule for generating the everything artifacts that are not run by
// default but only run based on the module configurations
func (d *Droidstubs) everythingOptionalCmd(ctx android.ModuleContext, cmd *android.RuleBuilderCommand, doApiLint bool, doCheckReleased bool) {

	// Add API lint options.
	if doApiLint {
		newSince := android.OptionalPathForModuleSrc(ctx, d.properties.Check_api.Api_lint.New_since)
		if newSince.Valid() {
			cmd.FlagWithInput("--api-lint ", newSince.Path())
		} else {
			cmd.Flag("--api-lint")
		}
		d.apiLintReport = android.PathForModuleOut(ctx, Everything.String(), "api_lint_report.txt")
		cmd.FlagWithOutput("--report-even-if-suppressed ", d.apiLintReport) // TODO:  Change to ":api-lint"

		// TODO(b/154317059): Clean up this allowlist by baselining and/or checking in last-released.
		if d.Name() != "android.car-system-stubs-docs" &&
			d.Name() != "android.car-stubs-docs" {
			cmd.Flag("--lints-as-errors")
			cmd.Flag("--warnings-as-errors") // Most lints are actually warnings.
		}

		baselineFile := android.OptionalPathForModuleSrc(ctx, d.properties.Check_api.Api_lint.Baseline_file)
		updatedBaselineOutput := android.PathForModuleOut(ctx, Everything.String(), "api_lint_baseline.txt")
		d.apiLintTimestamp = android.PathForModuleOut(ctx, Everything.String(), "api_lint.timestamp")

		// Note this string includes a special shell quote $' ... ', which decodes the "\n"s.
		//
		// TODO: metalava also has a slightly different message hardcoded. Should we unify this
		// message and metalava's one?
		msg := `$'` + // Enclose with $' ... '
			`************************************************************\n` +
			`Your API changes are triggering API Lint warnings or errors.\n` +
			`To make these errors go away, fix the code according to the\n` +
			`error and/or warning messages above.\n` +
			`\n` +
			`If it is not possible to do so, there are workarounds:\n` +
			`\n` +
			`1. You can suppress the errors with @SuppressLint("<id>")\n` +
			`   where the <id> is given in brackets in the error message above.\n`

		if baselineFile.Valid() {
			cmd.FlagWithInput("--baseline:api-lint ", baselineFile.Path())
			cmd.FlagWithOutput("--update-baseline:api-lint ", updatedBaselineOutput)

			msg += fmt.Sprintf(``+
				`2. You can update the baseline by executing the following\n`+
				`   command:\n`+
				`       (cd $ANDROID_BUILD_TOP && cp \\\n`+
				`       "%s" \\\n`+
				`       "%s")\n`+
				`   To submit the revised baseline.txt to the main Android\n`+
				`   repository, you will need approval.\n`, updatedBaselineOutput, baselineFile.Path())
		} else {
			msg += fmt.Sprintf(``+
				`2. You can add a baseline file of existing lint failures\n`+
				`   to the build rule of %s.\n`, d.Name())
		}
		// Note the message ends with a ' (single quote), to close the $' ... ' .
		msg += `************************************************************\n'`

		cmd.FlagWithArg("--error-message:api-lint ", msg)
	}

	// Add "check released" options. (Detect incompatible API changes from the last public release)
	if doCheckReleased {
		if len(d.Javadoc.properties.Out) > 0 {
			ctx.PropertyErrorf("out", "out property may not be combined with check_api")
		}

		apiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Last_released.Api_file))
		removedApiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Last_released.Removed_api_file))
		baselineFile := android.OptionalPathForModuleSrc(ctx, d.properties.Check_api.Last_released.Baseline_file)
		updatedBaselineOutput := android.PathForModuleOut(ctx, Everything.String(), "last_released_baseline.txt")

		d.checkLastReleasedApiTimestamp = android.PathForModuleOut(ctx, Everything.String(), "check_last_released_api.timestamp")

		cmd.FlagWithInput("--check-compatibility:api:released ", apiFile)
		cmd.FlagWithInput("--check-compatibility:removed:released ", removedApiFile)

		if baselineFile.Valid() {
			cmd.FlagWithInput("--baseline:compatibility:released ", baselineFile.Path())
			cmd.FlagWithOutput("--update-baseline:compatibility:released ", updatedBaselineOutput)
		}

		// Note this string includes quote ($' ... '), which decodes the "\n"s.
		msg := `$'\n******************************\n` +
			`You have tried to change the API from what has been previously released in\n` +
			`an SDK.  Please fix the errors listed above.\n` +
			`******************************\n'`

		cmd.FlagWithArg("--error-message:compatibility:released ", msg)
	}

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") {
		// Pass the current API file into metalava so it can use it as the basis for determining how to
		// generate the output signature files (both api and removed).
		currentApiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Current.Api_file))
		cmd.FlagWithInput("--use-same-format-as ", currentApiFile)
	}
}

// Sandbox rule for generating exportable stubs and other artifacts
func (d *Droidstubs) exportableStubCmd(ctx android.ModuleContext, params stubsCommandConfigParams) {
	optionalCmdParams := stubsCommandParams{
		stubConfig: params,
	}

	if params.generateStubs {
		d.Javadoc.exportableStubsSrcJar = android.PathForModuleOut(ctx, params.stubsType.String(), ctx.ModuleName()+"-"+"stubs.srcjar")
		optionalCmdParams.stubsSrcJar = d.Javadoc.exportableStubsSrcJar
	}

	if params.writeSdkValues {
		d.exportableArtifacts.metadataZip = android.PathForModuleOut(ctx, params.stubsType.String(), ctx.ModuleName()+"-metadata.zip")
		d.exportableArtifacts.metadataDir = android.PathForModuleOut(ctx, params.stubsType.String(), "metadata")
		optionalCmdParams.metadataZip = d.exportableArtifacts.metadataZip
		optionalCmdParams.metadataDir = d.exportableArtifacts.metadataDir
	}

	if Bool(d.properties.Annotations_enabled) {
		if params.validatingNullability {
			d.exportableArtifacts.nullabilityWarningsFile = android.PathForModuleOut(ctx, params.stubsType.String(), ctx.ModuleName()+"_nullability_warnings.txt")
			optionalCmdParams.nullabilityWarningsFile = d.exportableArtifacts.nullabilityWarningsFile
		}
		d.exportableArtifacts.annotationsZip = android.PathForModuleOut(ctx, params.stubsType.String(), ctx.ModuleName()+"_annotations.zip")
		optionalCmdParams.annotationsZip = d.exportableArtifacts.annotationsZip
	}
	if Bool(d.properties.Api_levels_annotations_enabled) {
		d.exportableArtifacts.apiVersionsXml = android.PathForModuleOut(ctx, params.stubsType.String(), "api-versions.xml")
		optionalCmdParams.apiVersionsXml = d.exportableArtifacts.apiVersionsXml
	}

	if params.checkApi || String(d.properties.Api_filename) != "" {
		filename := proptools.StringDefault(d.properties.Api_filename, ctx.ModuleName()+"_api.txt")
		d.exportableApiFile = android.PathForModuleOut(ctx, params.stubsType.String(), filename)
	}

	if params.checkApi || String(d.properties.Removed_api_filename) != "" {
		filename := proptools.StringDefault(d.properties.Removed_api_filename, ctx.ModuleName()+"_api.txt")
		d.exportableRemovedApiFile = android.PathForModuleOut(ctx, params.stubsType.String(), filename)
	}

	d.optionalStubCmd(ctx, optionalCmdParams)
}

func (d *Droidstubs) optionalStubCmd(ctx android.ModuleContext, params stubsCommandParams) {

	params.srcJarDir = android.PathForModuleOut(ctx, params.stubConfig.stubsType.String(), "srcjars")
	rule := android.NewRuleBuilder(pctx, ctx)
	rule.Sbox(android.PathForModuleOut(ctx, params.stubConfig.stubsType.String()),
		android.PathForModuleOut(ctx, fmt.Sprintf("metalava_%s.sbox.textproto", params.stubConfig.stubsType.String()))).
		SandboxInputs()

	if params.stubConfig.generateStubs {
		params.stubsDir = android.OptionalPathForPath(android.PathForModuleOut(ctx, params.stubConfig.stubsType.String(), "stubsDir"))
	}

	cmd := d.commonMetalavaStubCmd(ctx, rule, params)

	generateRevertAnnotationArgs(ctx, cmd, params.stubConfig.stubsType, params.stubConfig.deps.aconfigProtoFiles)

	if params.stubConfig.doApiLint {
		// Pass the lint baseline file as an input to resolve the lint errors.
		// The exportable stubs generation does not update the lint baseline file.
		// Lint baseline file update is handled by the everything stubs
		baselineFile := android.OptionalPathForModuleSrc(ctx, d.properties.Check_api.Api_lint.Baseline_file)
		if baselineFile.Valid() {
			cmd.FlagWithInput("--baseline:api-lint ", baselineFile.Path())
		}
	}

	if params.stubConfig.generateStubs {
		rule.Command().
			BuiltTool("soong_zip").
			Flag("-write_if_changed").
			Flag("-jar").
			FlagWithOutput("-o ", params.stubsSrcJar).
			FlagWithArg("-C ", params.stubsDir.String()).
			FlagWithArg("-D ", params.stubsDir.String())
	}

	if params.stubConfig.writeSdkValues {
		rule.Command().
			BuiltTool("soong_zip").
			Flag("-write_if_changed").
			Flag("-d").
			FlagWithOutput("-o ", params.metadataZip).
			FlagWithArg("-C ", params.metadataDir.String()).
			FlagWithArg("-D ", params.metadataDir.String())
	}

	// TODO(b/183630617): rewrapper doesn't support restat rules
	if !metalavaUseRbe(ctx) {
		rule.Restat()
	}

	zipSyncCleanupCmd(rule, params.srcJarDir)

	rule.Build(fmt.Sprintf("metalava_%s", params.stubConfig.stubsType.String()), "metalava merged")
}

func (d *Droidstubs) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	deps := d.Javadoc.collectDeps(ctx)

	javaVersion := getJavaVersion(ctx, String(d.Javadoc.properties.Java_version), android.SdkContext(d))
	generateStubs := BoolDefault(d.properties.Generate_stubs, true)

	// Add options for the other optional tasks: API-lint and check-released.
	// We generate separate timestamp files for them.
	doApiLint := BoolDefault(d.properties.Check_api.Api_lint.Enabled, false)
	doCheckReleased := apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released")

	writeSdkValues := Bool(d.properties.Write_sdk_values)

	annotationsEnabled := Bool(d.properties.Annotations_enabled)

	migratingNullability := annotationsEnabled && String(d.properties.Previous_api) != ""
	validatingNullability := annotationsEnabled && (strings.Contains(String(d.Javadoc.properties.Args), "--validate-nullability-from-merged-stubs") ||
		String(d.properties.Validate_nullability_from_list) != "")

	checkApi := apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") ||
		apiCheckEnabled(ctx, d.properties.Check_api.Last_released, "last_released")

	stubCmdParams := stubsCommandConfigParams{
		javaVersion:           javaVersion,
		deps:                  deps,
		checkApi:              checkApi,
		generateStubs:         generateStubs,
		doApiLint:             doApiLint,
		doCheckReleased:       doCheckReleased,
		writeSdkValues:        writeSdkValues,
		migratingNullability:  migratingNullability,
		validatingNullability: validatingNullability,
	}
	stubCmdParams.stubsType = Everything
	// Create default (i.e. "everything" stubs) rule for metalava
	d.everythingStubCmd(ctx, stubCmdParams)

	// The module generates "exportable" (and "runtime" eventually) stubs regardless of whether
	// aconfig_declarations property is defined or not. If the property is not defined, the module simply
	// strips all flagged apis to generate the "exportable" stubs
	stubCmdParams.stubsType = Exportable
	d.exportableStubCmd(ctx, stubCmdParams)

	if apiCheckEnabled(ctx, d.properties.Check_api.Current, "current") {

		if len(d.Javadoc.properties.Out) > 0 {
			ctx.PropertyErrorf("out", "out property may not be combined with check_api")
		}

		apiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Current.Api_file))
		removedApiFile := android.PathForModuleSrc(ctx, String(d.properties.Check_api.Current.Removed_api_file))
		baselineFile := android.OptionalPathForModuleSrc(ctx, d.properties.Check_api.Current.Baseline_file)

		if baselineFile.Valid() {
			ctx.PropertyErrorf("baseline_file", "current API check can't have a baseline file. (module %s)", ctx.ModuleName())
		}

		d.checkCurrentApiTimestamp = android.PathForModuleOut(ctx, Everything.String(), "check_current_api.timestamp")

		rule := android.NewRuleBuilder(pctx, ctx)

		// Diff command line.
		// -F matches the closest "opening" line, such as "package android {"
		// and "  public class Intent {".
		diff := `diff -u -F '{ *$'`

		rule.Command().Text("( true")
		rule.Command().
			Text(diff).
			Input(apiFile).Input(d.apiFile)

		rule.Command().
			Text(diff).
			Input(removedApiFile).Input(d.removedApiFile)

		msg := fmt.Sprintf(`\n******************************\n`+
			`You have tried to change the API from what has been previously approved.\n\n`+
			`To make these errors go away, you have two choices:\n`+
			`   1. You can add '@hide' javadoc comments (and remove @SystemApi/@TestApi/etc)\n`+
			`      to the new methods, etc. shown in the above diff.\n\n`+
			`   2. You can update current.txt and/or removed.txt by executing the following command:\n`+
			`         m %s-update-current-api\n\n`+
			`      To submit the revised current.txt to the main Android repository,\n`+
			`      you will need approval.\n`+
			`If your build failed due to stub validation, you can resolve the errors with\n`+
			`either of the two choices above and try re-building the target.\n`+
			`If the mismatch between the stubs and the current.txt is intended,\n`+
			`you can try re-building the target by executing the following command:\n`+
			`m DISABLE_STUB_VALIDATION=true <your build target>.\n`+
			`Note that DISABLE_STUB_VALIDATION=true does not bypass checkapi.\n`+
			`******************************\n`, ctx.ModuleName())

		rule.Command().
			Text("touch").Output(d.checkCurrentApiTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build("metalavaCurrentApiCheck", "check current API")

		d.updateCurrentApiTimestamp = android.PathForModuleOut(ctx, Everything.String(), "update_current_api.timestamp")

		// update API rule
		rule = android.NewRuleBuilder(pctx, ctx)

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

		rule.Build("metalavaCurrentApiUpdate", "update current API")
	}

	if String(d.properties.Check_nullability_warnings) != "" {
		if d.everythingArtifacts.nullabilityWarningsFile == nil {
			ctx.PropertyErrorf("check_nullability_warnings",
				"Cannot specify check_nullability_warnings unless validating nullability")
		}

		checkNullabilityWarnings := android.PathForModuleSrc(ctx, String(d.properties.Check_nullability_warnings))

		d.checkNullabilityWarningsTimestamp = android.PathForModuleOut(ctx, Everything.String(), "check_nullability_warnings.timestamp")

		msg := fmt.Sprintf(`\n******************************\n`+
			`The warnings encountered during nullability annotation validation did\n`+
			`not match the checked in file of expected warnings. The diffs are shown\n`+
			`above. You have two options:\n`+
			`   1. Resolve the differences by editing the nullability annotations.\n`+
			`   2. Update the file of expected warnings by running:\n`+
			`         cp %s %s\n`+
			`       and submitting the updated file as part of your change.`,
			d.everythingArtifacts.nullabilityWarningsFile, checkNullabilityWarnings)

		rule := android.NewRuleBuilder(pctx, ctx)

		rule.Command().
			Text("(").
			Text("diff").Input(checkNullabilityWarnings).Input(d.everythingArtifacts.nullabilityWarningsFile).
			Text("&&").
			Text("touch").Output(d.checkNullabilityWarningsTimestamp).
			Text(") || (").
			Text("echo").Flag("-e").Flag(`"` + msg + `"`).
			Text("; exit 38").
			Text(")")

		rule.Build("nullabilityWarningsCheck", "nullability warnings check")
	}
	android.CollectDependencyAconfigFiles(ctx, &d.mergedAconfigFiles)
}

func (d *Droidstubs) createApiContribution(ctx android.DefaultableHookContext) {
	api_file := d.properties.Check_api.Current.Api_file
	api_surface := d.properties.Api_surface

	props := struct {
		Name        *string
		Api_surface *string
		Api_file    *string
		Visibility  []string
	}{}

	props.Name = proptools.StringPtr(d.Name() + ".api.contribution")
	props.Api_surface = api_surface
	props.Api_file = api_file
	props.Visibility = []string{"//visibility:override", "//visibility:public"}

	ctx.CreateModule(ApiContributionFactory, &props)
}

// TODO (b/262014796): Export the API contributions of CorePlatformApi
// A map to populate the api surface of a droidstub from a substring appearing in its name
// This map assumes that droidstubs (either checked-in or created by java_sdk_library)
// use a strict naming convention
var (
	droidstubsModuleNamingToSdkKind = map[string]android.SdkKind{
		//public is commented out since the core libraries use public in their java_sdk_library names
		"intracore":     android.SdkIntraCore,
		"intra.core":    android.SdkIntraCore,
		"system_server": android.SdkSystemServer,
		"system-server": android.SdkSystemServer,
		"system":        android.SdkSystem,
		"module_lib":    android.SdkModule,
		"module-lib":    android.SdkModule,
		"platform.api":  android.SdkCorePlatform,
		"test":          android.SdkTest,
		"toolchain":     android.SdkToolchain,
	}
)

func StubsDefaultsFactory() android.Module {
	module := &DocDefaults{}

	module.AddProperties(
		&JavadocProperties{},
		&DroidstubsProperties{},
	)

	android.InitDefaultsModule(module)

	return module
}

var _ android.PrebuiltInterface = (*PrebuiltStubsSources)(nil)

type PrebuiltStubsSourcesProperties struct {
	Srcs []string `android:"path"`

	// Name of the source soong module that gets shadowed by this prebuilt
	// If unspecified, follows the naming convention that the source module of
	// the prebuilt is Name() without "prebuilt_" prefix
	Source_module_name *string

	// Non-nil if this prebuilt stub srcs  module was dynamically created by a java_sdk_library_import
	// The name is the undecorated name of the java_sdk_library as it appears in the blueprint file
	// (without any prebuilt_ prefix)
	Created_by_java_sdk_library_name *string `blueprint:"mutated"`
}

func (j *PrebuiltStubsSources) BaseModuleName() string {
	return proptools.StringDefault(j.properties.Source_module_name, j.ModuleBase.Name())
}

func (j *PrebuiltStubsSources) CreatedByJavaSdkLibraryName() *string {
	return j.properties.Created_by_java_sdk_library_name
}

type PrebuiltStubsSources struct {
	android.ModuleBase
	android.DefaultableModuleBase
	embeddableInModuleAndImport

	prebuilt android.Prebuilt

	properties PrebuiltStubsSourcesProperties

	stubsSrcJar android.Path
}

func (p *PrebuiltStubsSources) OutputFiles(tag string) (android.Paths, error) {
	switch tag {
	// prebuilt droidstubs does not output "exportable" stubs.
	// Output the "everything" stubs srcjar file if the tag is ".exportable".
	case "", ".exportable":
		return android.Paths{p.stubsSrcJar}, nil
	default:
		return nil, fmt.Errorf("unsupported module reference tag %q", tag)
	}
}

func (d *PrebuiltStubsSources) StubsSrcJar(_ StubsType) (android.Path, error) {
	return d.stubsSrcJar, nil
}

func (p *PrebuiltStubsSources) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	if len(p.properties.Srcs) != 1 {
		ctx.PropertyErrorf("srcs", "must only specify one directory path or srcjar, contains %d paths", len(p.properties.Srcs))
		return
	}

	src := p.properties.Srcs[0]
	if filepath.Ext(src) == ".srcjar" {
		// This is a srcjar. We can use it directly.
		p.stubsSrcJar = android.PathForModuleSrc(ctx, src)
	} else {
		outPath := android.PathForModuleOut(ctx, ctx.ModuleName()+"-"+"stubs.srcjar")

		// This is a directory. Glob the contents just in case the directory does not exist.
		srcGlob := src + "/**/*"
		srcPaths := android.PathsForModuleSrc(ctx, []string{srcGlob})

		// Although PathForModuleSrc can return nil if either the path doesn't exist or
		// the path components are invalid it won't in this case because no components
		// are specified and the module directory must exist in order to get this far.
		srcDir := android.PathForModuleSrc(ctx).(android.SourcePath).Join(ctx, src)

		rule := android.NewRuleBuilder(pctx, ctx)
		rule.Command().
			BuiltTool("soong_zip").
			Flag("-write_if_changed").
			Flag("-jar").
			FlagWithOutput("-o ", outPath).
			FlagWithArg("-C ", srcDir.String()).
			FlagWithRspFileInputList("-r ", outPath.ReplaceExtension(ctx, "rsp"), srcPaths)
		rule.Restat()
		rule.Build("zip src", "Create srcjar from prebuilt source")
		p.stubsSrcJar = outPath
	}
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
	module.initModuleAndImport(module)

	android.InitPrebuiltModule(module, &module.properties.Srcs)
	InitDroiddocModule(module, android.HostAndDeviceSupported)
	return module
}
