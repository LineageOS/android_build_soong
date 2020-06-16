package config

var (
	RustAllowedPaths = []string{
		"external/minijail",
		"external/rust",
		"external/crosvm",
		"external/adhd",
		"prebuilts/rust",
	}

	RustModuleTypes = []string{
		"rust_binary",
		"rust_binary_host",
		"rust_library",
		"rust_library_dylib",
		"rust_library_rlib",
		"rust_library_shared",
		"rust_library_static",
		"rust_library_host",
		"rust_library_host_dylib",
		"rust_library_host_rlib",
		"rust_library_host_shared",
		"rust_library_host_static",
		"rust_proc_macro",
		"rust_test",
		"rust_test_host",
	}
)
