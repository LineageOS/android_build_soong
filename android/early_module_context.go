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
	"github.com/google/blueprint"
	"os"
	"text/scanner"
)

// EarlyModuleContext provides methods that can be called early, as soon as the properties have
// been parsed into the module and before any mutators have run.
type EarlyModuleContext interface {
	// Module returns the current module as a Module.  It should rarely be necessary, as the module already has a
	// reference to itself.
	Module() Module

	// ModuleName returns the name of the module.  This is generally the value that was returned by Module.Name() when
	// the module was created, but may have been modified by calls to BaseMutatorContext.Rename.
	ModuleName() string

	// ModuleDir returns the path to the directory that contains the definition of the module.
	ModuleDir() string

	// ModuleType returns the name of the module type that was used to create the module, as specified in
	// RegisterModuleType.
	ModuleType() string

	// BlueprintFile returns the name of the blueprint file that contains the definition of this
	// module.
	BlueprintsFile() string

	// ContainsProperty returns true if the specified property name was set in the module definition.
	ContainsProperty(name string) bool

	// Errorf reports an error at the specified position of the module definition file.
	Errorf(pos scanner.Position, fmt string, args ...interface{})

	// ModuleErrorf reports an error at the line number of the module type in the module definition.
	ModuleErrorf(fmt string, args ...interface{})

	// PropertyErrorf reports an error at the line number of a property in the module definition.
	PropertyErrorf(property, fmt string, args ...interface{})

	// Failed returns true if any errors have been reported.  In most cases the module can continue with generating
	// build rules after an error, allowing it to report additional errors in a single run, but in cases where the error
	// has prevented the module from creating necessary data it can return early when Failed returns true.
	Failed() bool

	// AddNinjaFileDeps adds dependencies on the specified files to the rule that creates the ninja manifest.  The
	// primary builder will be rerun whenever the specified files are modified.
	AddNinjaFileDeps(deps ...string)

	DeviceSpecific() bool
	SocSpecific() bool
	ProductSpecific() bool
	SystemExtSpecific() bool
	Platform() bool

	Config() Config
	DeviceConfig() DeviceConfig

	// Deprecated: use Config()
	AConfig() Config

	// GlobWithDeps returns a list of files that match the specified pattern but do not match any
	// of the patterns in excludes.  It also adds efficient dependencies to rerun the primary
	// builder whenever a file matching the pattern as added or removed, without rerunning if a
	// file that does not match the pattern is added to a searched directory.
	GlobWithDeps(pattern string, excludes []string) ([]string, error)

	Glob(globPattern string, excludes []string) Paths
	GlobFiles(globPattern string, excludes []string) Paths
	IsSymlink(path Path) bool
	Readlink(path Path) string

	// Namespace returns the Namespace object provided by the NameInterface set by Context.SetNameInterface, or the
	// default SimpleNameInterface if Context.SetNameInterface was not called.
	Namespace() *Namespace
}

// Deprecated: use EarlyModuleContext instead
type BaseContext interface {
	EarlyModuleContext
}

type earlyModuleContext struct {
	blueprint.EarlyModuleContext

	kind   moduleKind
	config Config
}

func (e *earlyModuleContext) Glob(globPattern string, excludes []string) Paths {
	return Glob(e, globPattern, excludes)
}

func (e *earlyModuleContext) GlobFiles(globPattern string, excludes []string) Paths {
	return GlobFiles(e, globPattern, excludes)
}

func (e *earlyModuleContext) IsSymlink(path Path) bool {
	fileInfo, err := e.config.fs.Lstat(path.String())
	if err != nil {
		e.ModuleErrorf("os.Lstat(%q) failed: %s", path.String(), err)
	}
	return fileInfo.Mode()&os.ModeSymlink == os.ModeSymlink
}

func (e *earlyModuleContext) Readlink(path Path) string {
	dest, err := e.config.fs.Readlink(path.String())
	if err != nil {
		e.ModuleErrorf("os.Readlink(%q) failed: %s", path.String(), err)
	}
	return dest
}

func (e *earlyModuleContext) Module() Module {
	module, _ := e.EarlyModuleContext.Module().(Module)
	return module
}

func (e *earlyModuleContext) Config() Config {
	return e.EarlyModuleContext.Config().(Config)
}

func (e *earlyModuleContext) AConfig() Config {
	return e.config
}

func (e *earlyModuleContext) DeviceConfig() DeviceConfig {
	return DeviceConfig{e.config.deviceConfig}
}

func (e *earlyModuleContext) Platform() bool {
	return e.kind == platformModule
}

func (e *earlyModuleContext) DeviceSpecific() bool {
	return e.kind == deviceSpecificModule
}

func (e *earlyModuleContext) SocSpecific() bool {
	return e.kind == socSpecificModule
}

func (e *earlyModuleContext) ProductSpecific() bool {
	return e.kind == productSpecificModule
}

func (e *earlyModuleContext) SystemExtSpecific() bool {
	return e.kind == systemExtSpecificModule
}

func (e *earlyModuleContext) Namespace() *Namespace {
	return e.EarlyModuleContext.Namespace().(*Namespace)
}
