// Copyright 2018 Google Inc. All rights reserved.
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

package terminal

import (
	"io"

	"android/soong/ui/status"
)

// NewStatusOutput returns a StatusOutput that represents the
// current build status similarly to Ninja's built-in terminal
// output.
//
// statusFormat takes nearly all the same options as NINJA_STATUS.
// %c is currently unsupported.
func NewStatusOutput(w io.Writer, statusFormat string, forceSimpleOutput, quietBuild, forceKeepANSI bool) status.StatusOutput {
	useSmartStatus := !forceSimpleOutput && isSmartTerminal(w)
	formatter := newFormatter(statusFormat, quietBuild, useSmartStatus)

	if useSmartStatus {
		return NewSmartStatusOutput(w, formatter)
	} else {
		return NewSimpleStatusOutput(w, formatter, forceKeepANSI)
	}
}
