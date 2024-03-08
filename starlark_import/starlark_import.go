// Copyright 2023 Google Inc. All rights reserved.
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

package starlark_import

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkjson"
	"go.starlark.net/starlarkstruct"
)

func init() {
	go func() {
		startTime := time.Now()
		v, d, err := runStarlarkFile("//build/bazel/constants_exported_to_soong.bzl")
		endTime := time.Now()
		//fmt.Fprintf(os.Stderr, "starlark run time: %s\n", endTime.Sub(startTime).String())
		globalResult.Set(starlarkResult{
			values:    v,
			ninjaDeps: d,
			err:       err,
			startTime: startTime,
			endTime:   endTime,
		})
	}()
}

type starlarkResult struct {
	values    starlark.StringDict
	ninjaDeps []string
	err       error
	startTime time.Time
	endTime   time.Time
}

// setOnce wraps a value and exposes Set() and Get() accessors for it.
// The Get() calls will block until a Set() has been called.
// A second call to Set() will panic.
// setOnce must be created using newSetOnce()
type setOnce[T any] struct {
	value T
	lock  sync.Mutex
	wg    sync.WaitGroup
	isSet bool
}

func (o *setOnce[T]) Set(value T) {
	o.lock.Lock()
	defer o.lock.Unlock()
	if o.isSet {
		panic("Value already set")
	}

	o.value = value
	o.isSet = true
	o.wg.Done()
}

func (o *setOnce[T]) Get() T {
	if !o.isSet {
		o.wg.Wait()
	}
	return o.value
}

func newSetOnce[T any]() *setOnce[T] {
	result := &setOnce[T]{}
	result.wg.Add(1)
	return result
}

var globalResult = newSetOnce[starlarkResult]()

func GetStarlarkValue[T any](key string) (T, error) {
	result := globalResult.Get()
	if result.err != nil {
		var zero T
		return zero, result.err
	}
	if !result.values.Has(key) {
		var zero T
		return zero, fmt.Errorf("a starlark variable by that name wasn't found, did you update //build/bazel/constants_exported_to_soong.bzl?")
	}
	return Unmarshal[T](result.values[key])
}

func GetNinjaDeps() ([]string, error) {
	result := globalResult.Get()
	if result.err != nil {
		return nil, result.err
	}
	return result.ninjaDeps, nil
}

func getTopDir() (string, error) {
	// It's hard to communicate the top dir to this package in any other way than reading the
	// arguments directly, because we need to know this at package initialization time. Many
	// soong constants that we'd like to read from starlark are initialized during package
	// initialization.
	for i, arg := range os.Args {
		if arg == "--top" {
			if i < len(os.Args)-1 && os.Args[i+1] != "" {
				return os.Args[i+1], nil
			}
		}
	}

	// When running tests, --top is not passed. Instead, search for the top dir manually
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for cwd != "/" {
		if _, err := os.Stat(filepath.Join(cwd, "build/soong/soong_ui.bash")); err == nil {
			return cwd, nil
		}
		cwd = filepath.Dir(cwd)
	}
	return "", fmt.Errorf("could not find top dir")
}

const callerDirKey = "callerDir"

type modentry struct {
	globals starlark.StringDict
	err     error
}

func unsupportedMethod(t *starlark.Thread, fn *starlark.Builtin, _ starlark.Tuple, _ []starlark.Tuple) (starlark.Value, error) {
	return nil, fmt.Errorf("%sthis file is read by soong, and must therefore be pure starlark and include only constant information. %q is not allowed", t.CallStack().String(), fn.Name())
}

var builtins = starlark.StringDict{
	"aspect":     starlark.NewBuiltin("aspect", unsupportedMethod),
	"glob":       starlark.NewBuiltin("glob", unsupportedMethod),
	"json":       starlarkjson.Module,
	"provider":   starlark.NewBuiltin("provider", unsupportedMethod),
	"rule":       starlark.NewBuiltin("rule", unsupportedMethod),
	"struct":     starlark.NewBuiltin("struct", starlarkstruct.Make),
	"select":     starlark.NewBuiltin("select", unsupportedMethod),
	"transition": starlark.NewBuiltin("transition", unsupportedMethod),
}

