## Dexpreopt implementation

### Introduction

All dexpreopted Java code falls into three categories:

- bootclasspath
- system server
- apps and libraries

Dexpreopt implementation for bootclasspath libraries (boot images) is located in
[soong/java] (see e.g. [soong/java/dexpreopt_bootjars.go]), and install rules
are in [make/core/dex_preopt.mk].

Dexpreopt implementation for system server, libraries and apps is located in
[soong/dexpreopt]. For the rest of this section we focus primarily on it (and
not boot images).

Dexpeopt implementation is split across the Soong part and the Make part. The
core logic is in Soong, and Make only generates configs and scripts to pass
information to Soong.

### Global and module dexpreopt.config

The build system generates a global JSON dexpreopt config that is populated from
product variables. This is static configuration that is passed to both Soong and
Make. The `$OUT/soong/dexpreopt.config` file is generated in
[make/core/dex_preopt_config.mk]. Soong reads it in [soong/dexpreopt/config.go]
and makes a device-specific copy (this is needed to ensure incremental build
correctness). The global config contains lists of bootclasspath jars, system
server jars, dex2oat options, global switches that enable and disable parts of
dexpreopt and so on.

The build system also generates a module config for each dexpreopted package. It
contains package-specific configuration that is derived from the global
configuration and Android.bp or Android.mk module for the package.

Module configs for Make packages are generated in
[make/core/dex_preopt_odex_install.mk]; they are materialized as per-package
JSON dexpreopt.config files.

Module configs in Soong are not materialized as dexpreopt.config files and exist
as Go structures in memory, unless it is necessary to materialize them as a file
for dependent Make packages or for post-dexpreopting. Module configs are defined
in [soong/dexpreopt/config.go].

### Dexpreopt in Soong

The Soong implementation of dexpreopt consists roughly of the following steps:

- Read global dexpreopt config passed from Make ([soong/dexpreopt/config.go]).

- Construct a static boot image config ([soong/java/dexpreopt_config.go]).

- During dependency mutator pass, for each suitable module:
    - add uses-library dependencies (e.g. for apps: [soong/java/app.go:deps])

- During rule generation pass, for each suitable module:
    - compute transitive uses-library dependency closure
      ([soong/java/java.go:addCLCFromDep])

    - construct CLC from the dependency closure
      ([soong/dexpreopt/class_loader_context.go])

    - construct module config with CLC, boot image locations, etc.
      ([soong/java/dexpreopt.go])

    - generate build rules to verify build-time CLC against the manifest (e.g.
      for apps: [soong/java/app.go:verifyUsesLibraries])

    - generate dexpreopt build rule ([soong/dexpreopt/dexpreopt.go])

- At the end of rule generation pass:
    - generate build rules for boot images ([soong/java/dexpreopt_bootjars.go],
      [soong/java/bootclasspath_fragment.go] and
      [soong/java/platform_bootclasspath.go])

### Dexpreopt in Make - dexpreopt_gen

In order to reuse the same dexpreopt implementation for both Soong and Make
packages, part of Soong is compiled into a standalone binary dexpreopt_gen. It
runs during the Ninja stage of the build and generates shell scripts with
dexpreopt build rules for Make packages, and then executes them.

This setup causes many inconveniences. To name a few:

- Errors in the build rules are only revealed at the late stage of the build.

- These rules are not tested by the presubmit builds that run `m nothing` on
  many build targets/products.

- It is impossible to find dexpreopt build rules in the generated Ninja files.

However all these issues are a lesser evil compared to having a duplicate
dexpreopt implementation in Make. Also note that it would be problematic to
reimplement the logic in Make anyway, because Android.mk modules are not
processed in the order of uses-library dependencies and propagating dependency
information from one module to another would require a similar workaround with
a script.

Dexpreopt for Make packages involves a few steps:

- At Soong phase (during `m nothing`), see dexpreopt_gen:
    - generate build rules for dexpreopt_gen binary

- At Make/Kati phase (during `m nothing`), see
  [make/core/dex_preopt_odex_install.mk]:
    - generate build rules for module dexpreopt.config

    - generate build rules for merging dependency dexpreopt.config files (see
      [make/core/dex_preopt_config_merger.py])

    - generate build rules for dexpreopt_gen invocation

    - generate build rules for executing dexpreopt.sh scripts

