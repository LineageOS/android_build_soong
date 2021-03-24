// Copyright 2021 Google Inc. All rights reserved.
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

package genrule

import (
	"strings"

	"android/soong/android"
)

// location is used to service $(location) and $(locations) entries in genrule commands.
type location interface {
	Paths(cmd *android.RuleBuilderCommand) []string
	String() string
}

// inputLocation is a $(location) result for an entry in the srcs property.
type inputLocation struct {
	paths android.Paths
}

func (l inputLocation) String() string {
	return strings.Join(l.paths.Strings(), " ")
}

func (l inputLocation) Paths(cmd *android.RuleBuilderCommand) []string {
	return cmd.PathsForInputs(l.paths)
}

var _ location = inputLocation{}

// outputLocation is a $(location) result for an entry in the out property.
type outputLocation struct {
	path android.WritablePath
}

func (l outputLocation) String() string {
	return l.path.String()
}

func (l outputLocation) Paths(cmd *android.RuleBuilderCommand) []string {
	return []string{cmd.PathForOutput(l.path)}
}

var _ location = outputLocation{}

// toolLocation is a $(location) result for an entry in the tools or tool_files property.
type toolLocation struct {
	paths android.Paths
}

func (l toolLocation) String() string {
	return strings.Join(l.paths.Strings(), " ")
}

func (l toolLocation) Paths(cmd *android.RuleBuilderCommand) []string {
	return cmd.PathsForTools(l.paths)
}

var _ location = toolLocation{}

// packagedToolLocation is a $(location) result for an entry in the tools or tool_files property
// that has PackagingSpecs.
type packagedToolLocation struct {
	spec android.PackagingSpec
}

func (l packagedToolLocation) String() string {
	return l.spec.FileName()
}

func (l packagedToolLocation) Paths(cmd *android.RuleBuilderCommand) []string {
	return []string{cmd.PathForPackagedTool(l.spec)}
}

var _ location = packagedToolLocation{}

// errorLocation is a placeholder for a $(location) result that returns garbage to break the command
// when error reporting is delayed by ALLOW_MISSING_DEPENDENCIES=true.
type errorLocation struct {
	err string
}

func (l errorLocation) String() string {
	return l.err
}

func (l errorLocation) Paths(cmd *android.RuleBuilderCommand) []string {
	return []string{l.err}
}

var _ location = errorLocation{}
