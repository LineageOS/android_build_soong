## Native Code Coverage for Android

## Scope

These instructions are for Android developers to collect and inspect code
coverage for C++ and Rust code on the Android platform.

## Building with Native Code Coverage Instrumentation

Identify the paths where native code-coverage instrumentation should be enabled
and set up the following environment variables.

```
    export CLANG_COVERAGE=true
    export NATIVE_COVERAGE_PATHS="<paths-to-instrument-for-coverage>"
```

`NATIVE_COVERAGE_PATHS` should be a list of paths. Any Android.bp module defined
under these paths is instrumented for code-coverage. E.g:

```
export NATIVE_COVERAGE_PATHS="external/libcxx system/core/adb"
```

### Additional Notes

-   Native Code coverage is not supported for host modules or `Android.mk`
    modules.
-   `NATIVE_COVERAGE_PATHS="*"` enables coverage instrumentation for all paths.
-   Set `native_coverage: false` blueprint property to always disable code
    coverage instrumentation for a module. This is useful if this module has
    issues when building or running with coverage.
-   `NATIVE_COVERAGE_EXCLUDE_PATHS` can be set to exclude subdirs under
    `NATIVE_COVERAGE_PATHS` from coverage instrumentation. E.g.
    `NATIVE_COVERAGE_PATHS=frameworks/native
    NATIVE_COVERAGE_PATHS=frameworks/native/vulkan` will instrument all native
    code under `frameworks/native` except`frameworks/native/vulkan`.

## Running Tests

### Collecting Profiles

When an instrumented program is run, the profiles are stored to the path and
name specified in the `LLVM_PROFILE_FILE` environment variable. On Android
coverage builds it is set to `/data/misc/trace/clang-%p-%20m.profraw`.

*   `%`p is replaced by the pid of the process
*   `%m` by the hash of the library/binary
*   The `20` in`%20m` creates a pool of 20 profraw files and "online" profile
    merging is used to merge coverage to profiles onto this pool.

