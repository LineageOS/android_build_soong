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
		"FrontendStub_cc",
		"FrontendStub_h",
		"ImageProcessing-rscript",
		"ImageProcessing2-rscript",
		"ImageProcessingJB-rscript",
		"MultiDexLegacyTestApp_genrule",
		"PacketStreamerStub_cc",
		"PacketStreamerStub_h",
		"RSTest-rscript",
		"RSTest_v11-rscript",
		"RSTest_v14-rscript",
		"RSTest_v16-rscript",
		"Refocus-rscript",
		"RsBalls-rscript",
		"ScriptGroupTest-rscript",
		"TracingVMProtoStub_cc",
		"TracingVMProtoStub_h",
		"VehicleServerProtoStub_cc",
		"VehicleServerProtoStub_cc@2.0-grpc-trout",
		"VehicleServerProtoStub_cc@default-grpc",
		"VehicleServerProtoStub_h",
		"VehicleServerProtoStub_h@2.0-grpc-trout",
		"VehicleServerProtoStub_h@default-grpc",
		"aidl-golden-test-build-hook-gen",
		"aidl_camera_build_version",
		"android-cts-verifier",
		"android-support-multidex-instrumentation-version",
		"android-support-multidex-version",
		"angle_commit_id",
		"atest_integration_fake_src",
		"awkgram.tab.h",
		"c2hal_test_genc++",
		"c2hal_test_genc++_headers",
		"camera-its",
		"checkIn-service-stub-lite",
		"chre_atoms_log.h",
		"cronet_aml_base_android_runtime_jni_headers",
		"cronet_aml_base_android_runtime_jni_headers__testing",
		"cronet_aml_base_android_runtime_unchecked_jni_headers",
		"cronet_aml_base_android_runtime_unchecked_jni_headers__testing",
		"deqp_spvtools_update_build_version",
		"emp_ematch.yacc.c",
		"emp_ematch.yacc.h",
		"fdt_test_tree_empty_memory_range_dtb",
		"fdt_test_tree_multiple_memory_ranges_dtb",
		"fdt_test_tree_one_memory_range_dtb",
		"gen_corrupt_rebootless_apex",
		"gen_key_mismatch_capex",
		"libbssl_sys_src_nostd",
		"libc_musl_sysroot_bits",
		"libchrome-crypto-include",
		"libchrome-include",
		"libcore-non-cts-tests-txt",
		"libxml2_schema_fuzz_corpus",
		"libxml2_xml_fuzz_corpus",
		"pixelatoms_defs.h",
		"pixelstatsatoms.cpp",
		"pixelstatsatoms.h",
		"pvmfw_fdt_template_rs",
		"r8retrace-dexdump-sample-app",
		"r8retrace-run-retrace",
		"seller-frontend-service-stub-lite",
		"swiftshader_spvtools_update_build_version",
		"ue_unittest_erofs_imgs",
		"vm-tests-tf-lib",
		"vndk_abi_dump_zip",
		// go/keep-sorted end
	}

	SandboxingDenyPathList = []string{
		// go/keep-sorted start
		"art/test",
		"external/cronet",
		// go/keep-sorted end
	}
)
