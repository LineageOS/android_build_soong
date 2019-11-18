# Soong

Soong is the replacement for the old Android make-based build system.  It
replaces Android.mk files with Android.bp files, which are JSON-like simple
declarative descriptions of modules to build.

See [Simple Build
Configuration](https://source.android.com/compatibility/tests/development/blueprints)
on source.android.com to read how Soong is configured for testing.

## Android.bp file format

By design, Android.bp files are very simple.  There are no conditionals or
control flow statements - any complexity is handled in build logic written in
Go.  The syntax and semantics of Android.bp files are intentionally similar
to [Bazel BUILD files](https://www.bazel.io/versions/master/docs/be/overview.html)
when possible.

### Modules

A module in an Android.bp file starts with a module type, followed by a set of
properties in `name: value,` format:

```
cc_binary {
    name: "gzip",
    srcs: ["src/test/minigzip.c"],
    shared_libs: ["libz"],
    stl: "none",
}
```

Every module must have a `name` property, and the value must be unique across
all Android.bp files.

For a list of valid module types and their properties see
[$OUT_DIR/soong/docs/soong_build.html](https://ci.android.com/builds/latest/branches/aosp-build-tools/targets/linux/view/soong_build.html).

### Globs

Properties that take a list of files can also take glob patterns.  Glob
patterns can contain the normal Unix wildcard `*`, for example "*.java". Glob
patterns can also contain a single `**` wildcard as a path element, which will
match zero or more path elements.  For example, `java/**/*.java` will match
`java/Main.java` and `java/com/android/Main.java`.

### Variables

An Android.bp file may contain top-level variable assignments:
```
gzip_srcs = ["src/test/minigzip.c"],

cc_binary {
    name: "gzip",
    srcs: gzip_srcs,
    shared_libs: ["libz"],
    stl: "none",
}
```

Variables are scoped to the remainder of the file they are declared in, as well
as any child blueprint files.  Variables are immutable with one exception - they
can be appended to with a += assignment, but only before they have been
referenced.

### Comments
Android.bp files can contain C-style multiline `/* */` and C++ style single-line
`//` comments.

### Types

Variables and properties are strongly typed, variables dynamically based on the
first assignment, and properties statically by the module type.  The supported
types are:
* Bool (`true` or `false`)
* Integers (`int`)
* Strings (`"string"`)
* Lists of strings (`["string1", "string2"]`)
* Maps (`{key1: "value1", key2: ["value2"]}`)

Maps may values of any type, including nested maps.  Lists and maps may have
trailing commas after the last value.

Strings can contain double quotes using `\"`, for example `"cat \"a b\""`.

### Operators

Strings, lists of strings, and maps can be appended using the `+` operator.
Integers can be summed up using the `+` operator. Appending a map produces the
union of keys in both maps, appending the values of any keys that are present
in both maps.

### Defaults modules

A defaults module can be used to repeat the same properties in multiple modules.
For example:

```
cc_defaults {
    name: "gzip_defaults",
    shared_libs: ["libz"],
    stl: "none",
}

cc_binary {
    name: "gzip",
    defaults: ["gzip_defaults"],
    srcs: ["src/test/minigzip.c"],
}
```

### Packages

The build is organized into packages where each package is a collection of related files and a
specification of the dependencies among them in the form of modules.

A package is defined as a directory containing a file named `Android.bp`, residing beneath the
top-level directory in the build and its name is its path relative to the top-level directory. A
package includes all files in its directory, plus all subdirectories beneath it, except those which
themselves contain an `Android.bp` file.

The modules in a package's `Android.bp` and included files are part of the module.

For example, in the following directory tree (where `.../android/` is the top-level Android
directory) there are two packages, `my/app`, and the subpackage `my/app/tests`. Note that
`my/app/data` is not a package, but a directory belonging to package `my/app`.

    .../android/my/app/Android.bp
    .../android/my/app/app.cc
    .../android/my/app/data/input.txt
    .../android/my/app/tests/Android.bp
    .../android/my/app/tests/test.cc

This is based on the Bazel package concept.

The `package` module type allows information to be specified about a package. Only a single
`package` module can be specified per package and in the case where there are multiple `.bp` files
in the same package directory it is highly recommended that the `package` module (if required) is
specified in the `Android.bp` file.

Unlike most module type `package` does not have a `name` property. Instead the name is set to the
name of the package, e.g. if the package is in `top/intermediate/package` then the package name is
`//top/intermediate/package`.

E.g. The following will set the default visibility for all the modules defined in the package and
any subpackages that do not set their own default visibility (irrespective of whether they are in
the same `.bp` file as the `package` module) to be visible to all the subpackages by default.

```
package {
    default_visibility: [":__subpackages"]
}
```

### Name resolution

Soong provides the ability for modules in different directories to specify
the same name, as long as each module is declared within a separate namespace.
A namespace can be declared like this:

```
soong_namespace {
    imports: ["path/to/otherNamespace1", "path/to/otherNamespace2"],
}
```

Each Soong module is assigned a namespace based on its location in the tree.
Each Soong module is considered to be in the namespace defined by the
soong_namespace found in an Android.bp in the current directory or closest
ancestor directory, unless no such soong_namespace module is found, in which
case the module is considered to be in the implicit root namespace.

When Soong attempts to resolve dependency D declared my module M in namespace
N which imports namespaces I1, I2, I3..., then if D is a fully-qualified name
of the form "//namespace:module", only the specified namespace will be searched
for the specified module name. Otherwise, Soong will first look for a module
named D declared in namespace N. If that module does not exist, Soong will look
for a module named D in namespaces I1, I2, I3... Lastly, Soong will look in the
root namespace.

Until we have fully converted from Make to Soong, it will be necessary for the
Make product config to specify a value of PRODUCT_SOONG_NAMESPACES. Its value
should be a space-separated list of namespaces that Soong export to Make to be
built by the `m` command. After we have fully converted from Make to Soong, the
details of enabling namespaces could potentially change.

### Visibility

The `visibility` property on a module controls whether the module can be
used by other packages. Modules are always visible to other modules declared
in the same package. This is based on the Bazel visibility mechanism.

If specified the `visibility` property must contain at least one rule.

Each rule in the property must be in one of the following forms:
* `["//visibility:public"]`: Anyone can use this module.
* `["//visibility:private"]`: Only rules in the module's package (not its
subpackages) can use this module.
* `["//some/package:__pkg__", "//other/package:__pkg__"]`: Only modules in
`some/package` and `other/package` (defined in `some/package/*.bp` and
`other/package/*.bp`) have access to this module. Note that sub-packages do not
have access to the rule; for example, `//some/package/foo:bar` or
`//other/package/testing:bla` wouldn't have access. `__pkg__` is a special
module and must be used verbatim. It represents all of the modules in the
package.
* `["//project:__subpackages__", "//other:__subpackages__"]`: Only modules in
packages `project` or `other` or in one of their sub-packages have access to
this module. For example, `//project:rule`, `//project/library:lib` or
`//other/testing/internal:munge` are allowed to depend on this rule (but not
`//independent:evil`)
* `["//project"]`: This is shorthand for `["//project:__pkg__"]`
* `[":__subpackages__"]`: This is shorthand for `["//project:__subpackages__"]`
where `//project` is the module's package, e.g. using `[":__subpackages__"]` in
`packages/apps/Settings/Android.bp` is equivalent to
`//packages/apps/Settings:__subpackages__`.
* `["//visibility:legacy_public"]`: The default visibility, behaves as
`//visibility:public` for now. It is an error if it is used in a module.

The visibility rules of `//visibility:public` and `//visibility:private` cannot
be combined with any other visibility specifications, except
`//visibility:public` is allowed to override visibility specifications imported
through the `defaults` property.

Packages outside `vendor/` cannot make themselves visible to specific packages
in `vendor/`, e.g. a module in `libcore` cannot declare that it is visible to
say `vendor/google`, instead it must make itself visible to all packages within
`vendor/` using `//vendor:__subpackages__`.

If a module does not specify the `visibility` property then it uses the
`default_visibility` property of the `package` module in the module's package.

If the `default_visibility` property is not set for the module's package then
it will use the `default_visibility` of its closest ancestor package for which
a `default_visibility` property is specified.

If no `default_visibility` property can be found then the module uses the
global default of `//visibility:legacy_public`.

The `visibility` property has no effect on a defaults module although it does
apply to any non-defaults module that uses it. To set the visibility of a
defaults module, use the `defaults_visibility` property on the defaults module;
not to be confused with the `default_visibility` property on the package module.

Once the build has been completely switched over to soong it is possible that a
global refactoring will be done to change this to `//visibility:private` at
which point all packages that do not currently specify a `default_visibility`
property will be updated to have
`default_visibility = [//visibility:legacy_public]` added. It will then be the
owner's responsibility to replace that with a more appropriate visibility.

### Formatter

Soong includes a canonical formatter for blueprint files, similar to
[gofmt](https://golang.org/cmd/gofmt/).  To recursively reformat all Android.bp files
in the current directory:
```
bpfmt -w .
```

The canonical format includes 4 space indents, newlines after every element of a
multi-element list, and always includes a trailing comma in lists and maps.

### Convert Android.mk files

Soong includes a tool perform a first pass at converting Android.mk files
to Android.bp files:

```
androidmk Android.mk > Android.bp
```

The tool converts variables, modules, comments, and some conditionals, but any
custom Makefile rules, complex conditionals or extra includes must be converted
by hand.

#### Differences between Android.mk and Android.bp

* Android.mk files often have multiple modules with the same name (for example
for static and shared version of a library, or for host and device versions).
Android.bp files require unique names for every module, but a single module can
be built in multiple variants, for example by adding `host_supported: true`.
The androidmk converter will produce multiple conflicting modules, which must
be resolved by hand to a single module with any differences inside
`target: { android: { }, host: { } }` blocks.

## Build logic

The build logic is written in Go using the
[blueprint](http://godoc.org/github.com/google/blueprint) framework.  Build
logic receives module definitions parsed into Go structures using reflection
and produces build rules.  The build rules are collected by blueprint and
written to a [ninja](http://ninja-build.org) build file.

## Other documentation

* [Best Practices](docs/best_practices.md)
* [Build Performance](docs/perf.md)
* [Generating CLion Projects](docs/clion.md)
* [Generating YouCompleteMe/VSCode compile\_commands.json file](docs/compdb.md)
* Make-specific documentation: [build/make/README.md](https://android.googlesource.com/platform/build/+/master/README.md)

## FAQ

### How do I write conditionals?

Soong deliberately does not support conditionals in Android.bp files.  We
suggest removing most conditionals from the build.  See
[Best Practices](docs/best_practices.md#removing-conditionals) for some
examples on how to remove conditionals.

In cases where build time conditionals are unavoidable, complexity in build
rules that would require conditionals are handled in Go through Soong plugins.
This allows Go language features to be used for better readability and
testability, and implicit dependencies introduced by conditionals can be
tracked.  Most conditionals supported natively by Soong are converted to a map
property.  When building the module one of the properties in the map will be
selected, and its values appended to the property with the same name at the
top level of the module.

For example, to support architecture specific files:
```
cc_library {
    ...
    srcs: ["generic.cpp"],
    arch: {
        arm: {
            srcs: ["arm.cpp"],
        },
        x86: {
            srcs: ["x86.cpp"],
        },
    },
}
```

When building the module for arm the `generic.cpp` and `arm.cpp` sources will
be built.  When building for x86 the `generic.cpp` and 'x86.cpp' sources will
be built.

## Developing for Soong

To load Soong code in a Go-aware IDE, create a directory outside your android tree and then:
```bash
apt install bindfs
export GOPATH=<path to the directory you created>
build/soong/scripts/setup_go_workspace_for_soong.sh
```

This will bind mount the Soong source directories into the directory in the layout expected by
the IDE.

### Running Soong in a debugger

To run the soong_build process in a debugger, install `dlv` and then start the build with
`SOONG_DELVE=<listen addr>` in the environment.
For example:
```bash
SOONG_DELVE=:1234 m nothing
```
and then in another terminal:
```
dlv connect :1234
```

If you see an error:
```
Could not attach to pid 593: this could be caused by a kernel
security setting, try writing "0" to /proc/sys/kernel/yama/ptrace_scope
```
you can temporarily disable
[Yama's ptrace protection](https://www.kernel.org/doc/Documentation/security/Yama.txt)
using:
```bash
sudo sysctl -w kernel.yama.ptrace_scope=0
```

## Contact

Email android-building@googlegroups.com (external) for any questions, or see
[go/soong](http://go/soong) (internal).
