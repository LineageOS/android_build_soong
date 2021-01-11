// Copyright 2020 Google Inc. All rights reserved.
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

package main

import (
	"android/soong/android"
	"android/soong/bp2build"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/google/blueprint"
)

type queryviewContext struct {
	bpCtx *blueprint.Context
}

func (ctx *queryviewContext) ModuleName(module blueprint.Module) string {
	return ctx.bpCtx.ModuleName(module)
}

func (ctx *queryviewContext) ModuleDir(module blueprint.Module) string {
	return ctx.bpCtx.ModuleDir(module)
}

func (ctx *queryviewContext) ModuleSubDir(module blueprint.Module) string {
	return ctx.bpCtx.ModuleSubDir(module)
}

func (ctx *queryviewContext) ModuleType(module blueprint.Module) string {
	return ctx.bpCtx.ModuleType(module)
}

func (ctx *queryviewContext) VisitAllModulesBlueprint(visit func(blueprint.Module)) {
	ctx.bpCtx.VisitAllModules(visit)
}

func (ctx *queryviewContext) VisitDirectDeps(module android.Module, visit func(android.Module)) {
	ctx.bpCtx.VisitDirectDeps(module, func(m blueprint.Module) {
		if aModule, ok := m.(android.Module); ok {
			visit(aModule)
		}
	})
}

func createBazelQueryView(ctx *android.Context, bazelQueryViewDir string) error {
	qvCtx := queryviewContext{
		bpCtx: ctx.Context,
	}
	ruleShims := bp2build.CreateRuleShims(android.ModuleTypeFactories())
	buildToTargets := bp2build.GenerateSoongModuleTargets(&qvCtx)

	filesToWrite := bp2build.CreateBazelFiles(ruleShims, buildToTargets)
	for _, f := range filesToWrite {
		if err := writeReadOnlyFile(bazelQueryViewDir, f); err != nil {
			return err
		}
	}

	return nil
}

// The auto-conversion directory should be read-only, sufficient for bazel query. The files
// are not intended to be edited by end users.
func writeReadOnlyFile(dir string, f bp2build.BazelFile) error {
	dir = filepath.Join(dir, f.Dir)
	if err := createDirectoryIfNonexistent(dir); err != nil {
		return err
	}
	pathToFile := filepath.Join(dir, f.Basename)

	// 0444 is read-only
	err := ioutil.WriteFile(pathToFile, []byte(f.Contents), 0444)

	return err
}

func createDirectoryIfNonexistent(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, os.ModePerm)
	} else {
		return err
	}
}
