/*
 * Copyright (C) 2023 The Android Open Source Project
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

package apex

import (
	"encoding/json"

	"github.com/google/blueprint"

	"android/soong/android"
)

var (
	mtctx = android.NewPackageContext("android/soong/multitree_apex")
)

func init() {
	RegisterModulesSingleton(android.InitRegistrationContext)
}

func RegisterModulesSingleton(ctx android.RegistrationContext) {
	ctx.RegisterSingletonType("apex_multitree_singleton", multitreeAnalysisSingletonFactory)
}

var PrepareForTestWithApexMultitreeSingleton = android.FixtureRegisterWithContext(RegisterModulesSingleton)

func multitreeAnalysisSingletonFactory() android.Singleton {
	return &multitreeAnalysisSingleton{}
}

type multitreeAnalysisSingleton struct {
	multitreeApexMetadataPath android.OutputPath
}

type ApexMultitreeMetadataEntry struct {
	// The name of the apex.
	Name string

	// TODO: Add other properties as needed.
}

type ApexMultitreeMetadata struct {
	// Information about the installable apexes.
	Apexes map[string]ApexMultitreeMetadataEntry
}

func (p *multitreeAnalysisSingleton) GenerateBuildActions(context android.SingletonContext) {
	data := ApexMultitreeMetadata{
		Apexes: make(map[string]ApexMultitreeMetadataEntry, 0),
	}
	context.VisitAllModules(func(module android.Module) {
		// If this module is not being installed, ignore it.
		if !module.Enabled() || module.IsSkipInstall() {
			return
		}
		// Actual apexes provide ApexBundleInfoProvider.
		if _, ok := context.ModuleProvider(module, ApexBundleInfoProvider).(ApexBundleInfo); !ok {
			return
		}
		bundle, ok := module.(*apexBundle)
		if ok && !bundle.testApex && !bundle.vndkApex && bundle.primaryApexType {
			name := module.Name()
			entry := ApexMultitreeMetadataEntry{
				Name: name,
			}
			data.Apexes[name] = entry
		}
	})
	p.multitreeApexMetadataPath = android.PathForOutput(context, "multitree_apex_metadata.json")

	jsonStr, err := json.Marshal(data)
	if err != nil {
		context.Errorf(err.Error())
	}
	android.WriteFileRule(context, p.multitreeApexMetadataPath, string(jsonStr))
	// This seems cleaner, but doesn't emit the phony rule in testing.
	// context.Phony("multitree_apex_metadata", p.multitreeApexMetadataPath)

	context.Build(mtctx, android.BuildParams{
		Rule:        blueprint.Phony,
		Description: "phony rule for multitree_apex_metadata",
		Inputs:      []android.Path{p.multitreeApexMetadataPath},
		Output:      android.PathForPhony(context, "multitree_apex_metadata"),
	})
}
