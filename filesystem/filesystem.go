// Copyright (C) 2020 The Android Open Source Project
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

package filesystem

import (
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"android/soong/android"
	"android/soong/cc"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

func init() {
	registerBuildComponents(android.InitRegistrationContext)
}

func registerBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("android_filesystem", filesystemFactory)
	ctx.RegisterModuleType("android_system_image", systemImageFactory)
	ctx.RegisterModuleType("avb_add_hash_footer", avbAddHashFooterFactory)
	ctx.RegisterModuleType("avb_add_hash_footer_defaults", avbAddHashFooterDefaultsFactory)
	ctx.RegisterModuleType("avb_gen_vbmeta_image", avbGenVbmetaImageFactory)
	ctx.RegisterModuleType("avb_gen_vbmeta_image_defaults", avbGenVbmetaImageDefaultsFactory)
}

type filesystem struct {
	android.ModuleBase
	android.PackagingBase

	properties filesystemProperties

	// Function that builds extra files under the root directory and returns the files
	buildExtraFiles func(ctx android.ModuleContext, root android.OutputPath) android.OutputPaths

	// Function that filters PackagingSpecs returned by PackagingBase.GatherPackagingSpecs()
	filterPackagingSpecs func(specs map[string]android.PackagingSpec)

	output     android.OutputPath
	installDir android.InstallPath

	// For testing. Keeps the result of CopyDepsToZip()
	entries []string
}

type symlinkDefinition struct {
	Target *string
	Name   *string
}

type filesystemProperties struct {
	// When set to true, sign the image with avbtool. Default is false.
	Use_avb *bool

	// Path to the private key that avbtool will use to sign this filesystem image.
	// TODO(jiyong): allow apex_key to be specified here
	Avb_private_key *string `android:"path"`

	// Signing algorithm for avbtool. Default is SHA256_RSA4096.
	Avb_algorithm *string

	// Hash algorithm used for avbtool (for descriptors). This is passed as hash_algorithm to
	// avbtool. Default used by avbtool is sha1.
	Avb_hash_algorithm *string

	// Name of the partition stored in vbmeta desc. Defaults to the name of this module.
	Partition_name *string

	// Type of the filesystem. Currently, ext4, cpio, and compressed_cpio are supported. Default
	// is ext4.
	Type *string

	// file_contexts file to make image. Currently, only ext4 is supported.
	File_contexts *string `android:"path"`

	// Base directory relative to root, to which deps are installed, e.g. "system". Default is "."
	// (root).
	Base_dir *string

	// Directories to be created under root. e.g. /dev, /proc, etc.
	Dirs []string

	// Symbolic links to be created under root with "ln -sf <target> <name>".
	Symlinks []symlinkDefinition

	// Seconds since unix epoch to override timestamps of file entries
	Fake_timestamp *string

	// When set, passed to mkuserimg_mke2fs --mke2fs_uuid & --mke2fs_hash_seed.
	// Otherwise, they'll be set as random which might cause indeterministic build output.
	Uuid *string

	// Mount point for this image. Default is "/"
	Mount_point *string
}

// android_filesystem packages a set of modules and their transitive dependencies into a filesystem
// image. The filesystem images are expected to be mounted in the target device, which means the
// modules in the filesystem image are built for the target device (i.e. Android, not Linux host).
// The modules are placed in the filesystem image just like they are installed to the ordinary
// partitions like system.img. For example, cc_library modules are placed under ./lib[64] directory.
func filesystemFactory() android.Module {
	module := &filesystem{}
	initFilesystemModule(module)
	return module
}

func initFilesystemModule(module *filesystem) {
	module.AddProperties(&module.properties)
	android.InitPackageModule(module)
	android.InitAndroidMultiTargetsArchModule(module, android.DeviceSupported, android.MultilibCommon)
}

var dependencyTag = struct {
	blueprint.BaseDependencyTag
	android.PackagingItemAlwaysDepTag
}{}

