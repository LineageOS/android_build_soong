// Copyright 2017 Google Inc. All rights reserved.
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

package android

import (
	"path/filepath"
	"strings"
	"sync"

	"android/soong/bazel"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

const (
	canonicalPathFromRootDefault = true
)

// TODO(ccross): protos are often used to communicate between multiple modules.  If the only
// way to convert a proto to source is to reference it as a source file, and external modules cannot
// reference source files in other modules, then every module that owns a proto file will need to
// export a library for every type of external user (lite vs. full, c vs. c++ vs. java).  It would
// be better to support a proto module type that exported a proto file along with some include dirs,
// and then external modules could depend on the proto module but use their own settings to
// generate the source.

type ProtoFlags struct {
	Flags                 []string
	CanonicalPathFromRoot bool
	Dir                   ModuleGenPath
	SubDir                ModuleGenPath
	OutTypeFlag           string
	OutParams             []string
	Deps                  Paths
}

type protoDependencyTag struct {
	blueprint.BaseDependencyTag
	name string
}

var ProtoPluginDepTag = protoDependencyTag{name: "plugin"}

func ProtoDeps(ctx BottomUpMutatorContext, p *ProtoProperties) {
	if String(p.Proto.Plugin) != "" && String(p.Proto.Type) != "" {
		ctx.ModuleErrorf("only one of proto.type and proto.plugin can be specified.")
	}

	if plugin := String(p.Proto.Plugin); plugin != "" {
		ctx.AddFarVariationDependencies(ctx.Config().BuildOSTarget.Variations(),
			ProtoPluginDepTag, "protoc-gen-"+plugin)
	}
}

func GetProtoFlags(ctx ModuleContext, p *ProtoProperties) ProtoFlags {
	var flags []string
	var deps Paths

	if len(p.Proto.Local_include_dirs) > 0 {
		localProtoIncludeDirs := PathsForModuleSrc(ctx, p.Proto.Local_include_dirs)
		flags = append(flags, JoinWithPrefix(localProtoIncludeDirs.Strings(), "-I"))
	}
	if len(p.Proto.Include_dirs) > 0 {
		rootProtoIncludeDirs := PathsForSource(ctx, p.Proto.Include_dirs)
		flags = append(flags, JoinWithPrefix(rootProtoIncludeDirs.Strings(), "-I"))
	}

	ctx.VisitDirectDepsWithTag(ProtoPluginDepTag, func(dep Module) {
		if hostTool, ok := dep.(HostToolProvider); !ok || !hostTool.HostToolPath().Valid() {
			ctx.PropertyErrorf("proto.plugin", "module %q is not a host tool provider",
				ctx.OtherModuleName(dep))
		} else {
			plugin := String(p.Proto.Plugin)
			deps = append(deps, hostTool.HostToolPath().Path())
			flags = append(flags, "--plugin=protoc-gen-"+plugin+"="+hostTool.HostToolPath().String())
		}
	})

	var protoOutFlag string
	if plugin := String(p.Proto.Plugin); plugin != "" {
		protoOutFlag = "--" + plugin + "_out"
	}

	return ProtoFlags{
		Flags:                 flags,
		Deps:                  deps,
		OutTypeFlag:           protoOutFlag,
		CanonicalPathFromRoot: proptools.BoolDefault(p.Proto.Canonical_path_from_root, canonicalPathFromRootDefault),
		Dir:                   PathForModuleGen(ctx, "proto"),
		SubDir:                PathForModuleGen(ctx, "proto", ctx.ModuleDir()),
	}
}

type ProtoProperties struct {
	Proto struct {
		// Proto generator type.  C++: full or lite.  Java: micro, nano, stream, or lite.
		Type *string `android:"arch_variant"`

		// Proto plugin to use as the generator.  Must be a cc_binary_host module.
		Plugin *string `android:"arch_variant"`

		// list of directories that will be added to the protoc include paths.
		Include_dirs []string

		// list of directories relative to the bp file that will
		// be added to the protoc include paths.
		Local_include_dirs []string

		// whether to identify the proto files from the root of the
		// source tree (the original method in Android, useful for
		// android-specific protos), or relative from where they were
		// specified (useful for external/third party protos).
		//
		// This defaults to true today, but is expected to default to
		// false in the future.
		Canonical_path_from_root *bool
	} `android:"arch_variant"`
}

func ProtoRule(rule *RuleBuilder, protoFile Path, flags ProtoFlags, deps Paths,
	outDir WritablePath, depFile WritablePath, outputs WritablePaths) {

	var protoBase string
	if flags.CanonicalPathFromRoot {
		protoBase = "."
	} else {
		rel := protoFile.Rel()
		protoBase = strings.TrimSuffix(protoFile.String(), rel)
	}

	rule.Command().
		BuiltTool("aprotoc").
		FlagWithArg(flags.OutTypeFlag+"=", strings.Join(flags.OutParams, ",")+":"+outDir.String()).
		FlagWithDepFile("--dependency_out=", depFile).
		FlagWithArg("-I ", protoBase).
		Flags(flags.Flags).
		Input(protoFile).
		Implicits(deps).
		ImplicitOutputs(outputs)

	rule.Command().
		BuiltTool("dep_fixer").Flag(depFile.String())
}

// Bp2buildProtoInfo contains information necessary to pass on to language specific conversion.
type Bp2buildProtoInfo struct {
	Type                  *string
	Proto_libs            bazel.LabelList
	Transitive_proto_libs bazel.LabelList
}

type ProtoAttrs struct {
	Srcs                bazel.LabelListAttribute
	Import_prefix       *string
	Strip_import_prefix *string
	Deps                bazel.LabelListAttribute
}

// For each package in the include_dirs property a proto_library target should
// be added to the BUILD file in that package and a mapping should be added here
var includeDirsToProtoDeps = map[string]string{
	"external/protobuf/src": "//external/protobuf:libprotobuf-proto",
}

// Partitions srcs by the pkg it is in
// srcs has been created using `TransformSubpackagePaths`
// This function uses existence of Android.bp/BUILD files to create a label that is compatible with the package structure of bp2build workspace
func partitionSrcsByPackage(currentDir string, srcs bazel.LabelList) map[string]bazel.LabelList {
	getPackageFromLabel := func(label string) string {
		// Remove any preceding //
		label = strings.TrimPrefix(label, "//")
		split := strings.Split(label, ":")
		if len(split) == 1 {
			// e.g. foo.proto
			return currentDir
		} else if split[0] == "" {
			// e.g. :foo.proto
			return currentDir
		} else {
			return split[0]
		}
	}

	pkgToSrcs := map[string]bazel.LabelList{}
	for _, src := range srcs.Includes {
		pkg := getPackageFromLabel(src.Label)
		list := pkgToSrcs[pkg]
		list.Add(&src)
		pkgToSrcs[pkg] = list
	}
	return pkgToSrcs
}

// Bp2buildProtoProperties converts proto properties, creating a proto_library and returning the
// information necessary for language-specific handling.
func Bp2buildProtoProperties(ctx Bp2buildMutatorContext, m *ModuleBase, srcs bazel.LabelListAttribute) (Bp2buildProtoInfo, bool) {
	var info Bp2buildProtoInfo
	if srcs.IsEmpty() {
		return info, false
	}

	var protoLibraries bazel.LabelList
	var transitiveProtoLibraries bazel.LabelList
	var directProtoSrcs bazel.LabelList

	// For filegroups that should be converted to proto_library just collect the
	// labels of converted proto_library targets.
	for _, protoSrc := range srcs.Value.Includes {
		src := protoSrc.OriginalModuleName
		if fg, ok := ToFileGroupAsLibrary(ctx, src); ok &&
			fg.ShouldConvertToProtoLibrary(ctx) {
			protoLibraries.Add(&bazel.Label{
				Label: fg.GetProtoLibraryLabel(ctx),
			})
		} else {
			directProtoSrcs.Add(&protoSrc)
		}
	}

	name := m.Name() + "_proto"

	depsFromFilegroup := protoLibraries
	var canonicalPathFromRoot bool

	if len(directProtoSrcs.Includes) > 0 {
		pkgToSrcs := partitionSrcsByPackage(ctx.ModuleDir(), directProtoSrcs)
		protoIncludeDirs := []string{}
		for _, pkg := range SortedStringKeys(pkgToSrcs) {
			srcs := pkgToSrcs[pkg]
			attrs := ProtoAttrs{
				Srcs: bazel.MakeLabelListAttribute(srcs),
			}
			attrs.Deps.Append(bazel.MakeLabelListAttribute(depsFromFilegroup))

			for axis, configToProps := range m.GetArchVariantProperties(ctx, &ProtoProperties{}) {
				for _, rawProps := range configToProps {
					var props *ProtoProperties
					var ok bool
					if props, ok = rawProps.(*ProtoProperties); !ok {
						ctx.ModuleErrorf("Could not cast ProtoProperties to expected type")
					}
					if axis == bazel.NoConfigAxis {
						info.Type = props.Proto.Type

						canonicalPathFromRoot = proptools.BoolDefault(props.Proto.Canonical_path_from_root, canonicalPathFromRootDefault)
						if !canonicalPathFromRoot {
							// an empty string indicates to strips the package path
							path := ""
							attrs.Strip_import_prefix = &path
						}

						for _, dir := range props.Proto.Include_dirs {
							if dep, ok := includeDirsToProtoDeps[dir]; ok {
								attrs.Deps.Add(bazel.MakeLabelAttribute(dep))
							} else {
								protoIncludeDirs = append(protoIncludeDirs, dir)
							}
						}

						// proto.local_include_dirs are similar to proto.include_dirs, except that it is relative to the module directory
						for _, dir := range props.Proto.Local_include_dirs {
							relativeToTop := pathForModuleSrc(ctx, dir).String()
							protoIncludeDirs = append(protoIncludeDirs, relativeToTop)
						}

					} else if props.Proto.Type != info.Type && props.Proto.Type != nil {
						ctx.ModuleErrorf("Cannot handle arch-variant types for protos at this time.")
					}
				}
			}

			if p, ok := m.module.(PkgPathInterface); ok && p.PkgPath(ctx) != nil {
				// python_library with pkg_path
				// proto_library for this module should have the pkg_path as the import_prefix
				attrs.Import_prefix = p.PkgPath(ctx)
				attrs.Strip_import_prefix = proptools.StringPtr("")
			}

			tags := ApexAvailableTagsWithoutTestApexes(ctx, ctx.Module())

			moduleDir := ctx.ModuleDir()
			if !canonicalPathFromRoot {
				// Since we are creating the proto_library in a subpackage, set the import_prefix relative to the current package
				if rel, err := filepath.Rel(moduleDir, pkg); err != nil {
					ctx.ModuleErrorf("Could not get relative path for %v %v", pkg, err)
				} else if rel != "." {
					attrs.Import_prefix = &rel
				}
			}

			// TODO - b/246997908: Handle potential orphaned proto_library targets
			// To create proto_library targets in the same package, we split the .proto files
			// This means that if a proto_library in a subpackage imports another proto_library from the parent package
			// (or a different subpackage), it will not find it.
			// The CcProtoGen action itself runs fine because we construct the correct ProtoInfo,
			// but the FileDescriptorSet of each proto_library might not be compile-able
			//
			// Add manual tag if either
			// 1. .proto files are in more than one package
			// 2. proto.include_dirs is not empty
			if len(SortedStringKeys(pkgToSrcs)) > 1 || len(protoIncludeDirs) > 0 {
				tags.Append(bazel.MakeStringListAttribute([]string{"manual"}))
			}

			ctx.CreateBazelTargetModule(
				bazel.BazelTargetModuleProperties{Rule_class: "proto_library"},
				CommonAttributes{Name: name, Dir: proptools.StringPtr(pkg), Tags: tags},
				&attrs,
			)

			l := ""
			if pkg == moduleDir { // same package that the original module lives in
				l = ":" + name
			} else {
				l = "//" + pkg + ":" + name
			}
			protoLibraries.Add(&bazel.Label{
				Label: l,
			})
		}
		// Partitioning by packages can create dupes of protoIncludeDirs, so dedupe it first.
		protoLibrariesInIncludeDir := createProtoLibraryTargetsForIncludeDirs(ctx, SortedUniqueStrings(protoIncludeDirs))
		transitiveProtoLibraries.Append(protoLibrariesInIncludeDir)
	}

	info.Proto_libs = protoLibraries
	info.Transitive_proto_libs = transitiveProtoLibraries

	return info, true
}

// PkgPathInterface is used as a type assertion in bp2build to get pkg_path property of python_library_host
type PkgPathInterface interface {
	PkgPath(ctx BazelConversionContext) *string
}

var (
	protoIncludeDirGeneratedSuffix = ".include_dir_bp2build_generated_proto"
	protoIncludeDirsBp2buildKey    = NewOnceKey("protoIncludeDirsBp2build")
)

func getProtoIncludeDirsBp2build(config Config) *sync.Map {
	return config.Once(protoIncludeDirsBp2buildKey, func() interface{} {
		return &sync.Map{}
	}).(*sync.Map)
}

// key for dynamically creating proto_library per proto.include_dirs
type protoIncludeDirKey struct {
	dir            string
	subpackgeInDir string
}

// createProtoLibraryTargetsForIncludeDirs creates additional proto_library targets for .proto files in includeDirs
// Since Bazel imposes a constratint that the proto_library must be in the same package as the .proto file, this function
// might create the targets in a subdirectory of `includeDir`
// Returns the labels of the proto_library targets
func createProtoLibraryTargetsForIncludeDirs(ctx Bp2buildMutatorContext, includeDirs []string) bazel.LabelList {
	var ret bazel.LabelList
	for _, dir := range includeDirs {
		if exists, _, _ := ctx.Config().fs.Exists(filepath.Join(dir, "Android.bp")); !exists {
			ctx.ModuleErrorf("TODO: Add support for proto.include_dir: %v. This directory does not contain an Android.bp file", dir)
		}
		dirMap := getProtoIncludeDirsBp2build(ctx.Config())
		// Find all proto file targets in this dir
		protoLabelsInDir := BazelLabelForSrcPatternExcludes(ctx, dir, "**/*.proto", []string{})
		// Partition the labels by package and subpackage(s)
		protoLabelelsPartitionedByPkg := partitionSrcsByPackage(dir, protoLabelsInDir)
		for _, pkg := range SortedStringKeys(protoLabelelsPartitionedByPkg) {
			label := strings.ReplaceAll(dir, "/", ".") + protoIncludeDirGeneratedSuffix
			ret.Add(&bazel.Label{
				Label: "//" + pkg + ":" + label,
			})
			key := protoIncludeDirKey{dir: dir, subpackgeInDir: pkg}
			if _, exists := dirMap.LoadOrStore(key, true); exists {
				// A proto_library has already been created for this package relative to this include dir
				continue
			}
			srcs := protoLabelelsPartitionedByPkg[pkg]
			rel, err := filepath.Rel(dir, pkg)
			if err != nil {
				ctx.ModuleErrorf("Could not create a proto_library in pkg %v due to %v\n", pkg, err)
			}
			// Create proto_library
			attrs := ProtoAttrs{
				Srcs:                bazel.MakeLabelListAttribute(srcs),
				Strip_import_prefix: proptools.StringPtr(""),
			}
			if rel != "." {
				attrs.Import_prefix = proptools.StringPtr(rel)
			}

			// If a specific directory is listed in proto.include_dirs of two separate modules (one host-specific and another device-specific),
			// we do not want to create the proto_library with target_compatible_with of the first visited of these two modules
			// As a workarounds, delete `target_compatible_with`
			alwaysEnabled := bazel.BoolAttribute{}
			alwaysEnabled.Value = proptools.BoolPtr(true)
			// Add android and linux explicitly so that fillcommonbp2buildmoduleattrs can override these configs
			// When we extend b support for other os'es (darwin/windows), we should add those configs here as well
			alwaysEnabled.SetSelectValue(bazel.OsConfigurationAxis, bazel.OsAndroid, proptools.BoolPtr(true))
			alwaysEnabled.SetSelectValue(bazel.OsConfigurationAxis, bazel.OsLinux, proptools.BoolPtr(true))

			ctx.CreateBazelTargetModuleWithRestrictions(
				bazel.BazelTargetModuleProperties{Rule_class: "proto_library"},
				CommonAttributes{
					Name: label,
					Dir:  proptools.StringPtr(pkg),
					// This proto_library is used to construct a ProtoInfo
					// But it might not be buildable on its own
					Tags: bazel.MakeStringListAttribute([]string{"manual"}),
				},
				&attrs,
				alwaysEnabled,
			)
		}
	}
	return ret
}
