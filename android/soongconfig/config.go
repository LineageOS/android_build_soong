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

package soongconfig

import "strings"

type SoongConfig interface {
	// Bool interprets the variable named `name` as a boolean, returning true if, after
	// lowercasing, it matches one of "1", "y", "yes", "on", or "true". Unset, or any other
	// value will return false.
	Bool(name string) bool

	// String returns the string value of `name`. If the variable was not set, it will
	// return the empty string.
	String(name string) string

	// IsSet returns whether the variable `name` was set by Make.
	IsSet(name string) bool
}

func Config(vars map[string]string) SoongConfig {
	return soongConfig(vars)
}

type soongConfig map[string]string

func (c soongConfig) Bool(name string) bool {
	v := strings.ToLower(c[name])
	return v == "1" || v == "y" || v == "yes" || v == "on" || v == "true"
}

func (c soongConfig) String(name string) string {
	return c[name]
}

func (c soongConfig) IsSet(name string) bool {
	_, ok := c[name]
	return ok
}
