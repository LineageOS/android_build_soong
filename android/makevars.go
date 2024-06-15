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

package android

import (
	"bytes"
	"cmp"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
	"github.com/google/blueprint/proptools"
)

func init() {
	RegisterMakeVarsProvider(pctx, androidMakeVarsProvider)
}

func androidMakeVarsProvider(ctx MakeVarsContext) {
	ctx.Strict("MIN_SUPPORTED_SDK_VERSION", ctx.Config().MinSupportedSdkVersion().String())
}

// /////////////////////////////////////////////////////////////////////////////

// BaseMakeVarsContext contains the common functions for other packages to use
// to declare make variables
type BaseMakeVarsContext interface {
	Config() Config
	DeviceConfig() DeviceConfig
	AddNinjaFileDeps(deps ...string)

	Failed() bool

	// These are equivalent to Strict and Check, but do not attempt to
	// evaluate the values before writing them to the Makefile. They can
	// be used when all ninja variables have already been evaluated through
	// Eval().
	StrictRaw(name, value string)
	CheckRaw(name, value string)

	// GlobWithDeps returns a list of files that match the specified pattern but do not match any
	// of the patterns in excludes.  It also adds efficient dependencies to rerun the primary
	// builder whenever a file matching the pattern as added or removed, without rerunning if a
	// file that does not match the pattern is added to a searched directory.
	GlobWithDeps(pattern string, excludes []string) ([]string, error)

	// Phony creates a phony rule in Make, which will allow additional DistForGoal
	// dependencies to be added to it.  Phony can be called on the same name multiple
	// times to add additional dependencies.
	Phony(names string, deps ...Path)

	// DistForGoal creates a rule to copy one or more Paths to the artifacts
	// directory on the build server when the specified goal is built.
	DistForGoal(goal string, paths ...Path)

	// DistForGoalWithFilename creates a rule to copy a Path to the artifacts
	// directory on the build server with the given filename when the specified
	// goal is built.
	DistForGoalWithFilename(goal string, path Path, filename string)

	// DistForGoals creates a rule to copy one or more Paths to the artifacts
	// directory on the build server when any of the specified goals are built.
	DistForGoals(goals []string, paths ...Path)

	// DistForGoalsWithFilename creates a rule to copy a Path to the artifacts
	// directory on the build server with the given filename when any of the
	// specified goals are built.
	DistForGoalsWithFilename(goals []string, path Path, filename string)
}

// MakeVarsContext contains the set of functions available for MakeVarsProvider
// and SingletonMakeVarsProvider implementations.
type MakeVarsContext interface {
	BaseMakeVarsContext

	ModuleName(module blueprint.Module) string
	ModuleDir(module blueprint.Module) string
	ModuleSubDir(module blueprint.Module) string
	ModuleType(module blueprint.Module) string
	moduleProvider(module blueprint.Module, key blueprint.AnyProviderKey) (any, bool)
	BlueprintFile(module blueprint.Module) string

	ModuleErrorf(module blueprint.Module, format string, args ...interface{})
	Errorf(format string, args ...interface{})

	VisitAllModules(visit func(Module))
	VisitAllModulesIf(pred func(Module) bool, visit func(Module))

	// Verify the make variable matches the Soong version, fail the build
	// if it does not. If the make variable is empty, just set it.
	Strict(name, ninjaStr string)
	// Check to see if the make variable matches the Soong version, warn if
	// it does not. If the make variable is empty, just set it.
	Check(name, ninjaStr string)

	// These are equivalent to the above, but sort the make and soong
	// variables before comparing them. They also show the unique entries
	// in each list when displaying the difference, instead of the entire
	// string.
	StrictSorted(name, ninjaStr string)
	CheckSorted(name, ninjaStr string)

	// Evaluates a ninja string and returns the result. Used if more
	// complicated modification needs to happen before giving it to Make.
	Eval(ninjaStr string) (string, error)
}

// MakeVarsModuleContext contains the set of functions available for modules
// implementing the ModuleMakeVarsProvider interface.
type MakeVarsModuleContext interface {
	BaseMakeVarsContext
}

var _ PathContext = MakeVarsContext(nil)

type MakeVarsProvider func(ctx MakeVarsContext)

func RegisterMakeVarsProvider(pctx PackageContext, provider MakeVarsProvider) {
	makeVarsInitProviders = append(makeVarsInitProviders, makeVarsProvider{pctx, provider})
}

// SingletonMakeVarsProvider is a Singleton with an extra method to provide extra values to be exported to Make.
type SingletonMakeVarsProvider interface {
	// MakeVars uses a MakeVarsContext to provide extra values to be exported to Make.
	MakeVars(ctx MakeVarsContext)
}

