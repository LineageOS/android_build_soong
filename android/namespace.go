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
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/google/blueprint"
)

func init() {
	RegisterModuleType("soong_namespace", NamespaceFactory)
}

// threadsafe sorted list
type sortedNamespaces struct {
	lock   sync.Mutex
	items  []*Namespace
	sorted bool
}

func (s *sortedNamespaces) add(namespace *Namespace) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if s.sorted {
		panic("It is not supported to call sortedNamespaces.add() after sortedNamespaces.sortedItems()")
	}
	s.items = append(s.items, namespace)
}

func (s *sortedNamespaces) sortedItems() []*Namespace {
	s.lock.Lock()
	defer s.lock.Unlock()
	if !s.sorted {
		less := func(i int, j int) bool {
			return s.items[i].Path < s.items[j].Path
		}
		sort.Slice(s.items, less)
		s.sorted = true
	}
	return s.items
}

func (s *sortedNamespaces) index(namespace *Namespace) int {
	for i, candidate := range s.sortedItems() {
		if namespace == candidate {
			return i
		}
	}
	return -1
}

// A NameResolver implements blueprint.NameInterface, and implements the logic to
// find a module from namespaces based on a query string.
// A query string can be a module name or can be be "//namespace_path:module_path"
type NameResolver struct {
	rootNamespace *Namespace

	// id counter for atomic.AddInt32
	nextNamespaceId int32

	// All namespaces, without duplicates.
	sortedNamespaces sortedNamespaces

	// Map from dir to namespace. Will have duplicates if two dirs are part of the same namespace.
	namespacesByDir sync.Map // if generics were supported, this would be sync.Map[string]*Namespace

	// func telling whether to export a namespace to Kati
	namespaceExportFilter func(*Namespace) bool
}

func NewNameResolver(namespaceExportFilter func(*Namespace) bool) *NameResolver {
	r := &NameResolver{
		namespacesByDir:       sync.Map{},
		namespaceExportFilter: namespaceExportFilter,
	}
	r.rootNamespace = r.newNamespace(".")
	r.rootNamespace.visibleNamespaces = []*Namespace{r.rootNamespace}
	r.addNamespace(r.rootNamespace)

	return r
}

func (r *NameResolver) newNamespace(path string) *Namespace {
	namespace := NewNamespace(path)

	namespace.exportToKati = r.namespaceExportFilter(namespace)

	return namespace
}

func (r *NameResolver) addNewNamespaceForModule(module *NamespaceModule, path string) error {
	fileName := filepath.Base(path)
	if fileName != "Android.bp" {
		return errors.New("A namespace may only be declared in a file named Android.bp")
	}
	dir := filepath.Dir(path)

	namespace := r.newNamespace(dir)
	module.namespace = namespace
	module.resolver = r
	namespace.importedNamespaceNames = module.properties.Imports
	return r.addNamespace(namespace)
}

func (r *NameResolver) addNamespace(namespace *Namespace) (err error) {
	existingNamespace, exists := r.namespaceAt(namespace.Path)
	if exists {
		if existingNamespace.Path == namespace.Path {
			return fmt.Errorf("namespace %v already exists", namespace.Path)
		} else {
			// It would probably confuse readers if namespaces were declared anywhere but
			// the top of the file, so we forbid declaring namespaces after anything else.
			return fmt.Errorf("a namespace must be the first module in the file")
		}
	}
	if (namespace.exportToKati) {
		r.rootNamespace.visibleNamespaces = append(r.rootNamespace.visibleNamespaces, namespace)
	}
	r.sortedNamespaces.add(namespace)

	r.namespacesByDir.Store(namespace.Path, namespace)
	return nil
}

// non-recursive check for namespace
func (r *NameResolver) namespaceAt(path string) (namespace *Namespace, found bool) {
	mapVal, found := r.namespacesByDir.Load(path)
	if !found {
		return nil, false
	}
	return mapVal.(*Namespace), true
}

// recursive search upward for a namespace
func (r *NameResolver) findNamespace(path string) (namespace *Namespace) {
	namespace, found := r.namespaceAt(path)
	if found {
		return namespace
	}
	parentDir := filepath.Dir(path)
	if parentDir == path {
		return nil
	}
	namespace = r.findNamespace(parentDir)
	r.namespacesByDir.Store(path, namespace)
	return namespace
}

// A NamelessModule can never be looked up by name.  It must still implement Name(), but the return
// value doesn't have to be unique.
type NamelessModule interface {
	Nameless()
}

