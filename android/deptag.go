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

package android

import "github.com/google/blueprint"

// Dependency tags can implement this interface and return true from InstallDepNeeded to annotate
// that the installed files of the parent should depend on the installed files of the child.
type InstallNeededDependencyTag interface {
	// If InstallDepNeeded returns true then the installed files of the parent will depend on the
	// installed files of the child.
	InstallDepNeeded() bool
}

// Dependency tags can embed this struct to annotate that the installed files of the parent should
// depend on the installed files of the child.
type InstallAlwaysNeededDependencyTag struct{}

func (i InstallAlwaysNeededDependencyTag) InstallDepNeeded() bool {
	return true
}

var _ InstallNeededDependencyTag = InstallAlwaysNeededDependencyTag{}

// IsInstallDepNeededTag returns true if the dependency tag implements the InstallNeededDependencyTag
// interface and the InstallDepNeeded returns true, meaning that the installed files of the parent
// should depend on the installed files of the child.
func IsInstallDepNeededTag(tag blueprint.DependencyTag) bool {
	if i, ok := tag.(InstallNeededDependencyTag); ok {
		return i.InstallDepNeeded()
	}
	return false
}
