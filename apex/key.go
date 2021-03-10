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
	registerApexKeyBuildComponents(android.InitRegistrationContext)
}

func registerApexKeyBuildComponents(ctx android.RegistrationContext) {
	ctx.RegisterModuleType("apex_key", ApexKeyFactory)
	ctx.RegisterSingletonType("apex_keys_text", apexKeysTextFactory)
}

type apexKey struct {
	android.ModuleBase

	properties apexKeyProperties

	publicKeyFile  android.Path
	privateKeyFile android.Path

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
		m.publicKeyFile = android.PathForModuleSrc(ctx, String(m.properties.Public_key))
	} else {
		m.publicKeyFile = ctx.Config().ApexKeyDir(ctx).Join(ctx, String(m.properties.Public_key))
		// If not found, fall back to the local key pairs
		if !android.ExistentPathForSource(ctx, m.publicKeyFile.String()).Valid() {
			m.publicKeyFile = android.PathForModuleSrc(ctx, String(m.properties.Public_key))
		}
	}

	if android.SrcIsModule(String(m.properties.Private_key)) != "" {
		m.privateKeyFile = android.PathForModuleSrc(ctx, String(m.properties.Private_key))
	} else {
		m.privateKeyFile = ctx.Config().ApexKeyDir(ctx).Join(ctx, String(m.properties.Private_key))
		if !android.ExistentPathForSource(ctx, m.privateKeyFile.String()).Valid() {
			m.privateKeyFile = android.PathForModuleSrc(ctx, String(m.properties.Private_key))
		}
	}

	pubKeyName := m.publicKeyFile.Base()[0 : len(m.publicKeyFile.Base())-len(m.publicKeyFile.Ext())]
	privKeyName := m.privateKeyFile.Base()[0 : len(m.privateKeyFile.Base())-len(m.privateKeyFile.Ext())]

	if m.properties.Public_key != nil && m.properties.Private_key != nil && pubKeyName != privKeyName {
		ctx.ModuleErrorf("public_key %q (keyname:%q) and private_key %q (keyname:%q) do not have same keyname",
			m.publicKeyFile.String(), pubKeyName, m.privateKeyFile, privKeyName)
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
	type apexKeyEntry struct {
		name                 string
		presigned            bool
		publicKey            string
		privateKey           string
		containerCertificate string
		containerPrivateKey  string
		partition            string
	}
	toString := func(e apexKeyEntry) string {
		format := "name=%q public_key=%q private_key=%q container_certificate=%q container_private_key=%q partition=%q\n"
		if e.presigned {
			return fmt.Sprintf(format, e.name, "PRESIGNED", "PRESIGNED", "PRESIGNED", "PRESIGNED", e.partition)
		} else {
			return fmt.Sprintf(format, e.name, e.publicKey, e.privateKey, e.containerCertificate, e.containerPrivateKey, e.partition)
		}
	}

	apexKeyMap := make(map[string]apexKeyEntry)
	ctx.VisitAllModules(func(module android.Module) {
		if m, ok := module.(*apexBundle); ok && m.Enabled() && m.installable() {
			pem, key := m.getCertificateAndPrivateKey(ctx)
			apexKeyMap[m.Name()] = apexKeyEntry{
				name:                 m.Name() + ".apex",
				presigned:            false,
				publicKey:            m.publicKeyFile.String(),
				privateKey:           m.privateKeyFile.String(),
				containerCertificate: pem.String(),
				containerPrivateKey:  key.String(),
				partition:            m.PartitionTag(ctx.DeviceConfig()),
			}
		}
	})

	// Find prebuilts and let them override apexBundle if they are preferred
	ctx.VisitAllModules(func(module android.Module) {
		if m, ok := module.(*Prebuilt); ok && m.Enabled() && m.installable() &&
			m.Prebuilt().UsePrebuilt() {
			apexKeyMap[m.BaseModuleName()] = apexKeyEntry{
				name:      m.InstallFilename(),
				presigned: true,
				partition: m.PartitionTag(ctx.DeviceConfig()),
			}
		}
	})

	// Find apex_set and let them override apexBundle or prebuilts. This is done in a separate pass
	// so that apex_set are not overridden by prebuilts.
	ctx.VisitAllModules(func(module android.Module) {
		if m, ok := module.(*ApexSet); ok && m.Enabled() {
			entry := apexKeyEntry{
				name:      m.InstallFilename(),
				presigned: true,
				partition: m.PartitionTag(ctx.DeviceConfig()),
			}
			apexKeyMap[m.BaseModuleName()] = entry
		}
	})

	// iterating over map does not give consistent ordering in golang
	var moduleNames []string
	for key, _ := range apexKeyMap {
		moduleNames = append(moduleNames, key)
	}
	sort.Strings(moduleNames)

	var filecontent strings.Builder
	for _, name := range moduleNames {
		filecontent.WriteString(toString(apexKeyMap[name]))
	}
	android.WriteFileRule(ctx, s.output, filecontent.String())
}

func apexKeysTextFactory() android.Singleton {
	return &apexKeysText{}
}

func (s *apexKeysText) MakeVars(ctx android.MakeVarsContext) {
	ctx.Strict("SOONG_APEX_KEYS_FILE", s.output.String())
}
