package android

import (
	"github.com/google/blueprint/proptools"
	"sync"
)

func init() {
	RegisterModuleType("override_module", OverrideModuleFactory)
}

type OverrideModule struct {
	ModuleBase
	properties OverrideModuleProperties
}

type OverrideModuleProperties struct {
	// base module to override
	Base *string

	// file path or module name (in the form ":module") of a certificate to override with
	Certificate *string

	// manifest package name to override with
	Manifest_package_name *string
}

// TODO(jungjw): Work with the mainline team to see if we can deprecate all PRODUCT_*_OVERRIDES vars
// and hand over overriding values directly to base module code.
func processOverrides(ctx LoadHookContext, p *OverrideModuleProperties) {
	base := proptools.String(p.Base)
	if base == "" {
		ctx.PropertyErrorf("base", "base module name must be provided")
	}

	config := ctx.DeviceConfig()
	if other, loaded := config.moduleNameOverrides().LoadOrStore(base, ctx.ModuleName()); loaded {
		ctx.ModuleErrorf("multiple overriding modules for %q, the other: %q", base, other.(string))
	}

	if p.Certificate != nil {
		config.certificateOverrides().Store(base, *p.Certificate)
	}

	if p.Manifest_package_name != nil {
		config.manifestPackageNameOverrides().Store(base, *p.Manifest_package_name)
	}
}

func (i *OverrideModule) DepsMutator(ctx BottomUpMutatorContext) {
	base := *i.properties.Base
	// Right now, we add a dependency only to check the base module exists, and so are not using a tag here.
	// TODO(jungjw): Add a tag and check the base module type once we finalize supported base module types.
	ctx.AddDependency(ctx.Module(), nil, base)
}

func (i *OverrideModule) GenerateAndroidBuildActions(ctx ModuleContext) {
	// All the overrides happen in the base module.
	// TODO(jungjw): Check the base module type.
}

// override_module overrides an existing module with the specified properties.
//
// Currently, only android_app is officially supported.
func OverrideModuleFactory() Module {
	m := &OverrideModule{}
	AddLoadHook(m, func(ctx LoadHookContext) {
		processOverrides(ctx, &m.properties)
	})
	m.AddProperties(&m.properties)
	InitAndroidModule(m)
	return m
}

var moduleNameOverridesKey = NewOnceKey("moduleNameOverrides")

func (c *deviceConfig) moduleNameOverrides() *sync.Map {
	return c.Once(moduleNameOverridesKey, func() interface{} {
		return &sync.Map{}
	}).(*sync.Map)
}

var certificateOverridesKey = NewOnceKey("certificateOverrides")

func (c *deviceConfig) certificateOverrides() *sync.Map {
	return c.Once(certificateOverridesKey, func() interface{} {
		return &sync.Map{}
	}).(*sync.Map)
}

var manifestPackageNameOverridesKey = NewOnceKey("manifestPackageNameOverrides")

func (c *deviceConfig) manifestPackageNameOverrides() *sync.Map {
	return c.Once(manifestPackageNameOverridesKey, func() interface{} {
		return &sync.Map{}
	}).(*sync.Map)
}
