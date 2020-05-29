// Copyright 2015 Google Inc. All rights reserved.
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
	"fmt"
	"strings"

	"github.com/google/blueprint"
)

// PackageContext is a wrapper for blueprint.PackageContext that adds
// some android-specific helper functions.
type PackageContext struct {
	blueprint.PackageContext
}

func NewPackageContext(pkgPath string) PackageContext {
	return PackageContext{blueprint.NewPackageContext(pkgPath)}
}

// configErrorWrapper can be used with Path functions when a Context is not
// available. A Config can be provided, and errors are stored as a list for
// later retrieval.
//
// The most common use here will be with VariableFunc, where only a config is
// provided, and an error should be returned.
type configErrorWrapper struct {
	pctx   PackageContext
	config Config
	errors []error
}

var _ PathContext = &configErrorWrapper{}
var _ errorfContext = &configErrorWrapper{}
var _ PackageVarContext = &configErrorWrapper{}
var _ PackagePoolContext = &configErrorWrapper{}
var _ PackageRuleContext = &configErrorWrapper{}

func (e *configErrorWrapper) Config() Config {
	return e.config
}
func (e *configErrorWrapper) Errorf(format string, args ...interface{}) {
	e.errors = append(e.errors, fmt.Errorf(format, args...))
}
func (e *configErrorWrapper) AddNinjaFileDeps(deps ...string) {
	e.pctx.AddNinjaFileDeps(deps...)
}

type PackageVarContext interface {
	PathContext
	errorfContext
}

type PackagePoolContext PackageVarContext
type PackageRuleContext PackageVarContext

// VariableFunc wraps blueprint.PackageContext.VariableFunc, converting the interface{} config
// argument to a PackageVarContext.
func (p PackageContext) VariableFunc(name string,
	f func(PackageVarContext) string) blueprint.Variable {

	return p.PackageContext.VariableFunc(name, func(config interface{}) (string, error) {
		ctx := &configErrorWrapper{p, config.(Config), nil}
		ret := f(ctx)
		if len(ctx.errors) > 0 {
			return "", ctx.errors[0]
		}
		return ret, nil
	})
}

// PoolFunc wraps blueprint.PackageContext.PoolFunc, converting the interface{} config
// argument to a Context that supports Config().
func (p PackageContext) PoolFunc(name string,
	f func(PackagePoolContext) blueprint.PoolParams) blueprint.Pool {

	return p.PackageContext.PoolFunc(name, func(config interface{}) (blueprint.PoolParams, error) {
		ctx := &configErrorWrapper{p, config.(Config), nil}
		params := f(ctx)
		if len(ctx.errors) > 0 {
			return params, ctx.errors[0]
		}
		return params, nil
	})
}

// RuleFunc wraps blueprint.PackageContext.RuleFunc, converting the interface{} config
// argument to a Context that supports Config(), and provides a default Pool if none is
// specified.
func (p PackageContext) RuleFunc(name string,
	f func(PackageRuleContext) blueprint.RuleParams, argNames ...string) blueprint.Rule {

	return p.PackageContext.RuleFunc(name, func(config interface{}) (blueprint.RuleParams, error) {
		ctx := &configErrorWrapper{p, config.(Config), nil}
		params := f(ctx)
		if len(ctx.errors) > 0 {
			return params, ctx.errors[0]
		}
		if ctx.Config().UseRemoteBuild() && params.Pool == nil {
			// When USE_GOMA=true or USE_RBE=true are set and the rule is not supported by
			// goma/RBE, restrict jobs to the local parallelism value
			params.Pool = localPool
		}
		return params, nil
	}, argNames...)
}

// SourcePathVariable returns a Variable whose value is the source directory
// appended with the supplied path. It may only be called during a Go package's
// initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p PackageContext) SourcePathVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(ctx PackageVarContext) string {
		p, err := safePathForSource(ctx, path)
		if err != nil {
			ctx.Errorf("%s", err.Error())
		}
		return p.String()
	})
}

// SourcePathsVariable returns a Variable whose value is the source directory
// appended with the supplied paths, joined with separator. It may only be
// called during a Go package's initialization - either from the init()
// function or as part of a package-scoped variable's initialization.
func (p PackageContext) SourcePathsVariable(name, separator string, paths ...string) blueprint.Variable {
	return p.VariableFunc(name, func(ctx PackageVarContext) string {
		var ret []string
		for _, path := range paths {
			p, err := safePathForSource(ctx, path)
			if err != nil {
				ctx.Errorf("%s", err.Error())
			}
			ret = append(ret, p.String())
		}
		return strings.Join(ret, separator)
	})
}

