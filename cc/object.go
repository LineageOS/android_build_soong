// Copyright 2016 Google Inc. All rights reserved.
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

import (
	"fmt"
	"strings"

	"android/soong/android"
)

//
// Objects (for crt*.o)
//

func init() {
	android.RegisterModuleType("cc_object", ObjectFactory)
	android.RegisterSdkMemberType(ccObjectSdkMemberType)

}

var ccObjectSdkMemberType = &librarySdkMemberType{
	SdkMemberTypeBase: android.SdkMemberTypeBase{
		PropertyName: "native_objects",
		SupportsSdk:  true,
	},
	prebuiltModuleType: "cc_prebuilt_object",
}

type objectLinker struct {
	*baseLinker
	Properties ObjectLinkerProperties

	// Location of the object in the sysroot. Empty if the object is not
	// included in the NDK.
	ndkSysrootPath android.Path
}

type ObjectLinkerProperties struct {
	// list of static library modules that should only provide headers for this module.
	Static_libs []string `android:"arch_variant,variant_prepend"`

	// list of shared library modules should only provide headers for this module.
	Shared_libs []string `android:"arch_variant,variant_prepend"`

	// list of modules that should only provide headers for this module.
	Header_libs []string `android:"arch_variant,variant_prepend"`

	// list of default libraries that will provide headers for this module.  If unset, generally
	// defaults to libc, libm, and libdl.  Set to [] to prevent using headers from the defaults.
	System_shared_libs []string `android:"arch_variant"`

	// names of other cc_object modules to link into this module using partial linking
	Objs []string `android:"arch_variant"`

	// if set, add an extra objcopy --prefix-symbols= step
	Prefix_symbols *string

	// if set, the path to a linker script to pass to ld -r when combining multiple object files.
	Linker_script *string `android:"path,arch_variant"`

	// Indicates that this module is a CRT object. CRT objects will be split
	// into a variant per-API level between min_sdk_version and current.
	Crt *bool

	// Indicates that this module should not be included in the NDK sysroot.
	// Only applies to CRT objects. Defaults to false.
	Exclude_from_ndk_sysroot *bool
}

func newObject(hod android.HostOrDeviceSupported) *Module {
	module := newBaseModule(hod, android.MultilibBoth)
	module.sanitize = &sanitize{}
	module.stl = &stl{}
	return module
}

// cc_object runs the compiler without running the linker. It is rarely
// necessary, but sometimes used to generate .s files from .c files to use as
// input to a cc_genrule module.
func ObjectFactory() android.Module {
	module := newObject(android.HostAndDeviceSupported)
	module.linker = &objectLinker{
		baseLinker: NewBaseLinker(module.sanitize),
	}
	module.compiler = NewBaseCompiler()

	// Clang's address-significance tables are incompatible with ld -r.
	module.compiler.appendCflags([]string{"-fno-addrsig"})

	module.sdkMemberTypes = []android.SdkMemberType{ccObjectSdkMemberType}

	return module.Init()
}

func (object *objectLinker) appendLdflags(flags []string) {
	panic(fmt.Errorf("appendLdflags on objectLinker not supported"))
}

func (object *objectLinker) linkerProps() []interface{} {
	return []interface{}{&object.Properties}
}

func (*objectLinker) linkerInit(ctx BaseModuleContext) {}

func (object *objectLinker) linkerDeps(ctx DepsContext, deps Deps) Deps {
	deps.HeaderLibs = append(deps.HeaderLibs, object.Properties.Header_libs...)
	deps.SharedLibs = append(deps.SharedLibs, object.Properties.Shared_libs...)
	deps.StaticLibs = append(deps.StaticLibs, object.Properties.Static_libs...)
	deps.ObjFiles = append(deps.ObjFiles, object.Properties.Objs...)

	deps.SystemSharedLibs = object.Properties.System_shared_libs
	if deps.SystemSharedLibs == nil {
		// Provide a default set of shared libraries if system_shared_libs is unspecified.
		// Note: If an empty list [] is specified, it implies that the module declines the
		// default shared libraries.
		deps.SystemSharedLibs = append(deps.SystemSharedLibs, ctx.toolchain().DefaultSharedLibraries()...)
	}
	deps.LateSharedLibs = append(deps.LateSharedLibs, deps.SystemSharedLibs...)
	return deps
}