func (f *filesystem) DepsMutator(ctx android.BottomUpMutatorContext) {
	f.AddDeps(ctx, dependencyTag)
}

type fsType int

const (
	ext4Type fsType = iota
	compressedCpioType
	cpioType // uncompressed
	unknown
)

func (f *filesystem) fsType(ctx android.ModuleContext) fsType {
	typeStr := proptools.StringDefault(f.properties.Type, "ext4")
	switch typeStr {
	case "ext4":
		return ext4Type
	case "compressed_cpio":
		return compressedCpioType
	case "cpio":
		return cpioType
	default:
		ctx.PropertyErrorf("type", "%q not supported", typeStr)
		return unknown
	}
}

func (f *filesystem) installFileName() string {
	return f.BaseModuleName() + ".img"
}

var pctx = android.NewPackageContext("android/soong/filesystem")

func (f *filesystem) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	switch f.fsType(ctx) {
	case ext4Type:
		f.output = f.buildImageUsingBuildImage(ctx)
	case compressedCpioType:
		f.output = f.buildCpioImage(ctx, true)
	case cpioType:
		f.output = f.buildCpioImage(ctx, false)
	default:
		return
	}

	f.installDir = android.PathForModuleInstall(ctx, "etc")
	ctx.InstallFile(f.installDir, f.installFileName(), f.output)
}

// root zip will contain extra files/dirs that are not from the `deps` property.
func (f *filesystem) buildRootZip(ctx android.ModuleContext) android.OutputPath {
	rootDir := android.PathForModuleGen(ctx, "root").OutputPath
	builder := android.NewRuleBuilder(pctx, ctx)
	builder.Command().Text("rm -rf").Text(rootDir.String())
	builder.Command().Text("mkdir -p").Text(rootDir.String())

	// create dirs and symlinks
	for _, dir := range f.properties.Dirs {
		// OutputPath.Join verifies dir
		builder.Command().Text("mkdir -p").Text(rootDir.Join(ctx, dir).String())
	}

	for _, symlink := range f.properties.Symlinks {
		name := strings.TrimSpace(proptools.String(symlink.Name))
		target := strings.TrimSpace(proptools.String(symlink.Target))

		if name == "" {
			ctx.PropertyErrorf("symlinks", "Name can't be empty")
			continue
		}

		if target == "" {
			ctx.PropertyErrorf("symlinks", "Target can't be empty")
			continue
		}

		// OutputPath.Join verifies name. don't need to verify target.
		dst := rootDir.Join(ctx, name)

		builder.Command().Text("mkdir -p").Text(filepath.Dir(dst.String()))
		builder.Command().Text("ln -sf").Text(proptools.ShellEscape(target)).Text(dst.String())
	}

	// create extra files if there's any
	rootForExtraFiles := android.PathForModuleGen(ctx, "root-extra").OutputPath
	var extraFiles android.OutputPaths
	if f.buildExtraFiles != nil {
		extraFiles = f.buildExtraFiles(ctx, rootForExtraFiles)
		for _, f := range extraFiles {
			rel, _ := filepath.Rel(rootForExtraFiles.String(), f.String())
			if strings.HasPrefix(rel, "..") {
				panic(fmt.Errorf("%q is not under %q\n", f, rootForExtraFiles))
			}
		}
	}

	// Zip them all
	zipOut := android.PathForModuleGen(ctx, "root.zip").OutputPath
	zipCommand := builder.Command().BuiltTool("soong_zip")
	zipCommand.FlagWithOutput("-o ", zipOut).
		FlagWithArg("-C ", rootDir.String()).
		Flag("-L 0"). // no compression because this will be unzipped soon
		FlagWithArg("-D ", rootDir.String()).
		Flag("-d") // include empty directories
	if len(extraFiles) > 0 {
		zipCommand.FlagWithArg("-C ", rootForExtraFiles.String())
		for _, f := range extraFiles {
			zipCommand.FlagWithInput("-f ", f)
		}
	}

	builder.Command().Text("rm -rf").Text(rootDir.String())

	builder.Build("zip_root", fmt.Sprintf("zipping root contents for %s", ctx.ModuleName()))
	return zipOut
}

