// Copyright 2023 Google Inc. All rights reserved.
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

package genrule

var (
	DepfileAllowList = []string{
		// go/keep-sorted start
		"depfile_allowed_for_test",
		"tflite_support_metadata_schema",
		"tflite_support_spm_config",
		"tflite_support_spm_encoder_config",
		// go/keep-sorted end
	}

	SandboxingDenyModuleList = []string{
		// go/keep-sorted start
		"CtsApkVerityTestDebugFiles",
		"ScriptGroupTest-rscript",
		"aidl-golden-test-build-hook-gen",
		"aidl_camera_build_version",
		"camera-its",
		"checkIn-service-stub-lite",
		"chre_atoms_log.h",
		"cronet_aml_base_android_runtime_jni_headers",
		"cronet_aml_base_android_runtime_jni_headers__testing",
		"cronet_aml_base_android_runtime_unchecked_jni_headers",
		"cronet_aml_base_android_runtime_unchecked_jni_headers__testing",
		"deqp_spvtools_update_build_version",
		"gen_corrupt_rebootless_apex",
		"gen_key_mismatch_capex",
		"libbssl_sys_src_nostd",
		"libc_musl_sysroot_bits",
		"libcore-non-cts-tests-txt",
		"pvmfw_fdt_template_rs",
		"seller-frontend-service-stub-lite",
		"swiftshader_spvtools_update_build_version",
		"ue_unittest_erofs_imgs",
		"vm-tests-tf-lib",
		// go/keep-sorted end
	}

	SandboxingDenyPathList = []string{
		// go/keep-sorted start
		"art/test",
		"external/cronet",
		// go/keep-sorted end
	}
)
