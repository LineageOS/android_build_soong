/*
 * Copyright (C) 2021 The Android Open Source Project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package java

import (
	"android/soong/android"
)

// Build rules and utilities to generate individual packages/modules/SdkExtensions/proto/classpaths.proto
// config files based on build configuration to embed into /system and /apex on a device.
//
// See `derive_classpath` service that reads the configs at runtime and defines *CLASSPATH variables
// on the device.

type classpathType int

const (
	// Matches definition in packages/modules/SdkExtensions/proto/classpaths.proto
	BOOTCLASSPATH classpathType = iota
	DEX2OATBOOTCLASSPATH
	SYSTEMSERVERCLASSPATH
)

func (c classpathType) String() string {
	return [...]string{"BOOTCLASSPATH", "DEX2OATBOOTCLASSPATH", "SYSTEMSERVERCLASSPATH"}[c]
}

type classpathFragmentProperties struct {
}

// classpathFragment interface is implemented by a module that contributes jars to a *CLASSPATH
// variables at runtime.
type classpathFragment interface {
	android.Module

	classpathFragmentBase() *classpathFragmentBase
}

// classpathFragmentBase is meant to be embedded in any module types that implement classpathFragment;
// such modules are expected to call initClasspathFragment().
type classpathFragmentBase struct {
	properties classpathFragmentProperties

	classpathType classpathType

	outputFilepath android.OutputPath
}

// Initializes classpathFragmentBase struct. Must be called by all modules that include classpathFragmentBase.
func initClasspathFragment(c classpathFragment) {
	base := c.classpathFragmentBase()
	c.AddProperties(&base.properties)
}