func (f *filesystem) buildImageUsingBuildImage(ctx android.ModuleContext) android.OutputPath {
	depsZipFile := android.PathForModuleOut(ctx, "deps.zip").OutputPath
	f.entries = f.CopyDepsToZip(ctx, f.gatherFilteredPackagingSpecs(ctx), depsZipFile)

	builder := android.NewRuleBuilder(pctx, ctx)
	depsBase := proptools.StringDefault(f.properties.Base_dir, ".")
	rebasedDepsZip := android.PathForModuleOut(ctx, "rebased_deps.zip").OutputPath
	builder.Command().
		BuiltTool("zip2zip").
		FlagWithInput("-i ", depsZipFile).
		FlagWithOutput("-o ", rebasedDepsZip).
		Text("**/*:" + proptools.ShellEscape(depsBase)) // zip2zip verifies depsBase

	rootDir := android.PathForModuleOut(ctx, "root").OutputPath
	rootZip := f.buildRootZip(ctx)
	builder.Command().
		BuiltTool("zipsync").
		FlagWithArg("-d ", rootDir.String()). // zipsync wipes this. No need to clear.
		Input(rootZip).
		Input(rebasedDepsZip)

	// run host_init_verifier
	// Ideally we should have a concept of pluggable linters that verify the generated image.
	// While such concept is not implement this will do.
	// TODO(b/263574231): substitute with pluggable linter.
	builder.Command().
		BuiltTool("host_init_verifier").
		FlagWithArg("--out_system=", rootDir.String()+"/system")

	propFile, toolDeps := f.buildPropFile(ctx)
	output := android.PathForModuleOut(ctx, f.installFileName()).OutputPath
	builder.Command().BuiltTool("build_image").
		Text(rootDir.String()). // input directory
		Input(propFile).
		Implicits(toolDeps).
		Output(output).
		Text(rootDir.String()) // directory where to find fs_config_files|dirs

	// rootDir is not deleted. Might be useful for quick inspection.
	builder.Build("build_filesystem_image", fmt.Sprintf("Creating filesystem %s", f.BaseModuleName()))

	return output
}

func (f *filesystem) buildFileContexts(ctx android.ModuleContext) android.OutputPath {
	builder := android.NewRuleBuilder(pctx, ctx)
	fcBin := android.PathForModuleOut(ctx, "file_contexts.bin")
	builder.Command().BuiltTool("sefcontext_compile").
		FlagWithOutput("-o ", fcBin).
		Input(android.PathForModuleSrc(ctx, proptools.String(f.properties.File_contexts)))
	builder.Build("build_filesystem_file_contexts", fmt.Sprintf("Creating filesystem file contexts for %s", f.BaseModuleName()))
	return fcBin.OutputPath
}

// Calculates avb_salt from entry list (sorted) for deterministic output.
func (f *filesystem) salt() string {
	return sha1sum(f.entries)
}

