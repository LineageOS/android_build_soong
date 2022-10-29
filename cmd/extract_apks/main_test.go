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

package main

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	bp "android/soong/cmd/extract_apks/bundle_proto"
	"android/soong/third_party/zip"
)

type testConfigDesc struct {
	name         string
	targetConfig TargetConfig
	expected     SelectionResult
}

type testDesc struct {
	protoText string
	configs   []testConfigDesc
}

func TestSelectApks_ApkSet(t *testing.T) {
	testCases := []testDesc{
		{
			protoText: `
variant {
  targeting {
    sdk_version_targeting {
      value { min { value: 29 } } } }
  apk_set {
    module_metadata {
      name: "base" targeting {} delivery_type: INSTALL_TIME }
    apk_description {
      targeting {
        screen_density_targeting {
          value { density_alias: LDPI } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "splits/base-ldpi.apk"
      split_apk_metadata { split_id: "config.ldpi" } }
    apk_description {
      targeting {
        screen_density_targeting {
          value { density_alias: MDPI } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "splits/base-mdpi.apk"
      split_apk_metadata { split_id: "config.mdpi" } }
    apk_description {
      targeting {
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "splits/base-master.apk"
      split_apk_metadata { is_master_split: true } }
    apk_description {
      targeting {
        abi_targeting {
          value { alias: ARMEABI_V7A }
          alternatives { alias: ARM64_V8A }
          alternatives { alias: X86 }
          alternatives { alias: X86_64 } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "splits/base-armeabi_v7a.apk"
      split_apk_metadata { split_id: "config.armeabi_v7a" } }
    apk_description {
      targeting {
        abi_targeting {
          value { alias: ARM64_V8A }
          alternatives { alias: ARMEABI_V7A }
          alternatives { alias: X86 }
          alternatives { alias: X86_64 } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "splits/base-arm64_v8a.apk"
      split_apk_metadata { split_id: "config.arm64_v8a" } }
    apk_description {
      targeting {
        abi_targeting {
          value { alias: X86 }
          alternatives { alias: ARMEABI_V7A }
          alternatives { alias: ARM64_V8A }
          alternatives { alias: X86_64 } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "splits/base-x86.apk"
      split_apk_metadata { split_id: "config.x86" } }
    apk_description {
      targeting {
        abi_targeting {
          value { alias: X86_64 }
          alternatives { alias: ARMEABI_V7A }
          alternatives { alias: ARM64_V8A }
          alternatives { alias: X86 } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "splits/base-x86_64.apk"
      split_apk_metadata { split_id: "config.x86_64" } } }
}
bundletool {
  version: "0.10.3" }

`,
			configs: []testConfigDesc{
				{
					name: "one",
					targetConfig: TargetConfig{
						sdkVersion: 29,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARMEABI_V7A: 0,
							bp.Abi_ARM64_V8A:   1,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"splits/base-ldpi.apk",
							"splits/base-mdpi.apk",
							"splits/base-master.apk",
							"splits/base-armeabi_v7a.apk",
						},
					},
				},
				{
					name: "two",
					targetConfig: TargetConfig{
						sdkVersion: 29,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_LDPI: true,
						},
						abis: map[bp.Abi_AbiAlias]int{},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"splits/base-ldpi.apk",
							"splits/base-master.apk",
						},
					},
				},
				{
					name: "three",
					targetConfig: TargetConfig{
						sdkVersion: 20,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_LDPI: true,
						},
						abis: map[bp.Abi_AbiAlias]int{},
					},
					expected: SelectionResult{
						"",
						nil,
					},
				},
				{
					name: "four",
					targetConfig: TargetConfig{
						sdkVersion: 29,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_MDPI: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARM64_V8A:   0,
							bp.Abi_ARMEABI_V7A: 1,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"splits/base-mdpi.apk",
							"splits/base-master.apk",
							"splits/base-arm64_v8a.apk",
						},
					},
				},
			},
		},
		{
			protoText: `
variant {
  targeting {
    sdk_version_targeting {
      value { min { value: 10000 } } } }
  apk_set {
    module_metadata {
      name: "base" targeting {} delivery_type: INSTALL_TIME }
    apk_description {
      targeting {
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "splits/base-master.apk"
      split_apk_metadata { is_master_split: true } } } }`,
			configs: []testConfigDesc{
				{
					name: "Prerelease",
					targetConfig: TargetConfig{
						sdkVersion:       30,
						screenDpi:        map[bp.ScreenDensity_DensityAlias]bool{},
						abis:             map[bp.Abi_AbiAlias]int{},
						allowPrereleased: true,
					},
					expected: SelectionResult{
						"base",
						[]string{"splits/base-master.apk"},
					},
				},
			},
		},
		{
			protoText: `
variant {
  targeting {
    sdk_version_targeting {
      value { min { value: 29 } } } }
  apk_set {
    module_metadata {
      name: "base" targeting {} delivery_type: INSTALL_TIME }
    apk_description {
      targeting {}
      path: "universal.apk"
      standalone_apk_metadata { fused_module_name: "base" } } } }`,
			configs: []testConfigDesc{
				{
					name:         "Universal",
					targetConfig: TargetConfig{sdkVersion: 30},
					expected: SelectionResult{
						"base",
						[]string{"universal.apk"},
					},
				},
			},
		},
	}
	for _, testCase := range testCases {
		var toc bp.BuildApksResult
		if err := prototext.Unmarshal([]byte(testCase.protoText), &toc); err != nil {
			t.Fatal(err)
		}
		for _, config := range testCase.configs {
			actual := selectApks(&toc, config.targetConfig)
			if !reflect.DeepEqual(config.expected, actual) {
				t.Errorf("%s: expected %v, got %v", config.name, config.expected, actual)
			}
		}
	}
}

