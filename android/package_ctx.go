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
	"github.com/google/blueprint/pathtools"
)

// AndroidPackageContext is a wrapper for blueprint.PackageContext that adds
// some android-specific helper functions.
type AndroidPackageContext struct {
	blueprint.PackageContext
}

func NewPackageContext(pkgPath string) AndroidPackageContext {
	return AndroidPackageContext{blueprint.NewPackageContext(pkgPath)}
}

// configErrorWrapper can be used with Path functions when a Context is not
// available. A Config can be provided, and errors are stored as a list for
// later retrieval.
//
// The most common use here will be with VariableFunc, where only a config is
// provided, and an error should be returned.
type configErrorWrapper struct {
	pctx   AndroidPackageContext
	config Config
	errors []error
}

var _ PathContext = &configErrorWrapper{}
var _ errorfContext = &configErrorWrapper{}

func (e *configErrorWrapper) Config() interface{} {
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

// SourcePathVariable returns a Variable whose value is the source directory
// appended with the supplied path. It may only be called during a Go package's
// initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p AndroidPackageContext) SourcePathVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(config interface{}) (string, error) {
		ctx := &configErrorWrapper{p, config.(Config), []error{}}
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
func (p AndroidPackageContext) SourcePathsVariable(name, separator string, paths ...string) blueprint.Variable {
	return p.VariableFunc(name, func(config interface{}) (string, error) {
		ctx := &configErrorWrapper{p, config.(Config), []error{}}
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
func (p AndroidPackageContext) SourcePathVariableWithEnvOverride(name, path, env string) blueprint.Variable {
	return p.VariableFunc(name, func(config interface{}) (string, error) {
		ctx := &configErrorWrapper{p, config.(Config), []error{}}
		p := safePathForSource(ctx, path)
		if len(ctx.errors) > 0 {
			return "", ctx.errors[0]
		}
		return config.(Config).GetenvWithDefault(env, p.String()), nil
	})
}

// HostBinVariable returns a Variable whose value is the path to a host tool
// in the bin directory for host targets. It may only be called during a Go
// package's initialization - either from the init() function or as part of a
// package-scoped variable's initialization.
func (p AndroidPackageContext) HostBinToolVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(config interface{}) (string, error) {
		po, err := p.HostBinToolPath(config, path)
		if err != nil {
			return "", err
		}
		return po.String(), nil
	})
}

func (p AndroidPackageContext) HostBinToolPath(config interface{}, path string) (Path, error) {
	ctx := &configErrorWrapper{p, config.(Config), []error{}}
	pa := PathForOutput(ctx, "host", ctx.config.PrebuiltOS(), "bin", path)
	if len(ctx.errors) > 0 {
		return nil, ctx.errors[0]
	}
	return pa, nil
}

// HostJavaToolVariable returns a Variable whose value is the path to a host
// tool in the frameworks directory for host targets. It may only be called
// during a Go package's initialization - either from the init() function or as
// part of a package-scoped variable's initialization.
func (p AndroidPackageContext) HostJavaToolVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(config interface{}) (string, error) {
		ctx := &configErrorWrapper{p, config.(Config), []error{}}
		p := PathForOutput(ctx, "host", ctx.config.PrebuiltOS(), "framework", path)
		if len(ctx.errors) > 0 {
			return "", ctx.errors[0]
		}
		return p.String(), nil
	})
}

func (p AndroidPackageContext) HostJavaToolPath(config interface{}, path string) (Path, error) {
	ctx := &configErrorWrapper{p, config.(Config), []error{}}
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
func (p AndroidPackageContext) IntermediatesPathVariable(name, path string) blueprint.Variable {
	return p.VariableFunc(name, func(config interface{}) (string, error) {
		ctx := &configErrorWrapper{p, config.(Config), []error{}}
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
func (p AndroidPackageContext) PrefixedExistentPathsForSourcesVariable(
	name, prefix string, paths []string) blueprint.Variable {

	return p.VariableFunc(name, func(config interface{}) (string, error) {
		ctx := &configErrorWrapper{p, config.(Config), []error{}}
		paths := ExistentPathsForSources(ctx, "", paths)
		if len(ctx.errors) > 0 {
			return "", ctx.errors[0]
		}
		return JoinWithPrefix(paths.Strings(), prefix), nil
	})
}

type RuleParams struct {
	blueprint.RuleParams
	GomaSupported bool
}

// AndroidStaticRule wraps blueprint.StaticRule and provides a default Pool if none is specified
func (p AndroidPackageContext) AndroidStaticRule(name string, params blueprint.RuleParams,
	argNames ...string) blueprint.Rule {
	return p.AndroidRuleFunc(name, func(Config) (blueprint.RuleParams, error) {
		return params, nil
	}, argNames...)
}

// AndroidGomaStaticRule wraps blueprint.StaticRule but uses goma's parallelism if goma is enabled
func (p AndroidPackageContext) AndroidGomaStaticRule(name string, params blueprint.RuleParams,
	argNames ...string) blueprint.Rule {
	return p.StaticRule(name, params, argNames...)
}

func (p AndroidPackageContext) AndroidRuleFunc(name string,
	f func(Config) (blueprint.RuleParams, error), argNames ...string) blueprint.Rule {
	return p.PackageContext.RuleFunc(name, func(config interface{}) (blueprint.RuleParams, error) {
		params, err := f(config.(Config))
		if config.(Config).UseGoma() && params.Pool == nil {
			// When USE_GOMA=true is set and the rule is not supported by goma, restrict jobs to the
			// local parallelism value
			params.Pool = localPool
		}
		return params, err
	}, argNames...)
}