func (r *NameResolver) NewModule(ctx blueprint.NamespaceContext, moduleGroup blueprint.ModuleGroup, module blueprint.Module) (namespace blueprint.Namespace, errs []error) {
	// if this module is a namespace, then save it to our list of namespaces
	newNamespace, ok := module.(*NamespaceModule)
	if ok {
		err := r.addNewNamespaceForModule(newNamespace, ctx.ModulePath())
		if err != nil {
			return nil, []error{err}
		}
		return nil, nil
	}

	if _, ok := module.(NamelessModule); ok {
		return nil, nil
	}

	// if this module is not a namespace, then save it into the appropriate namespace
	ns := r.findNamespaceFromCtx(ctx)

	_, errs = ns.moduleContainer.NewModule(ctx, moduleGroup, module)
	if len(errs) > 0 {
		return nil, errs
	}

	amod, ok := module.(Module)
	if ok {
		// inform the module whether its namespace is one that we want to export to Make
		amod.base().commonProperties.NamespaceExportedToMake = ns.exportToKati
		amod.base().commonProperties.DebugName = module.Name()
	}

	return ns, nil
}

func (r *NameResolver) AllModules() []blueprint.ModuleGroup {
	childLists := [][]blueprint.ModuleGroup{}
	totalCount := 0
	for _, namespace := range r.sortedNamespaces.sortedItems() {
		newModules := namespace.moduleContainer.AllModules()
		totalCount += len(newModules)
		childLists = append(childLists, newModules)
	}

	allModules := make([]blueprint.ModuleGroup, 0, totalCount)
	for _, childList := range childLists {
		allModules = append(allModules, childList...)
	}
	return allModules
}

// parses a fully-qualified path (like "//namespace_path:module_name") into a namespace name and a
// module name
func (r *NameResolver) parseFullyQualifiedName(name string) (namespaceName string, moduleName string, ok bool) {
	if !strings.HasPrefix(name, "//") {
		return "", "", false
	}
	name = strings.TrimPrefix(name, "//")
	components := strings.Split(name, ":")
	if len(components) != 2 {
		return "", "", false
	}
	return components[0], components[1], true

}

func (r *NameResolver) getNamespacesToSearchForModule(sourceNamespace blueprint.Namespace) (searchOrder []*Namespace) {
	ns, ok := sourceNamespace.(*Namespace)
	if !ok || ns.visibleNamespaces == nil {
		// When handling dependencies before namespaceMutator, assume they are non-Soong Blueprint modules and give
		// access to all namespaces.
		return r.sortedNamespaces.sortedItems()
	}
	return ns.visibleNamespaces
}

func (r *NameResolver) ModuleFromName(name string, namespace blueprint.Namespace) (group blueprint.ModuleGroup, found bool) {
	// handle fully qualified references like "//namespace_path:module_name"
	nsName, moduleName, isAbs := r.parseFullyQualifiedName(name)
	if isAbs {
		namespace, found := r.namespaceAt(nsName)
		if !found {
			return blueprint.ModuleGroup{}, false
		}
		container := namespace.moduleContainer
		return container.ModuleFromName(moduleName, nil)
	}
	for _, candidate := range r.getNamespacesToSearchForModule(namespace) {
		group, found = candidate.moduleContainer.ModuleFromName(name, nil)
		if found {
			return group, true
		}
	}
	return blueprint.ModuleGroup{}, false

}

func (r *NameResolver) Rename(oldName string, newName string, namespace blueprint.Namespace) []error {
	return namespace.(*Namespace).moduleContainer.Rename(oldName, newName, namespace)
}

// resolve each element of namespace.importedNamespaceNames and put the result in namespace.visibleNamespaces
func (r *NameResolver) FindNamespaceImports(namespace *Namespace) (err error) {
	namespace.visibleNamespaces = make([]*Namespace, 0, 2+len(namespace.importedNamespaceNames))
	// search itself first
	namespace.visibleNamespaces = append(namespace.visibleNamespaces, namespace)
	// search its imports next
	for _, name := range namespace.importedNamespaceNames {
		imp, ok := r.namespaceAt(name)
		if !ok {
			return fmt.Errorf("namespace %v does not exist", name)
		}
		namespace.visibleNamespaces = append(namespace.visibleNamespaces, imp)
	}
	// search the root namespace last
	namespace.visibleNamespaces = append(namespace.visibleNamespaces, r.rootNamespace)
	return nil
}

func (r *NameResolver) chooseId(namespace *Namespace) {
	id := r.sortedNamespaces.index(namespace)
	if id < 0 {
		panic(fmt.Sprintf("Namespace not found: %v\n", namespace.id))
	}
	namespace.id = strconv.Itoa(id)
}

