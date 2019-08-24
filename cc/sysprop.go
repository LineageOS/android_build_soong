// Copyright (C) 2019 The Android Open Source Project
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

package cc

import (
	"sync"

	"android/soong/android"
)

type syspropLibraryInterface interface {
	BaseModuleName() string
	CcModuleName() string
}

var (
	syspropImplLibrariesKey  = android.NewOnceKey("syspropImplLibirares")
	syspropImplLibrariesLock sync.Mutex
)

func syspropImplLibraries(config android.Config) map[string]string {
	return config.Once(syspropImplLibrariesKey, func() interface{} {
		return make(map[string]string)
	}).(map[string]string)
}

// gather list of sysprop libraries
func SyspropMutator(mctx android.BottomUpMutatorContext) {
	if m, ok := mctx.Module().(syspropLibraryInterface); ok {
		syspropImplLibraries := syspropImplLibraries(mctx.Config())
		syspropImplLibrariesLock.Lock()
		defer syspropImplLibrariesLock.Unlock()

		syspropImplLibraries[m.BaseModuleName()] = m.CcModuleName()
	}
}
