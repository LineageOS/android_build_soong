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
		"gen_uwb_core_proto",
		"tflite_support_metadata_schema",
		"tflite_support_spm_config",
		"tflite_support_spm_encoder_config",
		// go/keep-sorted end
	}

	SandboxingDenyModuleList = []string{
		// go/keep-sorted start
		"ControlEnvProxyServerProto_cc",
		"ControlEnvProxyServerProto_h",
		"CtsApkVerityTestDebugFiles",
		"FrontendStub_cc",
		"FrontendStub_h",
		"ImageProcessing-rscript",
		"ImageProcessing2-rscript",
		"ImageProcessingJB-rscript",
		"MultiDexLegacyTestApp_genrule",
		"PackageManagerServiceServerTests_apks_as_resources",
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
		"apexer_test_host_tools",
		"atest_integration_fake_src",
		"authfs_test_apk_assets",
		"awkgram.tab.h",
		"c2hal_test_genc++",
		"c2hal_test_genc++_headers",
		"camera-its",
		"checkIn-service-stub-lite",
		"chre_atoms_log.h",
		"common-profile-text-protos",
		"core-tests-smali-dex",
		"cronet_aml_base_android_runtime_jni_headers",
		"cronet_aml_base_android_runtime_jni_headers__testing",
		"cronet_aml_base_android_runtime_unchecked_jni_headers",
		"cronet_aml_base_android_runtime_unchecked_jni_headers__testing",
		"deqp_spvtools_update_build_version",
		"egl_extensions_functions_hdr",
		"egl_functions_hdr",
		"emp_ematch.yacc.c",
		"emp_ematch.yacc.h",
		"fdt_test_tree_empty_memory_range_dtb",
		"fdt_test_tree_multiple_memory_ranges_dtb",
		"fdt_test_tree_one_memory_range_dtb",
		"futility_cmds",
		"gen_corrupt_rebootless_apex",
		"gen_corrupt_superblock_apex",
		"gen_key_mismatch_capex",
		"gen_manifest_mismatch_apex_no_hashtree",
		"generate_hash_v1",
		"gles1_core_functions_hdr",
		"gles1_extensions_functions_hdr",
		"gles2_core_functions_hdr",
		"gles2_extensions_functions_hdr",
		"gles31_only_functions_hdr",
		"gles3_only_functions_hdr",
		"lib-test-profile-text-protos",
		"libbssl_sys_src_nostd",
		"libc_musl_sysroot_bits",
		"libchrome-crypto-include",
		"libchrome-include",
		"libcore-non-cts-tests-txt",
		"libmojo_jni_headers",
		"libxml2_schema_fuzz_corpus",
		"libxml2_xml_fuzz_corpus",
		"measure_io_as_jar",
		"pandora-python-gen-src",
		"pixelatoms_defs.h",
		"pixelstatsatoms.cpp",
		"pixelstatsatoms.h",
		"pvmfw_fdt_template_rs",
		"r8retrace-dexdump-sample-app",
		"r8retrace-run-retrace",
		"sample-profile-text-protos",
		"seller-frontend-service-stub-lite",
		"services.core.protologsrc",
		"statsd-config-protos",
		"swiftshader_spvtools_update_build_version",
		"temp_layoutlib",
		"ue_unittest_erofs_imgs",
		"uwb_core_artifacts",
		"vm-tests-tf-lib",
		"vndk_abi_dump_zip",
		"vts_vndk_abi_dump_zip",
		"wm_shell_protolog_src",
		"wmtests.protologsrc",
		// go/keep-sorted end
	}

	SandboxingDenyPathList = []string{
		// go/keep-sorted start
		"art/test",
		"external/cronet",
		// go/keep-sorted end
	}
)
