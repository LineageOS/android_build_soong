// Copyright 2022 Google Inc. All rights reserved.
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
	"path/filepath"
	"testing"

	"android/soong/bazel"
	"github.com/google/blueprint"
	"github.com/google/blueprint/pathtools"
)

type TestBazelPathContext struct{}

func (*TestBazelPathContext) Config() Config {
	cfg := NullConfig("out", "out/soong")
	cfg.BazelContext = MockBazelContext{
		OutputBaseDir: "out/bazel",
	}
	return cfg
}

func (*TestBazelPathContext) AddNinjaFileDeps(...string) {
	panic("Unimplemented")
}

func TestPathForBazelOut(t *testing.T) {
	ctx := &TestBazelPathContext{}
	out := PathForBazelOut(ctx, "foo/bar/baz/boq.txt")
	expectedPath := filepath.Join(ctx.Config().BazelContext.OutputBase(), "execroot/__main__/foo/bar/baz/boq.txt")
	if out.String() != expectedPath {
		t.Errorf("incorrect OutputPath: expected %q, got %q", expectedPath, out.String())
	}

	expectedRelPath := "foo/bar/baz/boq.txt"
	if out.Rel() != expectedRelPath {
		t.Errorf("incorrect OutputPath.Rel(): expected %q, got %q", expectedRelPath, out.Rel())
	}
}

func TestPathForBazelOutRelative(t *testing.T) {
	ctx := &TestBazelPathContext{}
	out := PathForBazelOutRelative(ctx, "foo/bar", "foo/bar/baz/boq.txt")

	expectedPath := filepath.Join(ctx.Config().BazelContext.OutputBase(), "execroot/__main__/foo/bar/baz/boq.txt")
	if out.String() != expectedPath {
		t.Errorf("incorrect OutputPath: expected %q, got %q", expectedPath, out.String())
	}

	expectedRelPath := "baz/boq.txt"
	if out.Rel() != expectedRelPath {
		t.Errorf("incorrect OutputPath.Rel(): expected %q, got %q", expectedRelPath, out.Rel())
	}
}

func TestPathForBazelOutRelativeUnderBinFolder(t *testing.T) {
	ctx := &TestBazelPathContext{}
	out := PathForBazelOutRelative(ctx, "foo/bar", "bazel-out/linux_x86_64-fastbuild-ST-b4ef1c4402f9/bin/foo/bar/baz/boq.txt")

	expectedPath := filepath.Join(ctx.Config().BazelContext.OutputBase(), "execroot/__main__/bazel-out/linux_x86_64-fastbuild-ST-b4ef1c4402f9/bin/foo/bar/baz/boq.txt")
	if out.String() != expectedPath {
		t.Errorf("incorrect OutputPath: expected %q, got %q", expectedPath, out.String())
	}

	expectedRelPath := "baz/boq.txt"
	if out.Rel() != expectedRelPath {
		t.Errorf("incorrect OutputPath.Rel(): expected %q, got %q", expectedRelPath, out.Rel())
	}
}

func TestPathForBazelOutOutsideOfExecroot(t *testing.T) {
	ctx := &TestBazelPathContext{}
	out := PathForBazelOut(ctx, "../bazel_tools/linux_x86_64-fastbuild/bin/tools/android/java_base_extras.jar")

	expectedPath := filepath.Join(ctx.Config().BazelContext.OutputBase(), "execroot/bazel_tools/linux_x86_64-fastbuild/bin/tools/android/java_base_extras.jar")
	if out.String() != expectedPath {
		t.Errorf("incorrect OutputPath: expected %q, got %q", expectedPath, out.String())
	}

	expectedRelPath := "execroot/bazel_tools/linux_x86_64-fastbuild/bin/tools/android/java_base_extras.jar"
	if out.Rel() != expectedRelPath {
		t.Errorf("incorrect OutputPath.Rel(): expected %q, got %q", expectedRelPath, out.Rel())
	}
}

func TestPathForBazelOutRelativeWithParentDirectoryRoot(t *testing.T) {
	ctx := &TestBazelPathContext{}
	out := PathForBazelOutRelative(ctx, "../bazel_tools", "../bazel_tools/foo/bar/baz.sh")

	expectedPath := filepath.Join(ctx.Config().BazelContext.OutputBase(), "execroot/bazel_tools/foo/bar/baz.sh")
	if out.String() != expectedPath {
		t.Errorf("incorrect OutputPath: expected %q, got %q", expectedPath, out.String())
	}

	expectedRelPath := "foo/bar/baz.sh"
	if out.Rel() != expectedRelPath {
		t.Errorf("incorrect OutputPath.Rel(): expected %q, got %q", expectedRelPath, out.Rel())
	}
}

type TestBazelConversionPathContext struct {
	TestBazelConversionContext
	moduleDir string
	cfg       Config
}

func (ctx *TestBazelConversionPathContext) AddNinjaFileDeps(...string) {
	panic("Unimplemented")
}

func (ctx *TestBazelConversionPathContext) GlobWithDeps(string, []string) ([]string, error) {
	panic("Unimplemented")
}

func (ctx *TestBazelConversionPathContext) PropertyErrorf(string, string, ...interface{}) {
	panic("Unimplemented")
}

func (ctx *TestBazelConversionPathContext) GetDirectDep(string) (blueprint.Module, blueprint.DependencyTag) {
	panic("Unimplemented")
}

func (ctx *TestBazelConversionPathContext) ModuleFromName(string) (blueprint.Module, bool) {
	panic("Unimplemented")
}

func (ctx *TestBazelConversionPathContext) AddUnconvertedBp2buildDep(string) {
	panic("Unimplemented")
}

func (ctx *TestBazelConversionPathContext) AddMissingBp2buildDep(string) {
	panic("Unimplemented")
}

func (ctx *TestBazelConversionPathContext) Module() Module {
	panic("Unimplemented")
}

func (ctx *TestBazelConversionPathContext) Config() Config {
	return ctx.cfg
}

func (ctx *TestBazelConversionPathContext) ModuleDir() string {
	return ctx.moduleDir
}

func TestTransformSubpackagePath(t *testing.T) {
	cfg := NullConfig("out", "out/soong")
	cfg.fs = pathtools.MockFs(map[string][]byte{
		"x/Android.bp":   nil,
		"x/y/Android.bp": nil,
	})

	var ctx BazelConversionPathContext = &TestBazelConversionPathContext{
		moduleDir: "x",
		cfg:       cfg,
	}
	pairs := map[string]string{
		"y/a.c":   "//x/y:a.c",
		"./y/a.c": "//x/y:a.c",
		"z/b.c":   "z/b.c",
		"./z/b.c": "z/b.c",
	}
	for in, out := range pairs {
		actual := transformSubpackagePath(ctx, bazel.Label{Label: in}).Label
		if actual != out {
			t.Errorf("expected:\n%v\nactual:\n%v", out, actual)
		}
	}
}
