package config

var (
	// When adding a new path below, add a rustfmt.toml file at the root of
	// the repository and enable the rustfmt repo hook. See aosp/1458238
	// for an example.
	// TODO(b/160223496): enable rustfmt globally.
	RustAllowedPaths = []string{
		"device/google/cuttlefish",
		"external/adhd",
		"external/boringssl",
		"external/crosvm",
		"external/libchromeos-rs",
		"external/minijail",
		"external/open-dice",
		"external/rust",
		"external/selinux/libselinux",
		"external/uwb",
		"external/vm_tools/p9",
		"frameworks/native/libs/binder/rust",
		"frameworks/proto_logging/stats",
		"hardware/interfaces/security",
		"hardware/interfaces/uwb",
		"packages/modules/Bluetooth",
		"packages/modules/DnsResolver",
		"packages/modules/Uwb",
		"packages/modules/Virtualization",
		"platform_testing/tests/codecoverage/native/rust",
		"prebuilts/rust",
		"system/core/debuggerd/rust",
		"system/core/libstats/pull_rust",
		"system/core/trusty/libtrusty-rs",
		"system/extras/profcollectd",
		"system/extras/simpleperf",
		"system/hardware/interfaces/keystore2",
		"system/librustutils",
		"system/logging/liblog",
		"system/logging/rust",
		"system/nfc",
		"system/security",
		"system/tools/aidl",
		"tools/security/fuzzing/example_rust_fuzzer",
		"tools/security/fuzzing/orphans",
		"tools/security/remote_provisioning/cert_validator",
		"tools/vendor",
		"vendor/",
	}

	DownstreamRustAllowedPaths = []string{
		// Add downstream allowed Rust paths here.
	}

	RustModuleTypes = []string{
		// Don't add rust_bindgen or rust_protobuf as these are code generation modules
		// and can be expected to be in paths without Rust code.
		"rust_benchmark",
		"rust_benchmark_host",
		"rust_binary",
		"rust_binary_host",
		"rust_library",
		"rust_library_dylib",
		"rust_library_rlib",
		"rust_ffi",
		"rust_ffi_shared",
		"rust_ffi_static",
		"rust_fuzz",
		"rust_library_host",
		"rust_library_host_dylib",
		"rust_library_host_rlib",
		"rust_ffi_host",
		"rust_ffi_host_shared",
		"rust_ffi_host_static",
		"rust_proc_macro",
		"rust_test",
		"rust_test_host",
	}
)