var singletonMakeVarsProvidersKey = NewOnceKey("singletonMakeVarsProvidersKey")

func getSingletonMakevarsProviders(config Config) *[]makeVarsProvider {
	return config.Once(singletonMakeVarsProvidersKey, func() interface{} {
		return &[]makeVarsProvider{}
	}).(*[]makeVarsProvider)
}

// registerSingletonMakeVarsProvider adds a singleton that implements SingletonMakeVarsProvider to
// the list of MakeVarsProviders to run.
func registerSingletonMakeVarsProvider(config Config, singleton SingletonMakeVarsProvider) {
	// Singletons are registered on the Context and may be different between different Contexts,
	// for example when running multiple tests.  Store the SingletonMakeVarsProviders in the
	// Config so they are attached to the Context.
	singletonMakeVarsProviders := getSingletonMakevarsProviders(config)

	*singletonMakeVarsProviders = append(*singletonMakeVarsProviders,
		makeVarsProvider{pctx, singletonMakeVarsProviderAdapter(singleton)})
}

// singletonMakeVarsProviderAdapter converts a SingletonMakeVarsProvider to a MakeVarsProvider.
func singletonMakeVarsProviderAdapter(singleton SingletonMakeVarsProvider) MakeVarsProvider {
	return func(ctx MakeVarsContext) { singleton.MakeVars(ctx) }
}

// ModuleMakeVarsProvider is a Module with an extra method to provide extra values to be exported to Make.
type ModuleMakeVarsProvider interface {
	Module

	// MakeVars uses a MakeVarsModuleContext to provide extra values to be exported to Make.
	MakeVars(ctx MakeVarsModuleContext)
}

// /////////////////////////////////////////////////////////////////////////////

func makeVarsSingletonFunc() Singleton {
	return &makeVarsSingleton{}
}

type makeVarsSingleton struct {
	varsForTesting     []makeVarsVariable
	installsForTesting []byte
}

type makeVarsProvider struct {
	pctx PackageContext
	call MakeVarsProvider
}

// Collection of makevars providers that are registered in init() methods.
var makeVarsInitProviders []makeVarsProvider

type makeVarsContext struct {
	SingletonContext
	config  Config
	pctx    PackageContext
	vars    []makeVarsVariable
	phonies []phony
	dists   []dist
}

var _ MakeVarsContext = &makeVarsContext{}

type makeVarsVariable struct {
	name   string
	value  string
	sort   bool
	strict bool
}

type phony struct {
	name string
	deps []string
}

type dist struct {
	goals []string
	paths []string
}

