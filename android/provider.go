package android

import (
	"github.com/google/blueprint"
)

// OtherModuleProviderContext is a helper interface that is a subset of ModuleContext, BottomUpMutatorContext, or
// TopDownMutatorContext for use in OtherModuleProvider.
type OtherModuleProviderContext interface {
	otherModuleProvider(m blueprint.Module, provider blueprint.AnyProviderKey) (any, bool)
}

var _ OtherModuleProviderContext = BaseModuleContext(nil)
var _ OtherModuleProviderContext = ModuleContext(nil)
var _ OtherModuleProviderContext = BottomUpMutatorContext(nil)
var _ OtherModuleProviderContext = TopDownMutatorContext(nil)

// OtherModuleProvider reads the provider for the given module.  If the provider has been set the value is
// returned and the boolean is true.  If it has not been set the zero value of the provider's type  is returned
// and the boolean is false.  The value returned may be a deep copy of the value originally passed to SetProvider.
//
// OtherModuleProviderContext is a helper interface that accepts ModuleContext, BottomUpMutatorContext, or
// TopDownMutatorContext.
func OtherModuleProvider[K any](ctx OtherModuleProviderContext, module blueprint.Module, provider blueprint.ProviderKey[K]) (K, bool) {
	value, ok := ctx.otherModuleProvider(module, provider)
	if !ok {
		var k K
		return k, false
	}
	return value.(K), ok
}

// ModuleProviderContext is a helper interface that is a subset of ModuleContext, BottomUpMutatorContext, or
// TopDownMutatorContext for use in ModuleProvider.
type ModuleProviderContext interface {
	provider(provider blueprint.AnyProviderKey) (any, bool)
}

var _ ModuleProviderContext = BaseModuleContext(nil)
var _ ModuleProviderContext = ModuleContext(nil)
var _ ModuleProviderContext = BottomUpMutatorContext(nil)
var _ ModuleProviderContext = TopDownMutatorContext(nil)

// ModuleProvider reads the provider for the current module.  If the provider has been set the value is
// returned and the boolean is true.  If it has not been set the zero value of the provider's type  is returned
// and the boolean is false.  The value returned may be a deep copy of the value originally passed to SetProvider.
//
// ModuleProviderContext is a helper interface that accepts ModuleContext, BottomUpMutatorContext, or
// TopDownMutatorContext.
func ModuleProvider[K any](ctx ModuleProviderContext, provider blueprint.ProviderKey[K]) (K, bool) {
	value, ok := ctx.provider(provider)
	if !ok {
		var k K
		return k, false
	}
	return value.(K), ok
}

type SingletonModuleProviderContext interface {
	moduleProvider(blueprint.Module, blueprint.AnyProviderKey) (any, bool)
}

var _ SingletonModuleProviderContext = SingletonContext(nil)
var _ SingletonModuleProviderContext = (*TestContext)(nil)

// SingletonModuleProvider wraps blueprint.SingletonModuleProvider to provide a type-safe method to retrieve the value
// of the given provider from a module using a SingletonContext.  If the provider has not been set the first return
// value will be the zero value of the provider's type, and the second return value will be false.  If the provider has
// been set the second return value will be true.
func SingletonModuleProvider[K any](ctx SingletonModuleProviderContext, module blueprint.Module, provider blueprint.ProviderKey[K]) (K, bool) {
	value, ok := ctx.moduleProvider(module, provider)
	if !ok {
		var k K
		return k, false
	}
	return value.(K), ok
}

// SetProviderContext is a helper interface that is a subset of ModuleContext, BottomUpMutatorContext, or
// TopDownMutatorContext for use in SetProvider.
type SetProviderContext interface {
	setProvider(provider blueprint.AnyProviderKey, value any)
}

var _ SetProviderContext = BaseModuleContext(nil)
var _ SetProviderContext = ModuleContext(nil)
var _ SetProviderContext = BottomUpMutatorContext(nil)
var _ SetProviderContext = TopDownMutatorContext(nil)

// SetProvider sets the value for a provider for the current module.  It panics if not called
// during the appropriate mutator or GenerateBuildActions pass for the provider, if the value
// is not of the appropriate type, or if the value has already been set.  The value should not
// be modified after being passed to SetProvider.
//
// SetProviderContext is a helper interface that accepts ModuleContext, BottomUpMutatorContext, or
// TopDownMutatorContext.
func SetProvider[K any](ctx SetProviderContext, provider blueprint.ProviderKey[K], value K) {
	ctx.setProvider(provider, value)
}

var _ OtherModuleProviderContext = (*otherModuleProviderAdaptor)(nil)

// An OtherModuleProviderFunc can be passed to NewOtherModuleProviderAdaptor to create an OtherModuleProviderContext
// for use in tests.
type OtherModuleProviderFunc func(module blueprint.Module, provider blueprint.AnyProviderKey) (any, bool)

type otherModuleProviderAdaptor struct {
	otherModuleProviderFunc OtherModuleProviderFunc
}

func (p *otherModuleProviderAdaptor) otherModuleProvider(module blueprint.Module, provider blueprint.AnyProviderKey) (any, bool) {
	return p.otherModuleProviderFunc(module, provider)
}

// NewOtherModuleProviderAdaptor returns an OtherModuleProviderContext that proxies calls to otherModuleProvider to
// the provided OtherModuleProviderFunc.  It can be used in tests to unit test methods that need to call
// android.OtherModuleProvider.
func NewOtherModuleProviderAdaptor(otherModuleProviderFunc OtherModuleProviderFunc) OtherModuleProviderContext {
	return &otherModuleProviderAdaptor{otherModuleProviderFunc}
}
