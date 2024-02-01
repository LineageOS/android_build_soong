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

package config

type toolchainBionic struct {
	toolchainBase
}

var (
	bionicDefaultSharedLibraries = []string{"libc", "libm", "libdl"}

	bionicCrtBeginStaticBinary, bionicCrtEndStaticBinary   = []string{"crtbegin_static"}, []string{"crtend_android"}
	bionicCrtBeginSharedBinary, bionicCrtEndSharedBinary   = []string{"crtbegin_dynamic"}, []string{"crtend_android"}
	bionicCrtBeginSharedLibrary, bionicCrtEndSharedLibrary = []string{"crtbegin_so"}, []string{"crtend_so"}
	bionicCrtPadSegmentSharedLibrary                       = []string{"crt_pad_segment"}
)

func (toolchainBionic) Bionic() bool { return true }

func (toolchainBionic) DefaultSharedLibraries() []string { return bionicDefaultSharedLibraries }

func (toolchainBionic) ShlibSuffix() string { return ".so" }

func (toolchainBionic) ExecutableSuffix() string { return "" }

func (toolchainBionic) AvailableLibraries() []string { return nil }

func (toolchainBionic) CrtBeginStaticBinary() []string       { return bionicCrtBeginStaticBinary }
func (toolchainBionic) CrtBeginSharedBinary() []string       { return bionicCrtBeginSharedBinary }
func (toolchainBionic) CrtBeginSharedLibrary() []string      { return bionicCrtBeginSharedLibrary }
func (toolchainBionic) CrtEndStaticBinary() []string         { return bionicCrtEndStaticBinary }
func (toolchainBionic) CrtEndSharedBinary() []string         { return bionicCrtEndSharedBinary }
func (toolchainBionic) CrtEndSharedLibrary() []string        { return bionicCrtEndSharedLibrary }
func (toolchainBionic) CrtPadSegmentSharedLibrary() []string { return bionicCrtPadSegmentSharedLibrary }
