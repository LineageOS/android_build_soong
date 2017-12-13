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
	"runtime"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
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

func (e *configErrorWrapper) Config() Config {
	return e.config
}
func (e *configErrorWrapper) Errorf(format string, args ...interface{}) {
	e.errors = append(e.errors, fmt.Errorf(format, args...))
}
func (e *configErrorWrapper) AddNinjaFileDeps(deps ...string) {
	e.pctx.AddNinjaFileDeps(deps...)
}

func (e *configErrorWrapper) Fs() pathtools.FileSystem {
	return nil
}

// VariableFunc wraps blueprint.PackageContext.VariableFunc, converting the interface{} config
// argument to a Config.
func (p PackageContext) VariableFunc(name string,
	f func(Config) (string, error)) blueprint.Variable {

	return p.PackageContext.VariableFunc(name, func(config interface{}) (string, error) {
		return f(config.(Config))
	})
}

// PoolFunc wraps blueprint.PackageContext.PoolFunc, converting the interface{} config
// argument to a Config.
func (p PackageContext) PoolFunc(name string,
	f func(Config) (blueprint.PoolParams, error)) blueprint.Pool {

	return p.PackageContext.PoolFunc(name, func(config interface{}) (blueprint.PoolParams, error) {
		return f(config.(Config))
	})
}

// RuleFunc wraps blueprint.PackageContext.RuleFunc, converting the interface{} config
// argument to a Config.
func (p PackageContext) RuleFunc(name string,
	f func(Config) (blueprint.RuleParams, error), argNames ...string) blueprint.Rule {

	return p.PackageContext.RuleFunc(name, func(config interface{}) (blueprint.RuleParams, error) {
		return f(config.(Config))
	}, argNames...)
}

// SourcePathVariable returns a Variable whose value is the source directory
// appended with the supplied path. It may only be called during a Go package's
// initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p PackageContext) SourcePathVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(config Config) (string, error) {
		ctx := &configErrorWrapper{p, config, []error{}}
		p := safePathForSource(ctx, path)
		if len(ctx.errors) > 0 {
			return "", ctx.errors[0]
		}
		return p.String(), nil
	})
}

// SourcePathsVariable returns a Variable whose value is the source directory
// appended with the supplied paths, joined with separator. It may only be
// called during a Go package's initialization - either from the init()
// function or as part of a package-scoped variable's initialization.
func (p PackageContext) SourcePathsVariable(name, separator string, paths ...string) blueprint.Variable {
	return p.VariableFunc(name, func(config Config) (string, error) {
		ctx := &configErrorWrapper{p, config, []error{}}
		var ret []string
		for _, path := range paths {
			p := safePathForSource(ctx, path)
			if len(ctx.errors) > 0 {
				return "", ctx.errors[0]
			}
			ret = append(ret, p.String())
		}
		return strings.Join(ret, separator), nil
	})
}

// SourcePathVariableWithEnvOverride returns a Variable whose value is the source directory
// appended with the supplied path, or the value of the given environment variable if it is set.
// The environment variable is not required to point to a path inside the source tree.
// It may only be called during a Go package's initialization - either from the init() function or
// as part of a package-scoped variable's initialization.
func (p PackageContext) SourcePathVariableWithEnvOverride(name, path, env string) blueprint.Variable {
	return p.VariableFunc(name, func(config Config) (string, error) {
		ctx := &configErrorWrapper{p, config, []error{}}
		p := safePathForSource(ctx, path)
		if len(ctx.errors) > 0 {
			return "", ctx.errors[0]
		}
		return config.GetenvWithDefault(env, p.String()), nil
	})
}

// HostBinToolVariable returns a Variable whose value is the path to a host tool
// in the bin directory for host targets. It may only be called during a Go
// package's initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p PackageContext) HostBinToolVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(config Config) (string, error) {
		po, err := p.HostBinToolPath(config, path)
		if err != nil {
			return "", err
		}
		return po.String(), nil
	})
}

func (p PackageContext) HostBinToolPath(config Config, path string) (Path, error) {
	ctx := &configErrorWrapper{p, config, []error{}}
	pa := PathForOutput(ctx, "host", ctx.config.PrebuiltOS(), "bin", path)
	if len(ctx.errors) > 0 {
		return nil, ctx.errors[0]
	}
	return pa, nil
}

