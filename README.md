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

Soong deliberately does not support conditionals in Android.bp files.
Instead, complexity in build rules that would require conditionals are handled
in Go, where high level language features can be used and implicit dependencies
introduced by conditionals can be tracked.  Most conditionals are converted
to a map property, where one of the values in the map will be selected and
appended to the top level properties.

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

See [art/build/art.go](https://android.googlesource.com/platform/art/+/master/build/art.go)
or [external/llvm/soong/llvm.go](https://android.googlesource.com/platform/external/llvm/+/master/soong/llvm.go)
for examples of more complex conditionals on product variables or environment variables.

## Developing for Soong

To load Soong code in a Go-aware IDE, create a directory outside your android tree and then:
```bash
apt install bindfs
export GOPATH=<path to the directory you created>
build/soong/scripts/setup_go_workspace_for_soong.sh
```

This will bind mount the Soong source directories into the directory in the layout expected by
the IDE.

## Contact

Email android-building@googlegroups.com (external) for any questions, or see
[go/soong](http://go/soong) (internal).