- At Ninja phase (during `m`):
    - generate dexpreopt.config files

    - execute dexpreopt_gen rules (generate dexpreopt.sh scripts)

    - execute dexpreopt.sh scripts (this runs the actual dexpreopt)

The Make/Kati phase adds all the necessary dependencies that trigger
dexpreopt_gen and dexpreopt.sh rules. The real dexpreopt command (dex2oat
invocation that will be executed to AOT-compile a package) is in the
dexpreopt.sh script, which is generated close to the end of the build.

### Indirect build rules

The process described above for Make packages involves "indirect build rules",
i.e. build rules that are generated not at the time when the build system is
created (which is a small step at the very beginning of the build triggered with
`m nothing`), but at the time when the actual build is done (`m` phase).

Some build systems, such as Make, allow modifications of the build graph during
the build. Other build systems, such as Soong, have a clear separation into the
first "generation phase" (this is when build rules are created) and the second
"build phase" (this is when the build rules are executed), and they do not allow
modifications of the dependency graph during the second phase. The Soong
approach is better from performance standpoint, because with the Make approach
there are no guarantees regarding the time of the build --- recursive build
graph modfications continue until fixpoint. However the Soong approach is also
more restictive, as it can only generate build rules from the information that
is passed to the build system via global configuration, Android.bp files or
encoded in the Go code. Any other information (such as the contents of the Java
manifest files) are not accessible and cannot be used to generate build rules.

Hence the need for the "indirect build rules": during the generation phase only
stubs of the build rules are generated, and the real rules are generated by the
stub rules during the build phase (and executed immediately). Note that the
build system still has to add all the necessary dependencies during the
generation phase, because it will not be possible to change build order during
the build phase.

Indirect buils rules are used in a couple of places in dexpreopt:

- [soong/scripts/manifest_check.py]: first to extract targetSdkVersion from the
  manifest, and later to extract `<uses-library/>` tags from the manifest and
  compare them to the uses-library list known to the build system

- [soong/scripts/construct_context.py]: to trim compatibility libraries in CLC

- [make/core/dex_preopt_config_merger.py]: to merge information from
  dexpreopt.config files for uses-library dependencies into the dependent's
  dexpreopt.config file (mostly the CLC)

- autogenerated dexpreopt.sh scripts: to call dexpreopt_gen

### Consistency check - manifest_check.py

Because the information from the manifests has to be duplicated in the
Android.bp/Android.mk files, there is a danger that it may get out of sync. To
guard against that, the build system generates a rule that verifies
uses-libraries: checks the metadata in the build files against the contents of a
manifest. The manifest can be available as a source file, or as part of a
prebuilt APK.

The check is implemented in [soong/scripts/manifest_check.py].

It is possible to turn off the check globally for a product by setting
`PRODUCT_BROKEN_VERIFY_USES_LIBRARIES := true` in a product makefile, or for a
particular build by setting `RELAX_USES_LIBRARY_CHECK=true`.

### Compatibility libraries - construct_context.py

Compatibility libraries are libraries that didn’t exist prior to a certain SDK
version (say, `N`), but classes in them were in the bootclasspath jars, etc.,
and in version `N` they have been separated into a standalone uses-library.
Compatibility libraries should only be in the CLC of an app if its
`targetSdkVersion` in the manifest is less than `N`.

Currently compatibility libraries only affect apps (but not other libraries).

The build system cannot see `targetSdkVersion` of an app at the time it
generates dexpreopt build rules, so it doesn't know whether to add compatibility
libaries to CLC or not. As a workaround, the build system includes all
compatibility libraries regardless of the app version, and appends some extra
logic to the dexpreopt rule that will extract `targetSdkVersion` from the
manifest and filter CLC based on that version during Ninja stage of the build,
immediately before executing the dexpreopt command (see the
soong/scripts/construct_context.py script).

As of the time of writing (January 2022), there are the following compatibility
libraries:

