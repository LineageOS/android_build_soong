package config

var (
	RustAllowedPaths = []string{
		"external/rust/crates",
		"external/crosvm",
		"external/adhd",
	}

	RustModuleTypes = []string{
		"rust_binary",
		"rust_binary_host",
		"rust_library",
		"rust_library_dylib",
		"rust_library_rlib",
		"rust_library_host",
		"rust_library_host_dylib",
		"rust_library_host_rlib",
		"rust_proc_macro",
	}
)
