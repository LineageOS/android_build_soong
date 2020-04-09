# Native API Map Files

Native APIs such as those exposed by the NDK, LL-NDK, or APEX are described by
map.txt files. These files are [linker version scripts] with comments that are
semantically meaningful to [gen_stub_libs.py]. For an example of a map file, see
[libc.map.txt].

[gen_stub_libs.py]: https://cs.android.com/android/platform/superproject/+/master:build/soong/cc/gen_stub_libs.py
[libc.map.txt]: https://cs.android.com/android/platform/superproject/+/master:bionic/libc/libc.map.txt
[linker version scripts]: https://www.gnu.org/software/gnulib/manual/html_node/LD-Version-Scripts.html

## Basic format

A linker version script defines at least one alphanumeric "version" definition,
each of which contain a list of symbols. For example:

```txt
MY_API_R { # introduced=R
  global:
    api_foo;
    api_bar;
  local:
    *;
};

MY_API_S { # introduced=S
  global:
    api_baz;
} MY_API_R;
```

Comments on the same line as either a version definition or a symbol name have
meaning. If you need to add any comments that should not be interpreted by the
stub generator, keep them on their own line. For a list of supported comments,
see the "Tags" section.

Here, `api_foo` and `api_bar` are exposed in the generated stubs with the
`MY_API_R` version and `api_baz` is exposed with the `MY_API_S` version. No
other symbols are defined as public by this API. `MY_API_S` inherits all symbols
defined by `MY_API_R`.

When generating NDK API stubs from this version script, the stub library for R
will define `api_foo` and `api_bar`. The stub library for S will define all
three APIs.

Note that, with few exceptions (see "Special version names" below), the name of
the version has no inherent meaning.

These map files can (and should) also be used as version scripts for building
the implementation library rather than just defining the stub interface by using
the `version_script` property of `cc_library`. This has the effect of limiting
symbol visibility of the library to expose only the interface named by the map
file. Without this, APIs that you have not explicitly exposed will still be
available to users via `dlsym`. Note: All comments are ignored in this case. Any
symbol named in any `global:` group will be visible.

## Special version names

Version names that end with `_PRIVATE` or `_PLATFORM` will not be exposed in any
stubs, but will be exposed in the implementation library. Using either of these
naming schemes is equivalent to marking the version with the `platform-only`
tag. See the docs for `platform-only` for more information.

## Tags

Comments on the same line as a version definition or a symbol name are
interpreted by the stub generator. Multiple space-delimited tags may be used on
the same line. The supported tags are:

### apex

Indicates that the version or symbol is to be exposed in the APEX stubs rather
than the NDK. May be used in combination with `llndk` if the symbol is exposed
to both APEX and the LL-NDK.

### future

Indicates that the version or symbol is first introduced in the "future" API
level. This is an abitrarily high API level used to define APIs that have not
yet been added to a specific release.

### introduced

Indicates the version in which an API was first introduced. For example,
`introduced=21` specifies that the API was first added (or first made public) in
API level 21. This tag can be applied to either a version definition or an
individual symbol. If applied to a version, all symbols contained in the version
will have the tag applied. An `introduced` tag on a symbol overrides the value
set for the version, if both are defined.

Note: The map file alone does not contain all the information needed to
determine which API level an API was added in. The `first_version` property of
`ndk_library` will dictate which API levels stubs are generated for. If the
module sets `first_version: "21"`, no symbols were introduced before API 21.

Codenames can (and typically should) be used when defining new APIs. This allows
the actual number of the API level to remain vague during development of that
release. For example, `introduced=S` can be used to define APIs added in S. Any
code name known to the build system can be used. For a list of versions known to
the build system, see `out/soong/api_levels.json` (if not present, run `m
out/soong/api_levels.json` to generate it).

Architecture-specific variants of this tag exist:

* `introduced-arm=VERSION`
* `introduced-arm64=VERSION`
* `introduced-x86=VERSION`
* `introduced-x86_64=VERSION`

The architecture-specific tag will take precedence over the architecture-generic
tag when generating stubs for that architecture if both are present. If the
symbol is defined with only architecture-specific tags, it will not be present
for architectures that are not named.

Note: The architecture-specific tags should, in general, not be used. These are
primarily needed for APIs that were wrongly inconsistently exposed by libc/libm
in old versions of Android before the stubs were well maintained. Think hard
before using an architecture-specific tag for a new API.

### llndk

Indicates that the version or symbol is to be exposed in the LL-NDK stubs rather
than the NDK. May be used in combination with `apex` if the symbol is exposed to
both APEX and the LL-NDK.

### platform-only

Indicates that the version or symbol is public in the implementation library but
should not be exposed in the stub library. Developers can still access them via
`dlsym`, but they will not be exposed in the stubs so it should at least be
clear to the developer that they are up to no good.

The typical use for this tag is for exposing an API to the platform that is not
for use by the NDK, LL-NDK, or APEX. It is preferable to keep such APIs in an
entirely separate library to protect them from access via `dlsym`, but this is
not always possible.

### var

Used to define a public global variable. By default all symbols are exposed as
functions. In the uncommon situation of exposing a global variable, the `var`
tag may be used.

### versioned=VERSION

Behaves similarly to `introduced` but defines the first version that the stub
library should apply symbol versioning. For example:

```txt
R { # introduced=R
  global:
    foo;
    bar; # versioned=S
  local:
    *;
};
```

The stub library for R will contain symbols for both `foo` and `bar`, but only
`foo` will include a versioned symbol `foo@R`. The stub library for S will
contain both symbols, as well as the versioned symbols `foo@R` and `bar@R`.

This tag is not commonly needed and is only used to hide symbol versioning
mistakes that shipped as part of the platform.

Note: Like `introduced`, the map file does not tell the whole story. The
`ndk_library` Soong module may define a `unversioned_until` property that sets
the default for the entire map file.

### weak

Indicates that the symbol should be [weak] in the stub library.

[weak]: https://gcc.gnu.org/onlinedocs/gcc-4.7.2/gcc/Function-Attributes.html
