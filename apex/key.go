// Copyright (C) 2018 The Android Open Source Project
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

package apex

import (
	"fmt"
	"sort"
	"strings"

	"android/soong/android"

	"github.com/google/blueprint/proptools"
)

var String = proptools.String

func init() {
	android.RegisterModuleType("apex_key", ApexKeyFactory)
	android.RegisterSingletonType("apex_keys_text", apexKeysTextFactory)
}

type apexKey struct {
	android.ModuleBase

	properties apexKeyProperties

	public_key_file  android.Path
	private_key_file android.Path

	keyName string
}

type apexKeyProperties struct {
	// Path or module to the public key file in avbpubkey format. Installed to the device.
	// Base name of the file is used as the ID for the key.
	Public_key *string `android:"path"`
	// Path or module to the private key file in pem format. Used to sign APEXs.
	Private_key *string `android:"path"`

	// Whether this key is installable to one of the partitions. Defualt: true.
	Installable *bool
}

func ApexKeyFactory() android.Module {
	module := &apexKey{}
	module.AddProperties(&module.properties)
	android.InitAndroidArchModule(module, android.HostAndDeviceDefault, android.MultilibCommon)
	return module
}

func (m *apexKey) installable() bool {
	return false
}

func (m *apexKey) GenerateAndroidBuildActions(ctx android.ModuleContext) {
	// If the keys are from other modules (i.e. :module syntax) respect it.
	// Otherwise, try to locate the key files in the default cert dir or
	// in the local module dir
	if android.SrcIsModule(String(m.properties.Public_key)) != "" {
		m.public_key_file = android.PathForModuleSrc(ctx, String(m.properties.Public_key))
	} else {
		m.public_key_file = ctx.Config().ApexKeyDir(ctx).Join(ctx, String(m.properties.Public_key))
		// If not found, fall back to the local key pairs
		if !android.ExistentPathForSource(ctx, m.public_key_file.String()).Valid() {
			m.public_key_file = android.PathForModuleSrc(ctx, String(m.properties.Public_key))
		}
	}

	if android.SrcIsModule(String(m.properties.Private_key)) != "" {
		m.private_key_file = android.PathForModuleSrc(ctx, String(m.properties.Private_key))
	} else {
		m.private_key_file = ctx.Config().ApexKeyDir(ctx).Join(ctx, String(m.properties.Private_key))
		if !android.ExistentPathForSource(ctx, m.private_key_file.String()).Valid() {
			m.private_key_file = android.PathForModuleSrc(ctx, String(m.properties.Private_key))
		}
	}

	pubKeyName := m.public_key_file.Base()[0 : len(m.public_key_file.Base())-len(m.public_key_file.Ext())]
	privKeyName := m.private_key_file.Base()[0 : len(m.private_key_file.Base())-len(m.private_key_file.Ext())]

	if m.properties.Public_key != nil && m.properties.Private_key != nil && pubKeyName != privKeyName {
		ctx.ModuleErrorf("public_key %q (keyname:%q) and private_key %q (keyname:%q) do not have same keyname",
			m.public_key_file.String(), pubKeyName, m.private_key_file, privKeyName)
		return
	}
	m.keyName = pubKeyName
}

////////////////////////////////////////////////////////////////////////
// apex_keys_text
type apexKeysText struct {
	output android.OutputPath
}

func (s *apexKeysText) GenerateBuildActions(ctx android.SingletonContext) {
	s.output = android.PathForOutput(ctx, "apexkeys.txt")
	apexModulesMap := make(map[string]android.Module)
	ctx.VisitAllModules(func(module android.Module) {
		if m, ok := module.(*apexBundle); ok && m.Enabled() && m.installable() {
			apexModulesMap[m.Name()] = m
		}
	})

	// Find prebuilts and let them override apexBundle if they are preferred
	ctx.VisitAllModules(func(module android.Module) {
		if m, ok := module.(*Prebuilt); ok && m.Enabled() && m.installable() &&
			m.Prebuilt().UsePrebuilt() {
			apexModulesMap[m.BaseModuleName()] = m
		}
	})

	// iterating over map does not give consistent ordering in golang
	var moduleNames []string
	for key, _ := range apexModulesMap {
		moduleNames = append(moduleNames, key)
	}
	sort.Strings(moduleNames)

	var filecontent strings.Builder
	for _, key := range moduleNames {
		module := apexModulesMap[key]
		if m, ok := module.(*apexBundle); ok {
			fmt.Fprintf(&filecontent,
				"name=%q public_key=%q private_key=%q container_certificate=%q container_private_key=%q partition=%q\\n",
				m.Name()+".apex",
				m.public_key_file.String(),
				m.private_key_file.String(),
				m.container_certificate_file.String(),
				m.container_private_key_file.String(),
				m.PartitionTag(ctx.DeviceConfig()))
		} else if m, ok := module.(*Prebuilt); ok {
			fmt.Fprintf(&filecontent,
				"name=%q public_key=%q private_key=%q container_certificate=%q container_private_key=%q partition=%q\\n",
				m.InstallFilename(),
				"PRESIGNED", "PRESIGNED", "PRESIGNED", "PRESIGNED", m.PartitionTag(ctx.DeviceConfig()))
		}
	}

	ctx.Build(pctx, android.BuildParams{
		Rule:        android.WriteFile,
		Description: "apexkeys.txt",
		Output:      s.output,
		Args: map[string]string{
			"content": filecontent.String(),
		},
	})
}

func apexKeysTextFactory() android.Singleton {
	return &apexKeysText{}
}

func (s *apexKeysText) MakeVars(ctx android.MakeVarsContext) {
	ctx.Strict("SOONG_APEX_KEYS_FILE", s.output.String())
}