func (r *NameResolver) MissingDependencyError(depender string, dependerNamespace blueprint.Namespace, depName string) (err error) {
	text := fmt.Sprintf("%q depends on undefined module %q", depender, depName)

	_, _, isAbs := r.parseFullyQualifiedName(depName)
	if isAbs {
		// if the user gave a fully-qualified name, we don't need to look for other
		// modules that they might have been referring to
		return fmt.Errorf(text)
	}

	// determine which namespaces the module can be found in
	foundInNamespaces := []string{}
	for _, namespace := range r.sortedNamespaces.sortedItems() {
		_, found := namespace.moduleContainer.ModuleFromName(depName, nil)
		if found {
			foundInNamespaces = append(foundInNamespaces, namespace.Path)
		}
	}
	if len(foundInNamespaces) > 0 {
		// determine which namespaces are visible to dependerNamespace
		dependerNs := dependerNamespace.(*Namespace)
		searched := r.getNamespacesToSearchForModule(dependerNs)
		importedNames := []string{}
		for _, ns := range searched {
			importedNames = append(importedNames, ns.Path)
		}
		text += fmt.Sprintf("\nModule %q is defined in namespace %q which can read these %v namespaces: %q", depender, dependerNs.Path, len(importedNames), importedNames)
		text += fmt.Sprintf("\nModule %q can be found in these namespaces: %q", depName, foundInNamespaces)
	}

	return fmt.Errorf(text)
}

func (r *NameResolver) GetNamespace(ctx blueprint.NamespaceContext) blueprint.Namespace {
	return r.findNamespaceFromCtx(ctx)
}

func (r *NameResolver) findNamespaceFromCtx(ctx blueprint.NamespaceContext) *Namespace {
	return r.findNamespace(filepath.Dir(ctx.ModulePath()))
}

func (r *NameResolver) UniqueName(ctx blueprint.NamespaceContext, name string) (unique string) {
	prefix := r.findNamespaceFromCtx(ctx).id
	if prefix != "" {
		prefix = prefix + "-"
	}
	return prefix + name
}

var _ blueprint.NameInterface = (*NameResolver)(nil)

type Namespace struct {
	blueprint.NamespaceMarker
	Path string

	// names of namespaces listed as imports by this namespace
	importedNamespaceNames []string
	// all namespaces that should be searched when a module in this namespace declares a dependency
	visibleNamespaces []*Namespace

	id string

	exportToKati bool

	moduleContainer blueprint.NameInterface
}

func NewNamespace(path string) *Namespace {
	return &Namespace{Path: path, moduleContainer: blueprint.NewSimpleNameInterface()}
}

var _ blueprint.Namespace = (*Namespace)(nil)

type namespaceProperties struct {
	// a list of namespaces that contain modules that will be referenced
	// by modules in this namespace.
	Imports []string `android:"path"`
}

type NamespaceModule struct {
	ModuleBase

	namespace *Namespace
	resolver  *NameResolver

	properties namespaceProperties
}

func (n *NamespaceModule) GenerateAndroidBuildActions(ctx ModuleContext) {
}

func (n *NamespaceModule) GenerateBuildActions(ctx blueprint.ModuleContext) {
}

func (n *NamespaceModule) Name() (name string) {
	return *n.nameProperties.Name
}

// soong_namespace provides a scope to modules in an Android.bp file to prevent
// module name conflicts with other defined modules in different Android.bp
// files. Once soong_namespace has been defined in an Android.bp file, the
// namespacing is applied to all modules that follow the soong_namespace in
// the current Android.bp file, as well as modules defined in Android.bp files
// in subdirectories. An Android.bp file in a subdirectory can define its own
// soong_namespace which is applied to all its modules and as well as modules
// defined in subdirectories Android.bp files. Modules in a soong_namespace are
// visible to Make by listing the namespace path in PRODUCT_SOONG_NAMESPACES
// make variable in a makefile.
func NamespaceFactory() Module {
	module := &NamespaceModule{}

	name := "soong_namespace"
	module.nameProperties.Name = &name

	module.AddProperties(&module.properties)
	return module
}

func RegisterNamespaceMutator(ctx RegisterMutatorsContext) {
	ctx.BottomUp("namespace_deps", namespaceMutator).Parallel()
}

func namespaceMutator(ctx BottomUpMutatorContext) {
	module, ok := ctx.Module().(*NamespaceModule)
	if ok {
		err := module.resolver.FindNamespaceImports(module.namespace)
		if err != nil {
			ctx.ModuleErrorf(err.Error())
		}

		module.resolver.chooseId(module.namespace)
	}
}