// Takes a module name (the first argument to the load() function) and returns the path
// it's trying to load, stripping out leading //, and handling leading :s.
func cleanModuleName(moduleName string, callerDir string) (string, error) {
	if strings.Count(moduleName, ":") > 1 {
		return "", fmt.Errorf("at most 1 colon must be present in starlark path: %s", moduleName)
	}

	// We don't have full support for external repositories, but at least support skylib's dicts.
	if moduleName == "@bazel_skylib//lib:dicts.bzl" {
		return "external/bazel-skylib/lib/dicts.bzl", nil
	}

	localLoad := false
	if strings.HasPrefix(moduleName, "@//") {
		moduleName = moduleName[3:]
	} else if strings.HasPrefix(moduleName, "//") {
		moduleName = moduleName[2:]
	} else if strings.HasPrefix(moduleName, ":") {
		moduleName = moduleName[1:]
		localLoad = true
	} else {
		return "", fmt.Errorf("load path must start with // or :")
	}

	if ix := strings.LastIndex(moduleName, ":"); ix >= 0 {
		moduleName = moduleName[:ix] + string(os.PathSeparator) + moduleName[ix+1:]
	}

	if filepath.Clean(moduleName) != moduleName {
		return "", fmt.Errorf("load path must be clean, found: %s, expected: %s", moduleName, filepath.Clean(moduleName))
	}
	if strings.HasPrefix(moduleName, "../") {
		return "", fmt.Errorf("load path must not start with ../: %s", moduleName)
	}
	if strings.HasPrefix(moduleName, "/") {
		return "", fmt.Errorf("load path starts with /, use // for a absolute path: %s", moduleName)
	}

	if localLoad {
		return filepath.Join(callerDir, moduleName), nil
	}

	return moduleName, nil
}

// loader implements load statement. The format of the loaded module URI is
//
//	[//path]:base
//
// The file path is $ROOT/path/base if path is present, <caller_dir>/base otherwise.
func loader(thread *starlark.Thread, module string, topDir string, moduleCache map[string]*modentry, moduleCacheLock *sync.Mutex, filesystem map[string]string) (starlark.StringDict, error) {
	modulePath, err := cleanModuleName(module, thread.Local(callerDirKey).(string))
	if err != nil {
		return nil, err
	}
	moduleCacheLock.Lock()
	e, ok := moduleCache[modulePath]
	if e == nil {
		if ok {
			moduleCacheLock.Unlock()
			return nil, fmt.Errorf("cycle in load graph")
		}

		// Add a placeholder to indicate "load in progress".
		moduleCache[modulePath] = nil
		moduleCacheLock.Unlock()

		childThread := &starlark.Thread{Name: "exec " + module, Load: thread.Load}

		// Cheating for the sake of testing:
		// propagate starlarktest's Reporter key, otherwise testing
		// the load function may cause panic in starlarktest code.
		const testReporterKey = "Reporter"
		if v := thread.Local(testReporterKey); v != nil {
			childThread.SetLocal(testReporterKey, v)
		}

		childThread.SetLocal(callerDirKey, filepath.Dir(modulePath))

		if filesystem != nil {
			globals, err := starlark.ExecFile(childThread, filepath.Join(topDir, modulePath), filesystem[modulePath], builtins)
			e = &modentry{globals, err}
		} else {
			globals, err := starlark.ExecFile(childThread, filepath.Join(topDir, modulePath), nil, builtins)
			e = &modentry{globals, err}
		}

		// Update the cache.
		moduleCacheLock.Lock()
		moduleCache[modulePath] = e
	}
	moduleCacheLock.Unlock()
	return e.globals, e.err
}

// Run runs the given starlark file and returns its global variables and a list of all starlark
// files that were loaded. The top dir for starlark's // is found via getTopDir().
func runStarlarkFile(filename string) (starlark.StringDict, []string, error) {
	topDir, err := getTopDir()
	if err != nil {
		return nil, nil, err
	}
	return runStarlarkFileWithFilesystem(filename, topDir, nil)
}

func runStarlarkFileWithFilesystem(filename string, topDir string, filesystem map[string]string) (starlark.StringDict, []string, error) {
	if !strings.HasPrefix(filename, "//") && !strings.HasPrefix(filename, ":") {
		filename = "//" + filename
	}
	filename, err := cleanModuleName(filename, "")
	if err != nil {
		return nil, nil, err
	}
	moduleCache := make(map[string]*modentry)
	moduleCache[filename] = nil
	moduleCacheLock := &sync.Mutex{}
	mainThread := &starlark.Thread{
		Name: "main",
		Print: func(_ *starlark.Thread, msg string) {
			// Ignore prints
		},
		Load: func(thread *starlark.Thread, module string) (starlark.StringDict, error) {
			return loader(thread, module, topDir, moduleCache, moduleCacheLock, filesystem)
		},
	}
	mainThread.SetLocal(callerDirKey, filepath.Dir(filename))

	var result starlark.StringDict
	if filesystem != nil {
		result, err = starlark.ExecFile(mainThread, filepath.Join(topDir, filename), filesystem[filename], builtins)
	} else {
		result, err = starlark.ExecFile(mainThread, filepath.Join(topDir, filename), nil, builtins)
	}
	return result, sortedStringKeys(moduleCache), err
}

func sortedStringKeys(m map[string]*modentry) []string {
	s := make([]string, 0, len(m))
	for k := range m {
		s = append(s, k)
	}
	sort.Strings(s)
	return s
}