func (f *filesystem) buildPropFile(ctx android.ModuleContext) (propFile android.OutputPath, toolDeps android.Paths) {
	type prop struct {
		name  string
		value string
	}

	var props []prop
	var deps android.Paths
	addStr := func(name string, value string) {
		props = append(props, prop{name, value})
	}
	addPath := func(name string, path android.Path) {
		props = append(props, prop{name, path.String()})
		deps = append(deps, path)
	}

	// Type string that build_image.py accepts.
	fsTypeStr := func(t fsType) string {
		switch t {
		// TODO(jiyong): add more types like f2fs, erofs, etc.
		case ext4Type:
			return "ext4"
		}
		panic(fmt.Errorf("unsupported fs type %v", t))
	}

	addStr("fs_type", fsTypeStr(f.fsType(ctx)))
	addStr("mount_point", proptools.StringDefault(f.properties.Mount_point, "/"))
	addStr("use_dynamic_partition_size", "true")
	addPath("ext_mkuserimg", ctx.Config().HostToolPath(ctx, "mkuserimg_mke2fs"))
	// b/177813163 deps of the host tools have to be added. Remove this.
	for _, t := range []string{"mke2fs", "e2fsdroid", "tune2fs"} {
		deps = append(deps, ctx.Config().HostToolPath(ctx, t))
	}

	if proptools.Bool(f.properties.Use_avb) {
		addStr("avb_hashtree_enable", "true")
		addPath("avb_avbtool", ctx.Config().HostToolPath(ctx, "avbtool"))
		algorithm := proptools.StringDefault(f.properties.Avb_algorithm, "SHA256_RSA4096")
		addStr("avb_algorithm", algorithm)
		key := android.PathForModuleSrc(ctx, proptools.String(f.properties.Avb_private_key))
		addPath("avb_key_path", key)
		partitionName := proptools.StringDefault(f.properties.Partition_name, f.Name())
		addStr("partition_name", partitionName)
		avb_add_hashtree_footer_args := "--do_not_generate_fec"
		if hashAlgorithm := proptools.String(f.properties.Avb_hash_algorithm); hashAlgorithm != "" {
			avb_add_hashtree_footer_args += " --hash_algorithm " + hashAlgorithm
		}
		securityPatchKey := "com.android.build." + partitionName + ".security_patch"
		securityPatchValue := ctx.Config().PlatformSecurityPatch()
		avb_add_hashtree_footer_args += " --prop " + securityPatchKey + ":" + securityPatchValue
		addStr("avb_add_hashtree_footer_args", avb_add_hashtree_footer_args)
		addStr("avb_salt", f.salt())
	}

	if proptools.String(f.properties.File_contexts) != "" {
		addPath("selinux_fc", f.buildFileContexts(ctx))
	}
	if timestamp := proptools.String(f.properties.Fake_timestamp); timestamp != "" {
		addStr("timestamp", timestamp)
	}
	if uuid := proptools.String(f.properties.Uuid); uuid != "" {
		addStr("uuid", uuid)
		addStr("hash_seed", uuid)
	}
	propFile = android.PathForModuleOut(ctx, "prop").OutputPath
	builder := android.NewRuleBuilder(pctx, ctx)
	builder.Command().Text("rm").Flag("-rf").Output(propFile)
	for _, p := range props {
		builder.Command().
			Text("echo").
			Flag(`"` + p.name + "=" + p.value + `"`).
			Text(">>").Output(propFile)
	}
	builder.Build("build_filesystem_prop", fmt.Sprintf("Creating filesystem props for %s", f.BaseModuleName()))
	return propFile, deps
}