func TestSelectApks_ApexSet(t *testing.T) {
	testCases := []testDesc{
		{
			protoText: `
variant {
  targeting {
    sdk_version_targeting {
      value { min { value: 29 } } } }
  apk_set {
    module_metadata {
      name: "base" targeting {} delivery_type: INSTALL_TIME }
    apk_description {
      targeting {
        multi_abi_targeting {
          value { abi { alias: ARMEABI_V7A } }
          alternatives { abi { alias: ARM64_V8A } }
          alternatives { abi { alias: X86 } }
          alternatives { abi { alias: X86_64 } } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "standalones/standalone-armeabi_v7a.apex"
      apex_apk_metadata { } }
    apk_description {
      targeting {
        multi_abi_targeting {
          value { abi { alias: ARM64_V8A } }
          alternatives { abi { alias: ARMEABI_V7A } }
          alternatives { abi { alias: X86 } }
          alternatives { abi { alias: X86_64 } } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "standalones/standalone-arm64_v8a.apex"
      apex_apk_metadata { } }
    apk_description {
      targeting {
        multi_abi_targeting {
          value { abi { alias: X86 } }
          alternatives { abi { alias: ARMEABI_V7A } }
          alternatives { abi { alias: ARM64_V8A } }
          alternatives { abi { alias: X86_64 } } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "standalones/standalone-x86.apex"
      apex_apk_metadata { } }
    apk_description {
      targeting {
        multi_abi_targeting {
          value { abi { alias: X86_64 } }
          alternatives { abi { alias: ARMEABI_V7A } }
          alternatives { abi { alias: ARM64_V8A } }
          alternatives { abi { alias: X86 } } }
        sdk_version_targeting {
          value { min { value: 21 } } } }
      path: "standalones/standalone-x86_64.apex"
      apex_apk_metadata { } } }
}
bundletool {
  version: "0.10.3" }

`,
			configs: []testConfigDesc{
				{
					name: "order matches priorities",
					targetConfig: TargetConfig{
						sdkVersion: 29,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARM64_V8A:   0,
							bp.Abi_ARMEABI_V7A: 1,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-arm64_v8a.apex",
						},
					},
				},
				{
					name: "order doesn't match priorities",
					targetConfig: TargetConfig{
						sdkVersion: 29,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARMEABI_V7A: 0,
							bp.Abi_ARM64_V8A:   1,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-arm64_v8a.apex",
						},
					},
				},
				{
					name: "single choice",
					targetConfig: TargetConfig{
						sdkVersion: 29,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARMEABI_V7A: 0,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-armeabi_v7a.apex",
						},
					},
				},
				{
					name: "cross platform",
					targetConfig: TargetConfig{
						sdkVersion: 29,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARM64_V8A: 0,
							bp.Abi_MIPS64:    1,
							bp.Abi_X86:       2,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-x86.apex",
						},
					},
				},
			},
		},
	}
	for _, testCase := range testCases {
		var toc bp.BuildApksResult
		if err := prototext.Unmarshal([]byte(testCase.protoText), &toc); err != nil {
			t.Fatal(err)
		}
		for _, config := range testCase.configs {
			actual := selectApks(&toc, config.targetConfig)
			if !reflect.DeepEqual(config.expected, actual) {
				t.Errorf("%s: expected %v, got %v", config.name, config.expected, actual)
			}
		}
	}
}