Reference:`LLVM_PROFILE_FILE` can include additional specifiers as described
[here](https://clang.llvm.org/docs/SourceBasedCodeCoverage.html#running-the-instrumented-program).

For this and following steps, use the `acov-llvm.py` script:
`$ANDROID_BUILD_TOP/development/scripts/acov-llvm.py`.

There may be profiles in `/data/misc/trace` collected before the test is run.
Clear this data before running the test.

```
    # Clear any coverage that's already written to /data/misc/trace
    # and reset coverage for all daemons.
    <host>$ acov-llvm.py clean-device

    # Run the test.  The exact command depends on the nature of the test.
    <device>$ /data/local/tmp/$MY_TEST
```

For tests that exercise a daemon/service running in another process, write out
the coverage for those processes as well.

```
    # Flush coverage of all daemons/processes running on the device.
    <host>$ acov-llvm.py flush

    # Flush coverage for a particular daemon, say adbd.
    <host>$ acov-llvm.py flush adbd
```

## Viewing Coverage Data (acov-llvm.py)

To post-process and view coverage information we use the `acov-llvm.py report`
command. It invokes two LLVM utilities `llvm-profdata` and `llvm-cov`. An
advanced user can manually invoke these utilities for fine-grained control. This
is discussed [below](#viewing-coverage-data-manual).

To generate coverage report need the following parameters. These are dependent
on the test/module:

1.  One or more binaries and shared libraries from which coverage was collected.
    E.g.:

    1.  ART mainline module contains a few libraries such as `libart.so`,
        `libart-compiler.so`.
    2.  Bionic tests exercise code in `libc.so` and `libm.so`.

    We need the *unstripped* copies of these binaries. Source information
    included in the debuginfo is used to process the coverage data.

2.  One or more source directories under `$ANDROID_BUILD_TOP` for which coverage
    needs to be reported.

Invoke the report subcommand of acov-llvm.py to produce a html coverage summary:

```
    $ acov-llvm.py report \
        -s <one-or-more-source-paths-in-$ANDROID_BUILD_TOP \
        -b <one-or-more-(unstripped)-binaries-in-$OUT>
```

E.g.:

```
    $ acov-llvm.py report \
        -s bionic \
        -b \
        $OUT/symbols/apex/com.android.runtime/lib/bionic/libc.so \
        $OUT/symbols/apex/com.android.runtime/lib/bionic/libm.so
```

The script will produce a report in a temporary directory under
`$ANDROID_BUILD_TOP`. It'll produce a log as below:

```
    generating coverage report in covreport-xxxxxx
```

A html report would be generated under `covreport-xxxxxx/html`.

## Viewing Coverage Data (manual)

`acov-llvm.py report` does a few operations under the hood which we can also
manually invoke for flexibility.

### Post-processing Coverage Files

Fetch coverage files from the device and post-process them to a `.profdata` file
as follows:

```
    # Fetch the coverage data from the device.
    <host>$ cd coverage_data
    <host>$ adb pull /data/misc/trace/ $TRACE_DIR_HOST

    # Convert from .profraw format to the .profdata format.
    <host>$ llvm-profdata merge --output=$MY_TEST.profdata \
    $TRACE_DIR_HOST/clang-*.profraw
```

For added specificity, restrict the above command to just the <PID>s of the
daemon or test processes of interest.

```
    <host>$ llvm-profdata merge --output=$MY_TEST.profdata \
    $MY_TEST.profraw \
    trace/clang-<pid1>.profraw trace/clang-<pid2>.profraw ...
```

### Generating Coverage report

Documentation on Clang source-instrumentation-based coverage is available
[here](https://clang.llvm.org/docs/SourceBasedCodeCoverage.html#creating-coverage-reports).
The `llvm-cov` utility is used to show coverage from a `.profdata` file. The
documentation for commonly used `llvm-cov` command-line arguments is available
[here](https://llvm.org/docs/CommandGuide/llvm-cov.html#llvm-cov-report). (Try
`llvm-cov show --help` for a complete list).

#### `show` subcommand

The `show` command displays the function and line coverage for each source file
in the binary.

```
    <host>$ llvm-cov show \
        --show-region-summary=false
        --format=html --output-dir=coverage-html \
        --instr-profile=$MY_TEST.profdata \
        $MY_BIN \
```

*   In the above command, `$MY_BIN` should be the unstripped binary (i.e. with
    debuginfo) since `llvm-cov` reads some debuginfo to process the coverage
    data.

    E.g.:

    ~~~
    ```
    <host>$ llvm-cov report \
        --instr-profile=adbd.profdata \
        $LOCATION_OF_UNSTRIPPED_ADBD/adbd \
        --show-region-summary=false
    ```
    ~~~

*   The `-ignore-filename-regex=<regex>` option can be used to ignore files that
    are not of interest. E.g: `-ignore-filename-regex="external/*"`

*   Use the `--object=<BIN>` argument to specify additional binaries and shared
    libraries whose coverage is included in this profdata. See the earlier
    [section](#viewing-coverage-data-acov-llvm-py) for examples where more than
    one binary may need to be used.

    E.g., the following command is used for `bionic-unit-tests`, which tests
    both `libc.so` and `libm.so`:

    ~~~
    ```
    <host>$ llvm-cov report \
        --instr-profile=bionic.profdata \
        $OUT/.../libc.so \
        --object=$OUT/.../libm.so
    ```
    ~~~

*   `llvm-cov` also takes positional SOURCES argument to consider/display only
    particular paths of interest. E.g:

    ~~~
    ```
    <host>$ llvm-cov report \
        --instr-profile=adbd.profdata \
        $LOCATION_OF_ADBD/adbd \
        --show-region-summary=false \
        /proc/self/cwd/system/core/adb
    ```
    ~~~

Note that the paths for the sources need to be prepended with
'`/proc/self/cwd/`'. This is because Android C/C++ compilations run with
`PWD=/proc/self/cwd` and consequently the source names are recorded with that
prefix. Alternatively, the
[`--path-equivalence`](https://llvm.org/docs/CommandGuide/llvm-cov.html#cmdoption-llvm-cov-show-path-equivalence)
option to `llvm-cov` can be used.

#### `report` subcommand

The [`report`](https://llvm.org/docs/CommandGuide/llvm-cov.html#llvm-cov-report)
subcommand summarizes the percentage of covered lines to the console. It takes
options similar to the `show` subcommand.