// HostJNIToolVariable returns a Variable whose value is the path to a host tool
// in the lib directory for host targets. It may only be called during a Go
// package's initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p PackageContext) HostJNIToolVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(config Config) (string, error) {
		po, err := p.HostJNIToolPath(config, path)
		if err != nil {
			return "", err
		}
		return po.String(), nil
	})
}

func (p PackageContext) HostJNIToolPath(config Config, path string) (Path, error) {
	ctx := &configErrorWrapper{p, config, []error{}}
	ext := ".so"
	if runtime.GOOS == "darwin" {
		ext = ".dylib"
	}
	pa := PathForOutput(ctx, "host", ctx.config.PrebuiltOS(), "lib64", path+ext)
	if len(ctx.errors) > 0 {
		return nil, ctx.errors[0]
	}
	return pa, nil
}

// HostJavaToolVariable returns a Variable whose value is the path to a host
// tool in the frameworks directory for host targets. It may only be called
// during a Go package's initialization - either from the init() function or as
// part of a package-scoped variable's initialization.
func (p PackageContext) HostJavaToolVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(config Config) (string, error) {
		ctx := &configErrorWrapper{p, config, []error{}}
		p := PathForOutput(ctx, "host", ctx.config.PrebuiltOS(), "framework", path)
		if len(ctx.errors) > 0 {
			return "", ctx.errors[0]
		}
		return p.String(), nil
	})
}

func (p PackageContext) HostJavaToolPath(config Config, path string) (Path, error) {
	ctx := &configErrorWrapper{p, config, []error{}}
	pa := PathForOutput(ctx, "host", ctx.config.PrebuiltOS(), "framework", path)
	if len(ctx.errors) > 0 {
		return nil, ctx.errors[0]
	}
	return pa, nil
}

// IntermediatesPathVariable returns a Variable whose value is the intermediate
// directory appended with the supplied path. It may only be called during a Go
// package's initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p PackageContext) IntermediatesPathVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(config Config) (string, error) {
		ctx := &configErrorWrapper{p, config, []error{}}
		p := PathForIntermediates(ctx, path)
		if len(ctx.errors) > 0 {
			return "", ctx.errors[0]
		}
		return p.String(), nil
	})
}

// PrefixedExistentPathsForSourcesVariable returns a Variable whose value is the
// list of present source paths prefixed with the supplied prefix. It may only
// be called during a Go package's initialization - either from the init()
// function or as part of a package-scoped variable's initialization.
func (p PackageContext) PrefixedExistentPathsForSourcesVariable(
	name, prefix string, paths []string) blueprint.Variable {

	return p.VariableFunc(name, func(config Config) (string, error) {
		ctx := &configErrorWrapper{p, config, []error{}}
		paths := ExistentPathsForSources(ctx, "", paths)
		if len(ctx.errors) > 0 {
			return "", ctx.errors[0]
		}
		return JoinWithPrefix(paths.Strings(), prefix), nil
	})
}

// AndroidStaticRule wraps blueprint.StaticRule and provides a default Pool if none is specified
func (p PackageContext) AndroidStaticRule(name string, params blueprint.RuleParams,
	argNames ...string) blueprint.Rule {
	return p.AndroidRuleFunc(name, func(Config) (blueprint.RuleParams, error) {
		return params, nil
	}, argNames...)
}

// AndroidGomaStaticRule wraps blueprint.StaticRule but uses goma's parallelism if goma is enabled
func (p PackageContext) AndroidGomaStaticRule(name string, params blueprint.RuleParams,
	argNames ...string) blueprint.Rule {
	return p.StaticRule(name, params, argNames...)
}

func (p PackageContext) AndroidRuleFunc(name string,
	f func(Config) (blueprint.RuleParams, error), argNames ...string) blueprint.Rule {
	return p.RuleFunc(name, func(config Config) (blueprint.RuleParams, error) {
		params, err := f(config)
		if config.UseGoma() && params.Pool == nil {
			// When USE_GOMA=true is set and the rule is not supported by goma, restrict jobs to the
			// local parallelism value
			params.Pool = localPool
		}
		return params, err
	}, argNames...)
}
