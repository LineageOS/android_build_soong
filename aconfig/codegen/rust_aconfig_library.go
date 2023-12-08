package codegen

import (
	"fmt"

	"android/soong/aconfig"
	"android/soong/android"
	"android/soong/rust"

	"github.com/google/blueprint"
	"github.com/google/blueprint/proptools"
)

type rustDeclarationsTagType struct {
	blueprint.BaseDependencyTag
}

var rustDeclarationsTag = rustDeclarationsTagType{}

type RustAconfigLibraryProperties struct {
	// name of the aconfig_declarations module to generate a library for
	Aconfig_declarations string

	// default mode is "production", the other accepted modes are:
	// "test": to generate test mode version of the library
	// "exported": to generate exported mode version of the library
	// an error will be thrown if the mode is not supported
	Mode *string
}

type aconfigDecorator struct {
	*rust.BaseSourceProvider

	Properties RustAconfigLibraryProperties
}

func NewRustAconfigLibrary(hod android.HostOrDeviceSupported) (*rust.Module, *aconfigDecorator) {
	aconfig := &aconfigDecorator{
		BaseSourceProvider: rust.NewSourceProvider(),
		Properties:         RustAconfigLibraryProperties{},
	}

	module := rust.NewSourceProviderModule(android.HostAndDeviceSupported, aconfig, false, false)
	return module, aconfig
}

// rust_aconfig_library generates aconfig rust code from the provided aconfig declaration. This module type will
// create library variants that can be used as a crate dependency by adding it to the rlibs, dylibs, and rustlibs
// properties of other modules.
func RustAconfigLibraryFactory() android.Module {
	module, _ := NewRustAconfigLibrary(android.HostAndDeviceSupported)
	return module.Init()
}

func (a *aconfigDecorator) SourceProviderProps() []interface{} {
	return append(a.BaseSourceProvider.SourceProviderProps(), &a.Properties)
}

func (a *aconfigDecorator) GenerateSource(ctx rust.ModuleContext, deps rust.PathDeps) android.Path {
	generatedDir := android.PathForModuleGen(ctx)
	generatedSource := android.PathForModuleGen(ctx, "src", "lib.rs")

	declarationsModules := ctx.GetDirectDepsWithTag(rustDeclarationsTag)

	if len(declarationsModules) != 1 {
		panic(fmt.Errorf("Exactly one aconfig_declarations property required"))
	}
	declarations := ctx.OtherModuleProvider(declarationsModules[0], aconfig.DeclarationsProviderKey).(aconfig.DeclarationsProviderData)

	mode := proptools.StringDefault(a.Properties.Mode, "production")
	if !isModeSupported(mode) {
		ctx.PropertyErrorf("mode", "%q is not a supported mode", mode)
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:  rustRule,
		Input: declarations.IntermediateCacheOutputPath,
		Outputs: []android.WritablePath{
			generatedSource,
		},
		Description: "rust_aconfig_library",
		Args: map[string]string{
			"gendir": generatedDir.String(),
			"mode":   mode,
		},
	})
	a.BaseSourceProvider.OutputFiles = android.Paths{generatedSource}
	return generatedSource
}

func (a *aconfigDecorator) SourceProviderDeps(ctx rust.DepsContext, deps rust.Deps) rust.Deps {
	deps = a.BaseSourceProvider.SourceProviderDeps(ctx, deps)
	deps.Rustlibs = append(deps.Rustlibs, "libflags_rust")
	deps.Rustlibs = append(deps.Rustlibs, "liblazy_static")
	ctx.AddDependency(ctx.Module(), rustDeclarationsTag, a.Properties.Aconfig_declarations)
	return deps
}