func (object *objectLinker) linkerFlags(ctx ModuleContext, flags Flags) Flags {
	flags.Global.LdFlags = append(flags.Global.LdFlags, ctx.toolchain().ToolchainLdflags())

	if lds := android.OptionalPathForModuleSrc(ctx, object.Properties.Linker_script); lds.Valid() {
		flags.Local.LdFlags = append(flags.Local.LdFlags, "-Wl,-T,"+lds.String())
		flags.LdFlagsDeps = append(flags.LdFlagsDeps, lds.Path())
	}
	return flags
}

func (object *objectLinker) link(ctx ModuleContext,
	flags Flags, deps PathDeps, objs Objects) android.Path {

	objs = objs.Append(deps.Objs)

	var output android.WritablePath
	builderFlags := flagsToBuilderFlags(flags)
	outputName := ctx.ModuleName()
	if !strings.HasSuffix(outputName, objectExtension) {
		outputName += objectExtension
	}

	// isForPlatform is terribly named and actually means isNotApex.
	if Bool(object.Properties.Crt) &&
		!Bool(object.Properties.Exclude_from_ndk_sysroot) && ctx.useSdk() &&
		ctx.isSdkVariant() && ctx.isForPlatform() {

		output = getVersionedLibraryInstallPath(ctx,
			nativeApiLevelOrPanic(ctx, ctx.sdkVersion())).Join(ctx, outputName)
		object.ndkSysrootPath = output
	} else {
		output = android.PathForModuleOut(ctx, outputName)
	}

	outputFile := output

	if len(objs.objFiles) == 1 && String(object.Properties.Linker_script) == "" {
		if String(object.Properties.Prefix_symbols) != "" {
			transformBinaryPrefixSymbols(ctx, String(object.Properties.Prefix_symbols), objs.objFiles[0],
				builderFlags, output)
		} else {
			ctx.Build(pctx, android.BuildParams{
				Rule:   android.Cp,
				Input:  objs.objFiles[0],
				Output: output,
			})
		}
	} else {
		outputAddrSig := android.PathForModuleOut(ctx, "addrsig", outputName)

		if String(object.Properties.Prefix_symbols) != "" {
			input := android.PathForModuleOut(ctx, "unprefixed", outputName)
			transformBinaryPrefixSymbols(ctx, String(object.Properties.Prefix_symbols), input,
				builderFlags, output)
			output = input
		}

		transformObjsToObj(ctx, objs.objFiles, builderFlags, outputAddrSig, flags.LdFlagsDeps)

		// ld -r reorders symbols and invalidates the .llvm_addrsig section, which then causes warnings
		// if the resulting object is used with ld --icf=safe.  Strip the .llvm_addrsig section to
		// prevent the warnings.
		transformObjectNoAddrSig(ctx, outputAddrSig, output)
	}

	ctx.CheckbuildFile(outputFile)
	return outputFile
}

func (object *objectLinker) linkerSpecifiedDeps(specifiedDeps specifiedDeps) specifiedDeps {
	specifiedDeps.sharedLibs = append(specifiedDeps.sharedLibs, object.Properties.Shared_libs...)

	// Must distinguish nil and [] in system_shared_libs - ensure that [] in
	// either input list doesn't come out as nil.
	if specifiedDeps.systemSharedLibs == nil {
		specifiedDeps.systemSharedLibs = object.Properties.System_shared_libs
	} else {
		specifiedDeps.systemSharedLibs = append(specifiedDeps.systemSharedLibs, object.Properties.System_shared_libs...)
	}

	return specifiedDeps
}

func (object *objectLinker) unstrippedOutputFilePath() android.Path {
	return nil
}

func (object *objectLinker) strippedAllOutputFilePath() android.Path {
	panic("Not implemented.")
}

func (object *objectLinker) nativeCoverage() bool {
	return true
}

func (object *objectLinker) coverageOutputFilePath() android.OptionalPath {
	return android.OptionalPath{}
}

func (object *objectLinker) object() bool {
	return true
}

func (object *objectLinker) isCrt() bool {
	return Bool(object.Properties.Crt)
}