- org.apache.http.legacy (SDK 28)
- android.hidl.base-V1.0-java (SDK 29)
- android.hidl.manager-V1.0-java (SDK 29)
- android.test.base (SDK 30)
- android.test.mock (SDK 30)

### Manifest fixer

Sometimes uses-library tags are missing from the source manifest of a
library/app. This may happen for example if one of the transitive dependencies
of the library/app starts using another uses-library, and the library/app's
manifest isn't updated to include it.

Soong can compute some of the missing uses-library tags for a given library/app
automatically as SDK libraries in the transitive dependency closure of the
library/app. The closure is needed because a library/app may depend on a static
library that may in turn depend on an SDK library (possibly transitively via
another library).

Not all uses-library tags can be computed in this way, because some of the
uses-library dependencies are not SDK libraries, or they are not reachable via
transitive dependency closure. But when possible, allowing Soong to calculate
the manifest entries is less prone to errors and simplifies maintenance. For
example, consider a situation when many apps use some static library that adds a
new uses-library dependency -- all the apps will have to be updated. That is
difficult to maintain.

There is also a manifest merger, because sometimes the final manifest of an app
is merged from a few dependency manifests, so the final manifest installed on
devices contains a superset of uses-library tags of the source manifest of the
app.


[make/core/dex_preopt.mk]: https://cs.android.com/android/platform/superproject/+/main:build/make/core/dex_preopt.mk
[make/core/dex_preopt_config.mk]: https://cs.android.com/android/platform/superproject/+/main:build/make/core/dex_preopt_config.mk
[make/core/dex_preopt_config_merger.py]: https://cs.android.com/android/platform/superproject/+/main:build/make/core/dex_preopt_config_merger.py
[make/core/dex_preopt_odex_install.mk]: https://cs.android.com/android/platform/superproject/+/main:build/make/core/dex_preopt_odex_install.mk
[soong/dexpreopt]: https://cs.android.com/android/platform/superproject/+/main:build/soong/dexpreopt
[soong/dexpreopt/class_loader_context.go]: https://cs.android.com/android/platform/superproject/+/main:build/soong/dexpreopt/class_loader_context.go
[soong/dexpreopt/config.go]: https://cs.android.com/android/platform/superproject/+/main:build/soong/dexpreopt/config.go
[soong/dexpreopt/dexpreopt.go]: https://cs.android.com/android/platform/superproject/+/main:build/soong/dexpreopt/dexpreopt.go
[soong/java]: https://cs.android.com/android/platform/superproject/+/main:build/soong/java
[soong/java/app.go:deps]: https://cs.android.com/android/platform/superproject/+/main:build/soong/java/app.go?q=%22func%20\(u%20*usesLibrary\)%20deps%22
[soong/java/app.go:verifyUsesLibraries]: https://cs.android.com/android/platform/superproject/+/main:build/soong/java/app.go?q=%22func%20\(u%20*usesLibrary\)%20verifyUsesLibraries%22
[soong/java/bootclasspath_fragment.go]: https://cs.android.com/android/platform/superproject/+/main:build/soong/java/bootclasspath_fragment.go
[soong/java/dexpreopt.go]: https://cs.android.com/android/platform/superproject/+/main:build/soong/java/dexpreopt.go
[soong/java/dexpreopt_bootjars.go]: https://cs.android.com/android/platform/superproject/+/main:build/soong/java/dexpreopt_bootjars.go
[soong/java/dexpreopt_config.go]: https://cs.android.com/android/platform/superproject/+/main:build/soong/java/dexpreopt_config.go
[soong/java/java.go:addCLCFromDep]: https://cs.android.com/android/platform/superproject/+/main:build/soong/java/java.go?q=%22func%20addCLCfromDep%22
[soong/java/platform_bootclasspath.go]: https://cs.android.com/android/platform/superproject/+/main:build/soong/java/platform_bootclasspath.go
[soong/scripts/construct_context.py]: https://cs.android.com/android/platform/superproject/+/main:build/soong/scripts/construct_context.py
[soong/scripts/manifest_check.py]: https://cs.android.com/android/platform/superproject/+/main:build/soong/scripts/manifest_check.py
