# Clang-Tidy Rules and Checks

Android C/C++ source files can be checked by clang-tidy for issues like
coding style, error-prone/performance patterns, and static flow analysis.
See the official
[clang-tidy document](https://clang.llvm.org/extra/clang-tidy)
and list of
[clang-tidy checks](https://clang.llvm.org/extra/clang-tidy/checks/list.html).

## Global defaults for Android builds

The simplest way to enable clang-tidy checks is
to set environment variable `WITH_TIDY`.
```
$ WITH_TIDY=1 make
```

This will turn on the global default to run clang-tidy for every required
C/C++ source file compilation. The global default clang-tidy checks
do not include time-consuming static analyzer checks. To enable those
checks, set the `CLANG_ANALYZER_CHECKS` variable.
```
$ WITH_TIDY=1 CLANG_ANALYZER_CHECKS=1 make
```

The default global clang-tidy checks and flags are defined in
[build/soong/cc/config/tidy.go](https://android.googlesource.com/platform/build/soong/+/refs/heads/main/cc/config/tidy.go).


## Module clang-tidy properties

The global default can be overwritten by module properties in Android.bp.

### `tidy`, `tidy_checks`, and `ALLOW_LOCAL_TIDY_TRUE`

For example, in
[system/bpf/Android.bp](https://android.googlesource.com/platform/system/bpf/+/refs/heads/main/Android.bp),
clang-tidy is enabled explicitly and with a different check list:
```
cc_defaults {
    name: "bpf_defaults",
    // snipped
    tidy: true,
    tidy_checks: [
        "android-*",
        "cert-*",
        "-cert-err34-c",
        "clang-analyzer-security*",
        // Disabling due to many unavoidable warnings from POSIX API usage.
        "-google-runtime-int",
    ],
}
```
That means in normal builds, even without `WITH_TIDY=1`,
the modules that use `bpf_defaults` _should_ run clang-tidy
over C/C++ source files with the given `tidy_checks`.

However since clang-tidy warnings and its runtime cost might
not be wanted by all people, the default is to ignore the
`tidy:true` property unless the environment variable
`ALLOW_LOCAL_TIDY_TRUE` is set to true or 1.
To run clang-tidy on all modules that should be tested with clang-tidy,
`ALLOW_LOCAL_TIDY_TRUE` or `WITH_TIDY` should be set to true or 1.

Note that `clang-analyzer-security*` is included in `tidy_checks`
but not all `clang-analyzer-*` checks. Check `cert-err34-c` is
disabled, although `cert-*` is selected.

Some modules might want to disable clang-tidy even when
environment variable `WITH_TIDY=1` is set.
Examples can be found in
[system/netd/tests/Android.bp](https://android.googlesource.com/platform/system/netd/+/refs/heads/main/tests/Android.bp)
```
cc_test {
    name: "netd_integration_test",
    // snipped
    defaults: ["netd_defaults"],
    tidy: false,  // cuts test build time by almost 1 minute
```
and in
[bionic/tests/Android.bp](https://android.googlesource.com/platform/bionic/+/refs/heads/main/tests/Android.bp).
```
cc_test_library {
    name: "fortify_disabled_for_tidy",
    // snipped
    srcs: ["clang_fortify_tests.cpp"],
    tidy: false,
}
```

Note that `tidy:false` always disables clang-tidy, no matter
`ALLOW_LOCAL_TIDY_TRUE` is set or not.

### `tidy_checks_as_errors`

The global tidy checks are enabled as warnings.
If a C/C++ module wants to be free of certain clang-tidy warnings,
it can chose those checks to be treated as errors.
For example
[system/core/libsysutils/Android.bp](https://android.googlesource.com/platform/system/core/+/refs/heads/main/libsysutils/Android.bp)
has enabled clang-tidy explicitly, selected its own tidy checks,
and set three groups of tidy checks as errors:
```
cc_library {
    name: "libsysutils",
    // snipped
    tidy: true,
    tidy_checks: [
        "-*",
        "cert-*",
        "clang-analyzer-security*",
        "android-*",
    ],
    tidy_checks_as_errors: [
        "cert-*",
        "clang-analyzer-security*",
        "android-*",
    ],
    // snipped
}
```

### `tidy_flags` and `tidy_disabled_srcs`

Extra clang-tidy flags can be passed with the `tidy_flags` property.

Some Android modules use the `tidy_flags` to pass "-warnings-as-errors="
to clang-tidy. This usage should now be replaced with the
`tidy_checks_as_errors` property.

Some other tidy flags examples are `-format-style=` and `-header-filter=`
For example, in
[art/odrefresh/Android.bp](https://android.googlesource.com/platform/art/+/refs/heads/main/odrefresh/Android.bp),
we found
```
cc_defaults {
    name: "odrefresh-defaults",
    srcs: [
        "odrefresh.cc",
        "odr_common.cc",
        "odr_compilation_log.cc",
        "odr_fs_utils.cc",
        "odr_metrics.cc",
        "odr_metrics_record.cc",
    ],
    // snipped
    generated_sources: [
        "apex-info-list-tinyxml",
        "art-apex-cache-info",
        "art-odrefresh-operator-srcs",
    ],
    // snipped
    tidy: true,
    tidy_disabled_srcs: [":art-apex-cache-info"],
    tidy_flags: [
        "-format-style=file",
        "-header-filter=(art/odrefresh/|system/apex/)",
    ],
}
```
That means all modules with the `odrefresh-defaults` will
have clang-tidy enabled, but not for generated source
files in `art-apex-cache-info`.
The clang-tidy is called with extra flags to specify the
format-style and header-filter.

Note that the globally set default for header-filter is to
include only the module directory. So, the default clang-tidy
warnings for `art/odrefresh` modules will include source files
under that directory. Now `odrefresh-defaults` is interested
in seeing warnings from both `art/odrefresh/` and `system/apex/`
and it redefines `-header-filter` in its `tidy_flags`.


## Phony tidy-* targets

### The tidy-*directory* targets

Setting `WITH_TIDY=1` is easy to enable clang-tidy globally for any build.
However, it adds extra compilation time.

For developers focusing on just one directory, they only want to compile
their files with clang-tidy and wish to build other Android components as
fast as possible. Changing the `WITH_TIDY=1` variable setting is also expensive
since the build.ninja file will be regenerated due to any such variable change.

To manually select only some directories or modules to compile with clang-tidy,
do not set the `WITH_TIDY=1` variable, but use the special `tidy-<directory>`
phony target. For example, a person working on `system/libbase` can build
Android quickly with
```
unset WITH_TIDY # Optional, not if you haven't set WITH_TIDY
make droid tidy-system-libbase
```

For any directory `d1/d2/d3`, a phony target tidy-d1-d2-d3 is generated
if there is any C/C++ source file under `d1/d2/d3`.

Note that with `make droid tidy-system-libbase`, some C/C++ files
that are not needed by the `droid` target will be passed to clang-tidy
if they are under `system/libbase`. This is like a `checkbuild`
under `system/libbase` to include all modules, but only C/C++
files of those modules are compiled with clang-tidy.

### The tidy-soong target

A special `tidy-soong` target is defined to include all C/C++
source files in *all* directories. This phony target is sometimes
used to test if all source files compile with a new clang-tidy release.

### The tidy-*_subset targets

A *subset* of each tidy-* phony target is defined to reduce test time.
Since any Android module, a C/C++ library or binary, can be built
for many different *variants*, one C/C++ source file is usually
compiled multiple times with different compilation flags.
Many of such *variant* flags have little or no effect on clang-tidy
checks. To reduce clang-tidy check time, a *subset* target like
`tidy-soong_subset` or `tidy-system-libbase_subset` is generated
to include only a subset, the first variant, of each module in
the directory.

Hence, for C/C++ source code quality, instead of a long
"make checkbuild", we can use "make tidy-soong_subset".


## Limit clang-tidy runtime

Some Android modules have large files that take a long time to compile
with clang-tidy, with or without the clang-analyzer checks.
To limit clang-tidy time, an environment variable can be set as
```base
WITH_TIDY=1 TIDY_TIMEOUT=90 make
```
This 90-second limit is actually the default time limit
in several Android continuous builds where `WITH_TIDY=1` and
`CLANG_ANALYZER_CHECKS=1` are set.

Similar to `tidy_disabled_srcs` a `tidy_timeout_srcs` list
can be used to include all source files that took too much time to compile
with clang-tidy. Files listed in `tidy_timeout_srcs` will not
be compiled by clang-tidy when `TIDY_TIMEOUT` is defined.
This can save global build time, when it is necessary to set some
time limit globally to finish in an acceptable time.
For developers who want to find all clang-tidy warnings and
are willing to spend more time on all files in a project,
they should not define `TIDY_TIMEOUT` and build only the wanted project directories.

## Capabilities for Android.bp and Android.mk

Some of the previously mentioned features are defined only
for modules in Android.bp files, not for Android.mk modules yet.

* The global `WITH_TIDY=1` variable will enable clang-tidy for all C/C++
  modules in Android.bp or Android.mk files.

* The global `TIDY_TIMEOUT` variable is recognized by Android prebuilt
  clang-tidy, so it should work for any clang-tidy invocation.

* The clang-tidy module level properties are defined for Android.bp modules.
  For Android.mk modules, old `LOCAL_TIDY`, `LOCAL_TIDY_CHECKS`,
  `LOCAL_TIDY_FLAGS` work similarly, but it would be better to convert
  those modules to use Android.bp files.

* The `tidy-*` phony targets are only generated for Android.bp modules.