func TestSelectApks_ApexSet_Variants(t *testing.T) {
	testCases := []testDesc{
		{
			protoText: `
variant {
	targeting {
		sdk_version_targeting {value {min {value: 29}}}
		multi_abi_targeting {
			value {abi {alias: ARMEABI_V7A}}
			alternatives {
				abi {alias: ARMEABI_V7A}
				abi {alias: ARM64_V8A}
			}
			alternatives {abi {alias: ARM64_V8A}}
			alternatives {abi {alias: X86}}
			alternatives {
				abi {alias: X86}
				abi {alias: X86_64}
			}
		}
	}
	apk_set {
		module_metadata {
			name: "base"
			delivery_type: INSTALL_TIME
		}
		apk_description {
			targeting {
				multi_abi_targeting {
					value {abi {alias: ARMEABI_V7A}}
					alternatives {
						abi {alias: ARMEABI_V7A}
						abi {alias: ARM64_V8A}
					}
					alternatives {abi {alias: ARM64_V8A}}
					alternatives {abi {alias: X86}}
					alternatives {
						abi {alias: X86}
						abi {alias: X86_64}
					}
				}
			}
			path: "standalones/standalone-armeabi_v7a.apex"
		}
	}
	variant_number: 0
}
variant {
	targeting {
		sdk_version_targeting {value {min {value: 29}}}
		multi_abi_targeting {
			value {abi {alias: ARM64_V8A}}
			alternatives {abi {alias: ARMEABI_V7A}}
			alternatives {
				abi {alias: ARMEABI_V7A}
				abi {alias: ARM64_V8A}
			}
			alternatives {abi {alias: X86}}
			alternatives {
				abi {alias: X86}
				abi {alias: X86_64}
			}
		}
	}
	apk_set {
		module_metadata {
			name: "base"
			delivery_type: INSTALL_TIME
		}
		apk_description {
			targeting {
				multi_abi_targeting {
					value {abi {alias: ARM64_V8A}}
					alternatives {abi {alias: ARMEABI_V7A}}
					alternatives {
						abi {alias: ARMEABI_V7A}
						abi {alias: ARM64_V8A}
					}
					alternatives {abi {alias: X86}}
					alternatives {
						abi {alias: X86}
						abi {alias: X86_64}
					}
				}
			}
			path: "standalones/standalone-arm64_v8a.apex"
		}
	}
	variant_number: 1
}
variant {
	targeting {
		sdk_version_targeting {value {min {value: 29}}}
		multi_abi_targeting {
			value {
				abi {alias: ARMEABI_V7A}
				abi {alias: ARM64_V8A}
			}
			alternatives {abi {alias: ARMEABI_V7A}}
			alternatives {abi {alias: ARM64_V8A}}
			alternatives {abi {alias: X86}}
			alternatives {
				abi {alias: X86}
				abi {alias: X86_64}
			}
		}
	}
	apk_set {
		module_metadata {
			name: "base"
			delivery_type: INSTALL_TIME
		}
		apk_description {
			targeting {
				multi_abi_targeting {
					value {
						abi {alias: ARMEABI_V7A}
						abi {alias: ARM64_V8A}
					}
					alternatives {abi {alias: ARMEABI_V7A}}
					alternatives {abi {alias: ARM64_V8A}}
					alternatives {abi {alias: X86}}
					alternatives {
						abi {alias: X86}
						abi {alias: X86_64}
					}
				}
			}
			path: "standalones/standalone-armeabi_v7a.arm64_v8a.apex"
		}
	}
	variant_number: 2
}
variant {
	targeting {
		sdk_version_targeting {value {min {value: 29}}}
		multi_abi_targeting {
			value {abi {alias: X86}}
			alternatives {abi {alias: ARMEABI_V7A}}
			alternatives {
				abi {alias: ARMEABI_V7A}
				abi {alias: ARM64_V8A}
			}
			alternatives {abi {alias: ARM64_V8A}}
			alternatives {
				abi {alias: X86}
				abi {alias: X86_64}
			}
		}
	}
	apk_set {
		module_metadata {
			name: "base"
			delivery_type: INSTALL_TIME
		}
		apk_description {
			targeting {
				multi_abi_targeting {
					value {abi {alias: X86}}
					alternatives {abi {alias: ARMEABI_V7A}}
					alternatives {
						abi {alias: ARMEABI_V7A}
						abi {alias: ARM64_V8A}
					}
					alternatives {abi {alias: ARM64_V8A}}
					alternatives {
						abi {alias: X86}
						abi {alias: X86_64}
					}
				}
			}
			path: "standalones/standalone-x86.apex"
		}
	}
	variant_number: 3
}
variant {
	targeting {
		sdk_version_targeting {value {min {value: 29}}}
		multi_abi_targeting {
			value {
				abi {alias: X86}
				abi {alias: X86_64}
			}
			alternatives {abi {alias: ARMEABI_V7A}}
			alternatives {
				abi {alias: ARMEABI_V7A}
				abi {alias: ARM64_V8A}
			}
			alternatives {abi {alias: ARM64_V8A}}
			alternatives {abi {alias: X86}}
		}
	}
	apk_set {
		module_metadata {
			name: "base"
			delivery_type: INSTALL_TIME
		}
		apk_description {
			targeting {
				multi_abi_targeting {
					value {
						abi {alias: X86}
						abi {alias: X86_64}
					}
					alternatives {abi {alias: ARMEABI_V7A}}
					alternatives {
						abi {alias: ARMEABI_V7A}
						abi {alias: ARM64_V8A}
					}
					alternatives {abi {alias: ARM64_V8A}}
					alternatives {abi {alias: X86}}
				}
			}
			path: "standalones/standalone-x86.x86_64.apex"
		}
  }
  variant_number: 4
}
`,
			configs: []testConfigDesc{
				{
					name: "multi-variant multi-target ARM",
					targetConfig: TargetConfig{
						sdkVersion: 33,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARM64_V8A:   0,
							bp.Abi_ARMEABI_V7A: 1,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-armeabi_v7a.arm64_v8a.apex",
						},
					},
				},
				{
					name: "multi-variant single-target arm",
					targetConfig: TargetConfig{
						sdkVersion: 33,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARMEABI_V7A: 0,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-armeabi_v7a.apex",
						},
					},
				},
				{
					name: "multi-variant single-target arm64",
					targetConfig: TargetConfig{
						sdkVersion: 33,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARM64_V8A: 0,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-arm64_v8a.apex",
						},
					},
				},
				{
					name: "multi-variant multi-target x86",
					targetConfig: TargetConfig{
						sdkVersion: 33,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_X86:    0,
							bp.Abi_X86_64: 1,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-x86.x86_64.apex",
						},
					},
				},
				{
					name: "multi-variant single-target x86",
					targetConfig: TargetConfig{
						sdkVersion: 33,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_X86: 0,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-x86.apex",
						},
					},
				},
				{
					name: "multi-variant single-target x86_64",
					targetConfig: TargetConfig{
						sdkVersion: 33,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_X86_64: 0,
						},
					},
					expected: SelectionResult{},
				},
				{
					name: "multi-variant multi-target cross-target",
					targetConfig: TargetConfig{
						sdkVersion: 33,
						screenDpi: map[bp.ScreenDensity_DensityAlias]bool{
							bp.ScreenDensity_DENSITY_UNSPECIFIED: true,
						},
						abis: map[bp.Abi_AbiAlias]int{
							bp.Abi_ARM64_V8A: 0,
							bp.Abi_X86_64:    1,
						},
					},
					expected: SelectionResult{
						"base",
						[]string{
							"standalones/standalone-arm64_v8a.apex",
						},
					},
				},
			},
		},
	}
	for _, testCase := range testCases {
		var toc bp.BuildApksResult
		if err := prototext.Unmarshal([]byte(testCase.protoText), &toc); err != nil {
			t.Fatal(err)
		}
		for _, config := range testCase.configs {
			t.Run(config.name, func(t *testing.T) {
				actual := selectApks(&toc, config.targetConfig)
				if !reflect.DeepEqual(config.expected, actual) {
					t.Errorf("expected %v, got %v", config.expected, actual)
				}
			})
		}
	}
}