// SourcePathVariableWithEnvOverride returns a Variable whose value is the source directory
// appended with the supplied path, or the value of the given environment variable if it is set.
// The environment variable is not required to point to a path inside the source tree.
// It may only be called during a Go package's initialization - either from the init() function or
// as part of a package-scoped variable's initialization.
func (p PackageContext) SourcePathVariableWithEnvOverride(name, path, env string) blueprint.Variable {
	return p.VariableFunc(name, func(ctx PackageVarContext) string {
		p, err := safePathForSource(ctx, path)
		if err != nil {
			ctx.Errorf("%s", err.Error())
		}
		return ctx.Config().GetenvWithDefault(env, p.String())
	})
}

// HostBinToolVariable returns a Variable whose value is the path to a host tool
// in the bin directory for host targets. It may only be called during a Go
// package's initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p PackageContext) HostBinToolVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(ctx PackageVarContext) string {
		return ctx.Config().HostToolPath(ctx, path).String()
	})
}

// HostJNIToolVariable returns a Variable whose value is the path to a host tool
// in the lib directory for host targets. It may only be called during a Go
// package's initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p PackageContext) HostJNIToolVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(ctx PackageVarContext) string {
		return ctx.Config().HostJNIToolPath(ctx, path).String()
	})
}

// HostJavaToolVariable returns a Variable whose value is the path to a host
// tool in the frameworks directory for host targets. It may only be called
// during a Go package's initialization - either from the init() function or as
// part of a package-scoped variable's initialization.
func (p PackageContext) HostJavaToolVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(ctx PackageVarContext) string {
		return ctx.Config().HostJavaToolPath(ctx, path).String()
	})
}

// IntermediatesPathVariable returns a Variable whose value is the intermediate
// directory appended with the supplied path. It may only be called during a Go
// package's initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p PackageContext) IntermediatesPathVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(ctx PackageVarContext) string {
		return PathForIntermediates(ctx, path).String()
	})
}

// PrefixedExistentPathsForSourcesVariable returns a Variable whose value is the
// list of present source paths prefixed with the supplied prefix. It may only
// be called during a Go package's initialization - either from the init()
// function or as part of a package-scoped variable's initialization.
func (p PackageContext) PrefixedExistentPathsForSourcesVariable(
	name, prefix string, paths []string) blueprint.Variable {

	return p.VariableFunc(name, func(ctx PackageVarContext) string {
		paths := ExistentPathsForSources(ctx, paths)
		return JoinWithPrefix(paths.Strings(), prefix)
	})
}

// AndroidStaticRule is an alias for StaticRule.
func (p PackageContext) AndroidStaticRule(name string, params blueprint.RuleParams,
	argNames ...string) blueprint.Rule {
	return p.StaticRule(name, params, argNames...)
}

// StaticRule wraps blueprint.StaticRule and provides a default Pool if none is specified.
func (p PackageContext) StaticRule(name string, params blueprint.RuleParams,
	argNames ...string) blueprint.Rule {
	return p.RuleFunc(name, func(PackageRuleContext) blueprint.RuleParams {
		return params
	}, argNames...)
}

// RemoteRuleSupports configures rules with whether they have Goma and/or RBE support.
type RemoteRuleSupports struct {
	Goma bool
	RBE  bool
}

// AndroidRemoteStaticRule wraps blueprint.StaticRule but uses goma or RBE's parallelism if goma or RBE are enabled
// and the appropriate SUPPORTS_* flag is set.
func (p PackageContext) AndroidRemoteStaticRule(name string, supports RemoteRuleSupports, params blueprint.RuleParams,
	argNames ...string) blueprint.Rule {

	return p.PackageContext.RuleFunc(name, func(config interface{}) (blueprint.RuleParams, error) {
		ctx := &configErrorWrapper{p, config.(Config), nil}
		if ctx.Config().UseGoma() && !supports.Goma {
			// When USE_GOMA=true is set and the rule is not supported by goma, restrict jobs to the
			// local parallelism value
			params.Pool = localPool
		}

		if ctx.Config().UseRBE() && !supports.RBE {
			// When USE_RBE=true is set and the rule is not supported by RBE, restrict jobs to the
			// local parallelism value
			params.Pool = localPool
		}

		return params, nil
	}, argNames...)
}
