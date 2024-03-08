# starlark_import package

This allows soong to read constant information from starlark files. At package initialization
time, soong will read `build/bazel/constants_exported_to_soong.bzl`, and then make the
variables from that file available via `starlark_import.GetStarlarkValue()`. So to import
a new variable, it must be added to `constants_exported_to_soong.bzl` and then it can
be accessed by name.

Only constant information can be read, since this is not a full bazel execution but a
standalone starlark interpreter. This means you can't use bazel contructs like `rule`,
`provider`, `select`, `glob`, etc.

All starlark files that were loaded must be added as ninja deps that cause soong to rerun.
The loaded files can be retrieved via `starlark_import.GetNinjaDeps()`.