func (s *makeVarsSingleton) GenerateBuildActions(ctx SingletonContext) {
	if !ctx.Config().KatiEnabled() {
		return
	}

	outFile := absolutePath(PathForOutput(ctx,
		"make_vars"+proptools.String(ctx.Config().productVariables.Make_suffix)+".mk").String())

	lateOutFile := absolutePath(PathForOutput(ctx,
		"late"+proptools.String(ctx.Config().productVariables.Make_suffix)+".mk").String())

	installsFile := absolutePath(PathForOutput(ctx,
		"installs"+proptools.String(ctx.Config().productVariables.Make_suffix)+".mk").String())

	if ctx.Failed() {
		return
	}

	var vars []makeVarsVariable
	var dists []dist
	var phonies []phony
	var katiInstalls []katiInstall
	var katiInitRcInstalls []katiInstall
	var katiVintfManifestInstalls []katiInstall
	var katiSymlinks []katiInstall

	providers := append([]makeVarsProvider(nil), makeVarsInitProviders...)
	providers = append(providers, *getSingletonMakevarsProviders(ctx.Config())...)

	for _, provider := range providers {
		mctx := &makeVarsContext{
			SingletonContext: ctx,
			pctx:             provider.pctx,
		}

		provider.call(mctx)

		vars = append(vars, mctx.vars...)
		phonies = append(phonies, mctx.phonies...)
		dists = append(dists, mctx.dists...)
	}

	ctx.VisitAllModules(func(m Module) {
		if provider, ok := m.(ModuleMakeVarsProvider); ok && m.Enabled() {
			mctx := &makeVarsContext{
				SingletonContext: ctx,
			}

			provider.MakeVars(mctx)

			vars = append(vars, mctx.vars...)
			phonies = append(phonies, mctx.phonies...)
			dists = append(dists, mctx.dists...)
		}

		if m.ExportedToMake() {
			katiInstalls = append(katiInstalls, m.base().katiInstalls...)
			katiInitRcInstalls = append(katiInitRcInstalls, m.base().katiInitRcInstalls...)
			katiVintfManifestInstalls = append(katiVintfManifestInstalls, m.base().katiVintfInstalls...)
			katiSymlinks = append(katiSymlinks, m.base().katiSymlinks...)
		}
	})

	compareKatiInstalls := func(a, b katiInstall) int {
		aTo, bTo := a.to.String(), b.to.String()
		if cmpTo := cmp.Compare(aTo, bTo); cmpTo != 0 {
			return cmpTo
		}

		aFrom, bFrom := a.from.String(), b.from.String()
		return cmp.Compare(aFrom, bFrom)
	}

	slices.SortFunc(katiInitRcInstalls, compareKatiInstalls)
	katiInitRcInstalls = slices.CompactFunc(katiInitRcInstalls, func(a, b katiInstall) bool {
		return compareKatiInstalls(a, b) == 0
	})
	katiInstalls = append(katiInstalls, katiInitRcInstalls...)

	slices.SortFunc(katiVintfManifestInstalls, compareKatiInstalls)
	katiVintfManifestInstalls = slices.CompactFunc(katiVintfManifestInstalls, func(a, b katiInstall) bool {
		return compareKatiInstalls(a, b) == 0
	})

	if ctx.Failed() {
		return
	}

	sort.Slice(vars, func(i, j int) bool {
		return vars[i].name < vars[j].name
	})
	sort.Slice(phonies, func(i, j int) bool {
		return phonies[i].name < phonies[j].name
	})
	lessArr := func(a, b []string) bool {
		if len(a) == len(b) {
			for i := range a {
				if a[i] < b[i] {
					return true
				}
			}
			return false
		}
		return len(a) < len(b)
	}
	sort.Slice(dists, func(i, j int) bool {
		return lessArr(dists[i].goals, dists[j].goals) || lessArr(dists[i].paths, dists[j].paths)
	})

	outBytes := s.writeVars(vars)

	if err := pathtools.WriteFileIfChanged(outFile, outBytes, 0666); err != nil {
		ctx.Errorf(err.Error())
	}

	lateOutBytes := s.writeLate(phonies, dists)

	if err := pathtools.WriteFileIfChanged(lateOutFile, lateOutBytes, 0666); err != nil {
		ctx.Errorf(err.Error())
	}

	installsBytes := s.writeInstalls(katiInstalls, katiSymlinks, katiVintfManifestInstalls)
	if err := pathtools.WriteFileIfChanged(installsFile, installsBytes, 0666); err != nil {
		ctx.Errorf(err.Error())
	}

	// Only save state for tests when testing.
	if ctx.Config().RunningInsideUnitTest() {
		s.varsForTesting = vars
		s.installsForTesting = installsBytes
	}
}

func (s *makeVarsSingleton) writeVars(vars []makeVarsVariable) []byte {
	buf := &bytes.Buffer{}

	fmt.Fprint(buf, `# Autogenerated file

# Compares SOONG_$(1) against $(1), and warns if they are not equal.
#
# If the original variable is empty, then just set it to the SOONG_ version.
#
# $(1): Name of the variable to check
# $(2): If not-empty, sort the values before comparing
# $(3): Extra snippet to run if it does not match
define soong-compare-var
ifneq ($$($(1)),)
  my_val_make := $$(strip $(if $(2),$$(sort $$($(1))),$$($(1))))
  my_val_soong := $(if $(2),$$(sort $$(SOONG_$(1))),$$(SOONG_$(1)))
  ifneq ($$(my_val_make),$$(my_val_soong))
    $$(warning $(1) does not match between Make and Soong:)
    $(if $(2),$$(warning Make  adds: $$(filter-out $$(my_val_soong),$$(my_val_make))),$$(warning Make : $$(my_val_make)))
    $(if $(2),$$(warning Soong adds: $$(filter-out $$(my_val_make),$$(my_val_soong))),$$(warning Soong: $$(my_val_soong)))
    $(3)
  endif
  my_val_make :=
  my_val_soong :=
else
  $(1) := $$(SOONG_$(1))
endif
.KATI_READONLY := $(1) SOONG_$(1)
endef

my_check_failed := false

`)

	// Write all the strict checks out first so that if one of them errors,
	// we get all of the strict errors printed, but not the non-strict
	// warnings.
	for _, v := range vars {
		if !v.strict {
			continue
		}

		sort := ""
		if v.sort {
			sort = "true"
		}

		fmt.Fprintf(buf, "SOONG_%s := %s\n", v.name, v.value)
		fmt.Fprintf(buf, "$(eval $(call soong-compare-var,%s,%s,my_check_failed := true))\n\n", v.name, sort)
	}

	fmt.Fprint(buf, `
ifneq ($(my_check_failed),false)
  $(error Soong variable check failed)
endif
my_check_failed :=


`)

	for _, v := range vars {
		if v.strict {
			continue
		}

		sort := ""
		if v.sort {
			sort = "true"
		}

		fmt.Fprintf(buf, "SOONG_%s := %s\n", v.name, v.value)
		fmt.Fprintf(buf, "$(eval $(call soong-compare-var,%s,%s))\n\n", v.name, sort)
	}

	fmt.Fprintln(buf, "\nsoong-compare-var :=")

	fmt.Fprintln(buf)

	return buf.Bytes()
}