func (f *filesystem) buildCpioImage(ctx android.ModuleContext, compressed bool) android.OutputPath {
	if proptools.Bool(f.properties.Use_avb) {
		ctx.PropertyErrorf("use_avb", "signing compresed cpio image using avbtool is not supported."+
			"Consider adding this to bootimg module and signing the entire boot image.")
	}

	if proptools.String(f.properties.File_contexts) != "" {
		ctx.PropertyErrorf("file_contexts", "file_contexts is not supported for compressed cpio image.")
	}

	depsZipFile := android.PathForModuleOut(ctx, "deps.zip").OutputPath
	f.entries = f.CopyDepsToZip(ctx, f.gatherFilteredPackagingSpecs(ctx), depsZipFile)

	builder := android.NewRuleBuilder(pctx, ctx)
	depsBase := proptools.StringDefault(f.properties.Base_dir, ".")
	rebasedDepsZip := android.PathForModuleOut(ctx, "rebased_deps.zip").OutputPath
	builder.Command().
		BuiltTool("zip2zip").
		FlagWithInput("-i ", depsZipFile).
		FlagWithOutput("-o ", rebasedDepsZip).
		Text("**/*:" + proptools.ShellEscape(depsBase)) // zip2zip verifies depsBase

	rootDir := android.PathForModuleOut(ctx, "root").OutputPath
	rootZip := f.buildRootZip(ctx)
	builder.Command().
		BuiltTool("zipsync").
		FlagWithArg("-d ", rootDir.String()). // zipsync wipes this. No need to clear.
		Input(rootZip).
		Input(rebasedDepsZip)

	output := android.PathForModuleOut(ctx, f.installFileName()).OutputPath
	cmd := builder.Command().
		BuiltTool("mkbootfs").
		Text(rootDir.String()) // input directory
	if compressed {
		cmd.Text("|").
			BuiltTool("lz4").
			Flag("--favor-decSpeed"). // for faster boot
			Flag("-12").              // maximum compression level
			Flag("-l").               // legacy format for kernel
			Text(">").Output(output)
	} else {
		cmd.Text(">").Output(output)
	}

	// rootDir is not deleted. Might be useful for quick inspection.
	builder.Build("build_cpio_image", fmt.Sprintf("Creating filesystem %s", f.BaseModuleName()))

	return output
}

var _ android.AndroidMkEntriesProvider = (*filesystem)(nil)

// Implements android.AndroidMkEntriesProvider
func (f *filesystem) AndroidMkEntries() []android.AndroidMkEntries {
	return []android.AndroidMkEntries{android.AndroidMkEntries{
		Class:      "ETC",
		OutputFile: android.OptionalPathForPath(f.output),
		ExtraEntries: []android.AndroidMkExtraEntriesFunc{
			func(ctx android.AndroidMkExtraEntriesContext, entries *android.AndroidMkEntries) {
				entries.SetString("LOCAL_MODULE_PATH", f.installDir.String())
				entries.SetString("LOCAL_INSTALLED_MODULE_STEM", f.installFileName())
			},
		},
	}}
}

var _ android.OutputFileProducer = (*filesystem)(nil)

// Implements android.OutputFileProducer
func (f *filesystem) OutputFiles(tag string) (android.Paths, error) {
	if tag == "" {
		return []android.Path{f.output}, nil
	}
	return nil, fmt.Errorf("unsupported module reference tag %q", tag)
}

// Filesystem is the public interface for the filesystem struct. Currently, it's only for the apex
// package to have access to the output file.
type Filesystem interface {
	android.Module
	OutputPath() android.Path

	// Returns the output file that is signed by avbtool. If this module is not signed, returns
	// nil.
	SignedOutputPath() android.Path
}

var _ Filesystem = (*filesystem)(nil)

func (f *filesystem) OutputPath() android.Path {
	return f.output
}

func (f *filesystem) SignedOutputPath() android.Path {
	if proptools.Bool(f.properties.Use_avb) {
		return f.OutputPath()
	}
	return nil
}

// Filter the result of GatherPackagingSpecs to discard items targeting outside "system" partition.
// Note that "apex" module installs its contents to "apex"(fake partition) as well
// for symbol lookup by imitating "activated" paths.
func (f *filesystem) gatherFilteredPackagingSpecs(ctx android.ModuleContext) map[string]android.PackagingSpec {
	specs := f.PackagingBase.GatherPackagingSpecs(ctx)
	if f.filterPackagingSpecs != nil {
		f.filterPackagingSpecs(specs)
	}
	return specs
}

func sha1sum(values []string) string {
	h := sha256.New()
	for _, value := range values {
		io.WriteString(h, value)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Base cc.UseCoverage

var _ cc.UseCoverage = (*filesystem)(nil)

func (*filesystem) IsNativeCoverageNeeded(ctx android.BaseModuleContext) bool {
	return ctx.Device() && ctx.DeviceConfig().NativeCoverageEnabled()
}
