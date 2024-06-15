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
	"github.com/google/blueprint"
)

// SingletonContext
type SingletonContext interface {
	blueprintSingletonContext() blueprint.SingletonContext

	Config() Config
	DeviceConfig() DeviceConfig

	ModuleName(module blueprint.Module) string
	ModuleDir(module blueprint.Module) string
	ModuleSubDir(module blueprint.Module) string
	ModuleType(module blueprint.Module) string
	BlueprintFile(module blueprint.Module) string

	// ModuleVariantsFromName returns the list of module variants named `name` in the same namespace as `referer` enforcing visibility rules.
	// Allows generating build actions for `referer` based on the metadata for `name` deferred until the singleton context.
	ModuleVariantsFromName(referer Module, name string) []Module

	moduleProvider(module blueprint.Module, provider blueprint.AnyProviderKey) (any, bool)

	ModuleErrorf(module blueprint.Module, format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Failed() bool

	Variable(pctx PackageContext, name, value string)
	Rule(pctx PackageContext, name string, params blueprint.RuleParams, argNames ...string) blueprint.Rule
	Build(pctx PackageContext, params BuildParams)

	// Phony creates a Make-style phony rule, a rule with no commands that can depend on other
	// phony rules or real files.  Phony can be called on the same name multiple times to add
	// additional dependencies.
	Phony(name string, deps ...Path)

	RequireNinjaVersion(major, minor, micro int)

	// SetOutDir sets the value of the top-level "builddir" Ninja variable
	// that controls where Ninja stores its build log files.  This value can be
	// set at most one time for a single build, later calls are ignored.
	SetOutDir(pctx PackageContext, value string)

	// Eval takes a string with embedded ninja variables, and returns a string
	// with all of the variables recursively expanded. Any variables references
	// are expanded in the scope of the PackageContext.
	Eval(pctx PackageContext, ninjaStr string) (string, error)

	VisitAllModulesBlueprint(visit func(blueprint.Module))
	VisitAllModules(visit func(Module))
	VisitAllModulesIf(pred func(Module) bool, visit func(Module))

	VisitDirectDeps(module Module, visit func(Module))
	VisitDirectDepsIf(module Module, pred func(Module) bool, visit func(Module))

	// Deprecated: use WalkDeps instead to support multiple dependency tags on the same module
	VisitDepsDepthFirst(module Module, visit func(Module))
	// Deprecated: use WalkDeps instead to support multiple dependency tags on the same module
	VisitDepsDepthFirstIf(module Module, pred func(Module) bool,
		visit func(Module))

	VisitAllModuleVariants(module Module, visit func(Module))

	PrimaryModule(module Module) Module
	FinalModule(module Module) Module

	AddNinjaFileDeps(deps ...string)

	// GlobWithDeps returns a list of files that match the specified pattern but do not match any
	// of the patterns in excludes.  It also adds efficient dependencies to rerun the primary
	// builder whenever a file matching the pattern as added or removed, without rerunning if a
	// file that does not match the pattern is added to a searched directory.
	GlobWithDeps(pattern string, excludes []string) ([]string, error)
}

type singletonAdaptor struct {
	Singleton

	buildParams []BuildParams
	ruleParams  map[blueprint.Rule]blueprint.RuleParams
}

var _ testBuildProvider = (*singletonAdaptor)(nil)

func (s *singletonAdaptor) GenerateBuildActions(ctx blueprint.SingletonContext) {
	sctx := &singletonContextAdaptor{SingletonContext: ctx}
	if sctx.Config().captureBuild {
		sctx.ruleParams = make(map[blueprint.Rule]blueprint.RuleParams)
	}

	s.Singleton.GenerateBuildActions(sctx)

	s.buildParams = sctx.buildParams
	s.ruleParams = sctx.ruleParams
}

func (s *singletonAdaptor) BuildParamsForTests() []BuildParams {
	return s.buildParams
}

func (s *singletonAdaptor) RuleParamsForTests() map[blueprint.Rule]blueprint.RuleParams {
	return s.ruleParams
}

type Singleton interface {
	GenerateBuildActions(SingletonContext)
}

type singletonContextAdaptor struct {
	blueprint.SingletonContext

	buildParams []BuildParams
	ruleParams  map[blueprint.Rule]blueprint.RuleParams
}

func (s *singletonContextAdaptor) blueprintSingletonContext() blueprint.SingletonContext {
	return s.SingletonContext
}

func (s *singletonContextAdaptor) Config() Config {
	return s.SingletonContext.Config().(Config)
}

func (s *singletonContextAdaptor) DeviceConfig() DeviceConfig {
	return DeviceConfig{s.Config().deviceConfig}
}

func (s *singletonContextAdaptor) Variable(pctx PackageContext, name, value string) {
	s.SingletonContext.Variable(pctx.PackageContext, name, value)
}

func (s *singletonContextAdaptor) Rule(pctx PackageContext, name string, params blueprint.RuleParams, argNames ...string) blueprint.Rule {
	if s.Config().UseRemoteBuild() {
		if params.Pool == nil {
			// When USE_GOMA=true or USE_RBE=true are set and the rule is not supported by goma/RBE, restrict
			// jobs to the local parallelism value
			params.Pool = localPool
		} else if params.Pool == remotePool {
			// remotePool is a fake pool used to identify rule that are supported for remoting. If the rule's
			// pool is the remotePool, replace with nil so that ninja runs it at NINJA_REMOTE_NUM_JOBS
			// parallelism.
			params.Pool = nil
		}
	}
	rule := s.SingletonContext.Rule(pctx.PackageContext, name, params, argNames...)
	if s.Config().captureBuild {
		s.ruleParams[rule] = params
	}
	return rule
}

func (s *singletonContextAdaptor) Build(pctx PackageContext, params BuildParams) {
	if s.Config().captureBuild {
		s.buildParams = append(s.buildParams, params)
	}
	bparams := convertBuildParams(params)
	s.SingletonContext.Build(pctx.PackageContext, bparams)
}

func (s *singletonContextAdaptor) Phony(name string, deps ...Path) {
	addPhony(s.Config(), name, deps...)
}

func (s *singletonContextAdaptor) SetOutDir(pctx PackageContext, value string) {
	s.SingletonContext.SetOutDir(pctx.PackageContext, value)
}

func (s *singletonContextAdaptor) Eval(pctx PackageContext, ninjaStr string) (string, error) {
	return s.SingletonContext.Eval(pctx.PackageContext, ninjaStr)
}

// visitAdaptor wraps a visit function that takes an android.Module parameter into
// a function that takes an blueprint.Module parameter and only calls the visit function if the
// blueprint.Module is an android.Module.
func visitAdaptor(visit func(Module)) func(blueprint.Module) {
	return func(module blueprint.Module) {
		if aModule, ok := module.(Module); ok {
			visit(aModule)
		}
	}
}

// predAdaptor wraps a pred function that takes an android.Module parameter
// into a function that takes an blueprint.Module parameter and only calls the visit function if the
// blueprint.Module is an android.Module, otherwise returns false.
func predAdaptor(pred func(Module) bool) func(blueprint.Module) bool {
	return func(module blueprint.Module) bool {
		if aModule, ok := module.(Module); ok {
			return pred(aModule)
		} else {
			return false
		}
	}
}

func (s *singletonContextAdaptor) VisitAllModulesBlueprint(visit func(blueprint.Module)) {
	s.SingletonContext.VisitAllModules(visit)
}

func (s *singletonContextAdaptor) VisitAllModules(visit func(Module)) {
	s.SingletonContext.VisitAllModules(visitAdaptor(visit))
}

func (s *singletonContextAdaptor) VisitAllModulesIf(pred func(Module) bool, visit func(Module)) {
	s.SingletonContext.VisitAllModulesIf(predAdaptor(pred), visitAdaptor(visit))
}

func (s *singletonContextAdaptor) VisitDirectDeps(module Module, visit func(Module)) {
	s.SingletonContext.VisitDirectDeps(module, visitAdaptor(visit))
}

func (s *singletonContextAdaptor) VisitDirectDepsIf(module Module, pred func(Module) bool, visit func(Module)) {
	s.SingletonContext.VisitDirectDepsIf(module, predAdaptor(pred), visitAdaptor(visit))
}

func (s *singletonContextAdaptor) VisitDepsDepthFirst(module Module, visit func(Module)) {
	s.SingletonContext.VisitDepsDepthFirst(module, visitAdaptor(visit))
}

func (s *singletonContextAdaptor) VisitDepsDepthFirstIf(module Module, pred func(Module) bool, visit func(Module)) {
	s.SingletonContext.VisitDepsDepthFirstIf(module, predAdaptor(pred), visitAdaptor(visit))
}

func (s *singletonContextAdaptor) VisitAllModuleVariants(module Module, visit func(Module)) {
	s.SingletonContext.VisitAllModuleVariants(module, visitAdaptor(visit))
}

func (s *singletonContextAdaptor) PrimaryModule(module Module) Module {
	return s.SingletonContext.PrimaryModule(module).(Module)
}

func (s *singletonContextAdaptor) FinalModule(module Module) Module {
	return s.SingletonContext.FinalModule(module).(Module)
}

func (s *singletonContextAdaptor) ModuleVariantsFromName(referer Module, name string) []Module {
	// get module reference for visibility enforcement
	qualified := createVisibilityModuleReference(s.ModuleName(referer), s.ModuleDir(referer), s.ModuleType(referer))

	modules := s.SingletonContext.ModuleVariantsFromName(referer, name)
	result := make([]Module, 0, len(modules))
	for _, m := range modules {
		if module, ok := m.(Module); ok {
			// enforce visibility
			depName := s.ModuleName(module)
			depDir := s.ModuleDir(module)
			depQualified := qualifiedModuleName{depDir, depName}
			// Targets are always visible to other targets in their own package.
			if depQualified.pkg != qualified.name.pkg {
				rule := effectiveVisibilityRules(s.Config(), depQualified)
				if !rule.matches(qualified) {
					s.ModuleErrorf(referer, "module %q references %q which is not visible to this module\nYou may need to add %q to its visibility",
						referer.Name(), depQualified, "//"+s.ModuleDir(referer))
					continue
				}
			}
			result = append(result, module)
		}
	}
	return result
}

func (s *singletonContextAdaptor) moduleProvider(module blueprint.Module, provider blueprint.AnyProviderKey) (any, bool) {
	return s.SingletonContext.ModuleProvider(module, provider)
}