func (s *makeVarsSingleton) writeLate(phonies []phony, dists []dist) []byte {
	buf := &bytes.Buffer{}

	fmt.Fprint(buf, `# Autogenerated file

# Values written by Soong read after parsing all Android.mk files.


`)

	for _, phony := range phonies {
		fmt.Fprintf(buf, ".PHONY: %s\n", phony.name)
		fmt.Fprintf(buf, "%s: %s\n", phony.name, strings.Join(phony.deps, "\\\n  "))
	}

	fmt.Fprintln(buf)

	for _, dist := range dists {
		fmt.Fprintf(buf, ".PHONY: %s\n", strings.Join(dist.goals, " "))
		fmt.Fprintf(buf, "$(call dist-for-goals,%s,%s)\n",
			strings.Join(dist.goals, " "), strings.Join(dist.paths, " "))
	}

	return buf.Bytes()
}

// writeInstalls writes the list of install rules generated by Soong to a makefile.  The rules
// are exported to Make instead of written directly to the ninja file so that main.mk can add
// the dependencies from the `required` property that are hard to resolve in Soong.
func (s *makeVarsSingleton) writeInstalls(installs, symlinks, katiVintfManifestInstalls []katiInstall) []byte {
	buf := &bytes.Buffer{}

	fmt.Fprint(buf, `# Autogenerated file

# Values written by Soong to generate install rules that can be amended by Kati.


`)

	preserveSymlinksFlag := "-d"
	if runtime.GOOS == "darwin" {
		preserveSymlinksFlag = "-R"
	}

	for _, install := range installs {
		// Write a rule for each install request in the form:
		//  to: from [ deps ] [ | order only deps ]
		//       cp -f -d $< $@ [ && chmod +x $@ ]
		fmt.Fprintf(buf, "%s: %s", install.to.String(), install.from.String())
		for _, dep := range install.implicitDeps {
			fmt.Fprintf(buf, " %s", dep.String())
		}
		if extraFiles := install.extraFiles; extraFiles != nil {
			fmt.Fprintf(buf, " %s", extraFiles.zip.String())
		}
		if len(install.orderOnlyDeps) > 0 {
			fmt.Fprintf(buf, " |")
		}
		for _, dep := range install.orderOnlyDeps {
			fmt.Fprintf(buf, " %s", dep.String())
		}
		fmt.Fprintln(buf)
		fmt.Fprintln(buf, "\t@echo \"Install: $@\"")
		fmt.Fprintf(buf, "\trm -f $@ && cp -f %s $< $@\n", preserveSymlinksFlag)
		if install.executable {
			fmt.Fprintf(buf, "\tchmod +x $@\n")
		}
		if extraFiles := install.extraFiles; extraFiles != nil {
			fmt.Fprintf(buf, "\t( unzip -qDD -d '%s' '%s' 2>&1 | grep -v \"zipfile is empty\"; exit $${PIPESTATUS[0]} ) || \\\n", extraFiles.dir.String(), extraFiles.zip.String())
			fmt.Fprintf(buf, "\t  ( code=$$?; if [ $$code -ne 0 -a $$code -ne 1 ]; then exit $$code; fi )\n")
		}
		fmt.Fprintln(buf)
	}

	for _, symlink := range symlinks {
		fmt.Fprintf(buf, "%s:", symlink.to.String())
		if symlink.from != nil {
			// The katiVintfManifestInstall doesn't need updating when the target is modified, but we sometimes
			// have a dependency on a katiVintfManifestInstall to a binary instead of to the binary directly, and
			// the mtime of the katiVintfManifestInstall must be updated when the binary is modified, so use a
			// normal dependency here instead of an order-only dependency.
			fmt.Fprintf(buf, " %s", symlink.from.String())
		}
		for _, dep := range symlink.implicitDeps {
			fmt.Fprintf(buf, " %s", dep.String())
		}
		if len(symlink.orderOnlyDeps) > 0 {
			fmt.Fprintf(buf, " |")
		}
		for _, dep := range symlink.orderOnlyDeps {
			fmt.Fprintf(buf, " %s", dep.String())
		}
		fmt.Fprintln(buf)

		fromStr := ""
		if symlink.from != nil {
			rel, err := filepath.Rel(filepath.Dir(symlink.to.String()), symlink.from.String())
			if err != nil {
				panic(fmt.Errorf("failed to find relative path for katiVintfManifestInstall from %q to %q: %w",
					symlink.from.String(), symlink.to.String(), err))
			}
			fromStr = rel
		} else {
			fromStr = symlink.absFrom
		}

		fmt.Fprintln(buf, "\t@echo \"Symlink: $@\"")
		fmt.Fprintf(buf, "\trm -f $@ && ln -sfn %s $@", fromStr)
		fmt.Fprintln(buf)
		fmt.Fprintln(buf)
	}

	for _, install := range katiVintfManifestInstalls {
		// Write a rule for each vintf install request that calls the copy-vintf-manifest-chedk make function.
		fmt.Fprintf(buf, "$(eval $(call copy-vintf-manifest-checked, %s, %s))\n", install.from.String(), install.to.String())

		if len(install.implicitDeps) > 0 {
			panic(fmt.Errorf("unsupported implicitDeps %q in vintf install rule %q", install.implicitDeps, install.to))
		}
		if len(install.orderOnlyDeps) > 0 {
			panic(fmt.Errorf("unsupported orderOnlyDeps %q in vintf install rule %q", install.orderOnlyDeps, install.to))
		}

		fmt.Fprintln(buf)
	}
	return buf.Bytes()
}