type testZip2ZipWriter struct {
	entries map[string]string
}

func (w testZip2ZipWriter) CopyFrom(file *zip.File, out string) error {
	if x, ok := w.entries[out]; ok {
		return fmt.Errorf("%s and %s both write to %s", x, file.Name, out)
	}
	w.entries[out] = file.Name
	return nil
}

type testCaseWriteApks struct {
	name       string
	moduleName string
	stem       string
	partition  string
	// what we write from what
	zipEntries       map[string]string
	expectedApkcerts []string
}

func TestWriteApks(t *testing.T) {
	testCases := []testCaseWriteApks{
		{
			name:       "splits",
			moduleName: "mybase",
			stem:       "Foo",
			partition:  "system",
			zipEntries: map[string]string{
				"Foo.apk":       "splits/mybase-master.apk",
				"Foo-xhdpi.apk": "splits/mybase-xhdpi.apk",
			},
			expectedApkcerts: []string{
				`name="Foo-xhdpi.apk" certificate="PRESIGNED" private_key="" partition="system"`,
				`name="Foo.apk" certificate="PRESIGNED" private_key="" partition="system"`,
			},
		},
		{
			name:       "universal",
			moduleName: "base",
			stem:       "Bar",
			partition:  "product",
			zipEntries: map[string]string{
				"Bar.apk": "universal.apk",
			},
			expectedApkcerts: []string{
				`name="Bar.apk" certificate="PRESIGNED" private_key="" partition="product"`,
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testZipBuf := &bytes.Buffer{}
			testZip := zip.NewWriter(testZipBuf)
			for _, in := range testCase.zipEntries {
				f, _ := testZip.Create(in)
				f.Write([]byte(in))
			}
			testZip.Close()

			zipReader, _ := zip.NewReader(bytes.NewReader(testZipBuf.Bytes()), int64(testZipBuf.Len()))

			apkSet := ApkSet{entries: make(map[string]*zip.File)}
			sel := SelectionResult{moduleName: testCase.moduleName}
			for _, f := range zipReader.File {
				apkSet.entries[f.Name] = f
				sel.entries = append(sel.entries, f.Name)
			}

			zipWriter := testZip2ZipWriter{make(map[string]string)}
			outWriter := &bytes.Buffer{}
			config := TargetConfig{stem: testCase.stem}
			apkcerts, err := apkSet.writeApks(sel, config, outWriter, zipWriter, testCase.partition)
			if err != nil {
				t.Error(err)
			}
			expectedZipEntries := make(map[string]string)
			for k, v := range testCase.zipEntries {
				if k != testCase.stem+".apk" {
					expectedZipEntries[k] = v
				}
			}
			if !reflect.DeepEqual(expectedZipEntries, zipWriter.entries) {
				t.Errorf("expected zip entries %v, got %v", testCase.zipEntries, zipWriter.entries)
			}
			if !reflect.DeepEqual(testCase.expectedApkcerts, apkcerts) {
				t.Errorf("expected apkcerts %v, got %v", testCase.expectedApkcerts, apkcerts)
			}
			if g, w := outWriter.String(), testCase.zipEntries[testCase.stem+".apk"]; !reflect.DeepEqual(g, w) {
				t.Errorf("expected output file contents %q, got %q", testCase.stem+".apk", outWriter.String())
			}
		})
	}
}