func (c *makeVarsContext) DeviceConfig() DeviceConfig {
	return DeviceConfig{c.Config().deviceConfig}
}

var ninjaDescaper = strings.NewReplacer("$$", "$")

func (c *makeVarsContext) Eval(ninjaStr string) (string, error) {
	s, err := c.SingletonContext.Eval(c.pctx, ninjaStr)
	if err != nil {
		return "", err
	}
	// SingletonContext.Eval returns an exapnded string that is valid for a ninja file, de-escape $$ to $ for use
	// in a Makefile
	return ninjaDescaper.Replace(s), nil
}

func (c *makeVarsContext) addVariableRaw(name, value string, strict, sort bool) {
	c.vars = append(c.vars, makeVarsVariable{
		name:   name,
		value:  value,
		strict: strict,
		sort:   sort,
	})
}

func (c *makeVarsContext) addVariable(name, ninjaStr string, strict, sort bool) {
	value, err := c.Eval(ninjaStr)
	if err != nil {
		c.SingletonContext.Errorf(err.Error())
	}
	c.addVariableRaw(name, value, strict, sort)
}

func (c *makeVarsContext) addPhony(name string, deps []string) {
	c.phonies = append(c.phonies, phony{name, deps})
}

func (c *makeVarsContext) addDist(goals []string, paths []string) {
	c.dists = append(c.dists, dist{
		goals: goals,
		paths: paths,
	})
}

func (c *makeVarsContext) Strict(name, ninjaStr string) {
	c.addVariable(name, ninjaStr, true, false)
}
func (c *makeVarsContext) StrictSorted(name, ninjaStr string) {
	c.addVariable(name, ninjaStr, true, true)
}
func (c *makeVarsContext) StrictRaw(name, value string) {
	c.addVariableRaw(name, value, true, false)
}

func (c *makeVarsContext) Check(name, ninjaStr string) {
	c.addVariable(name, ninjaStr, false, false)
}
func (c *makeVarsContext) CheckSorted(name, ninjaStr string) {
	c.addVariable(name, ninjaStr, false, true)
}
func (c *makeVarsContext) CheckRaw(name, value string) {
	c.addVariableRaw(name, value, false, false)
}

func (c *makeVarsContext) Phony(name string, deps ...Path) {
	c.addPhony(name, Paths(deps).Strings())
}

func (c *makeVarsContext) DistForGoal(goal string, paths ...Path) {
	c.DistForGoals([]string{goal}, paths...)
}

func (c *makeVarsContext) DistForGoalWithFilename(goal string, path Path, filename string) {
	c.DistForGoalsWithFilename([]string{goal}, path, filename)
}

func (c *makeVarsContext) DistForGoals(goals []string, paths ...Path) {
	c.addDist(goals, Paths(paths).Strings())
}

func (c *makeVarsContext) DistForGoalsWithFilename(goals []string, path Path, filename string) {
	c.addDist(goals, []string{path.String() + ":" + filename})
}
