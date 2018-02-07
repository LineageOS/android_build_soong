# Build System Best Practices

## Read only source tree

Never write to the source directory during the build, always write to
`$OUT_DIR`. We expect to enforce this in the future.

If you want to verify / provide an update to a checked in generated source
file, generate that file into `$OUT_DIR` during the build, fail the build
asking the user to run a command (either a straight command, checked in script,
generated script, etc) to explicitly copy that file from the output into the
source tree.

## Network access

Never access the network during the build. We expect to enforce this in the
future, though there will be some level of exceptions for tools like `distcc`
and `goma`.

## Paths

Don't use absolute paths in Ninja files (with make's `$(abspath)` or similar),
as that could trigger extra rebuilds when a source directory is moved.

Assume that the source directory is `$PWD`. If a script is going to change
directories and needs to convert an input from a relative to absolute path,
prefer to do that in the script.

Don't encode absolute paths in build intermediates or outputs. This would make
it difficult to reproduce builds on other machines.

Don't assume that `$OUT_DIR` is `out`. The source and output trees are very
large these days, so some people put these on different disks. There are many
other uses as well.

Don't assume that `$OUT_DIR` is under `$PWD`, users can set it to a relative path
or an absolute path.

## $(shell) use in Android.mk files

Don't use `$(shell)` to write files, create symlinks, etc. We expect to
enforce this in the future. Encode these as build rules in the build graph
instead.  This can be problematic in a number of ways:

* `$(shell)` calls run at the beginning of every build, at minimum this slows
  down build startup, but it can also trigger more build steps to run than are
  necessary, since these files will change more often than necessary.
* It's no longer possible for a stripped-down product configuration to opt-out
  of these created files. It's better to have actual rules and dependencies set
  up so that space isn't wasted, but the files are there when necessary.

## Headers

`LOCAL_COPY_HEADERS` is deprecated. Soong modules cannot use these headers, and
when the VNDK is enabled, System modules in Make cannot declare or use them
either.

The set of global include paths provided by the build system is also being
removed. They've been switched from using `-isystem` to `-I` already, and are
removed entirely in some environments (vendor code when the VNDK is enabled).

Instead, use `LOCAL_EXPORT_C_INCLUDE_DIRS`/`export_include_dirs`. These allow
access to the headers automatically if you link to the associated code.

If your library uses `LOCAL_EXPORT_C_INCLUDE_DIRS`/`export_include_dirs`, and
the exported headers reference a library that you link to, use
`LOCAL_EXPORT_SHARED_LIBRARY_HEADERS`/`LOCAL_EXPORT_STATIC_LIBRARY_HEADERS`/`LOCAL_EXPORT_HEADER_LIBRARY_HEADERS`
(`export_shared_lib_headers`/`export_static_lib_headers`/`export_header_lib_headers`)
to re-export the necessary headers to your users.

Don't use non-local paths in your `LOCAL_EXPORT_C_INCLUDE_DIRS`, use one of the
`LOCAL_EXPORT_*_HEADERS` instead. Non-local exported include dirs are not
supported in Soong. You may need to either move your module definition up a
directory (for example, if you have ./src/ and ./include/, you probably want to
define the module in ./Android.bp, not ./src/Android.bp), define a header
library and re-export it, or move the headers into a more appropriate location.

Prefer to use header libraries (`BUILD_HEADER_LIBRARY`/ `cc_library_headers`)
only if the headers are actually standalone, and do not have associated code.
Sometimes there are headers that have header-only sections, but also define
interfaces to a library. Prefer to split those header-only sections out to a
separate header-only library containing only the header-only sections, and
re-export that header library from the existing library. This will prevent
accidentally linking more code than you need (slower at build and/or runtime),
or accidentally not linking to a library that's actually necessary.

Prefer `LOCAL_EXPORT_C_INCLUDE_DIRS` over `LOCAL_C_INCLUDES` as well.
Eventually we'd like to remove `LOCAL_C_INCLUDES`, though significant cleanup
will be required first. This will be necessary to detect cases where modules
are using headers that shouldn't be available to them -- usually due to the
lack of ABI/API guarantees, but for various other reasons as well: layering
violations, planned deprecations, potential optimizations like C++ modules,
etc.

## Use defaults over variables

Soong supports variable definitions in Android.bp files, but in many cases,
it's better to use defaults modules like `cc_defaults`, `java_defaults`, etc.

* It moves more information next to the values -- that the array of strings
  will be used as a list of sources is useful, both for humans and automated
  tools.  This is even more useful if it's used inside an architecture or
  target specific property.
* It can collect multiple pieces of information together into logical
  inheritable groups that can be selected with a single property.

## Custom build tools

If writing multiple files from a tool, declare them all in the build graph.
* Make: Use `.KATI_IMPLICIT_OUTPUTS`
* Android.bp: Just add them to the `out` list in genrule
* Custom Soong Plugin: Add to `Outputs` or `ImplicitOutputs`

Declare all files read by the tool, either with a dependency if you can, or by
writing a dependency file. Ninja supports a fairly limited set of dependency
file formats. You can verify that the dependencies are read correctly with:

```
NINJA_ARGS="-t deps <output_file>" m
```

Prefer to list input files on the command line, otherwise we may not know to
re-run your command when a new input file is added. Ninja does not treat a
change in dependencies as something that would invalidate an action -- the
command line would need to change, or one of the inputs would need to be newer
than the output file. If you don't include the inputs in your command line, you
may need to add the the directories to your dependency list or dependency file,
so that any additions or removals from those directories would trigger your
tool to be re-run. That can be more expensive than necessary though, since many
editors will write temporary files into the same directory, so changing a
README could trigger the directory's timestamp to be updated.

Only control output files based on the command line, not by an input file. We
need to know which files will be created before any inputs are read, since we
generate the entire build graph before reading source files, or running your
tool. This comes up with Java based tools fairly often -- they'll generate
different output files based on the classes declared in their input files.
We've worked around these tools with the "srcjar" concept, which is just a jar
file containing the generated sources. Our Java compilation tasks understand
*.srcjar files, and will extract them before passing them on to the compiler.

## Libraries in PRODUCT_PACKAGES

Most libraries aren't necessary to include in `PRODUCT_PACKAGES`, unless
they're used dynamically via `dlopen`. If they're only used via
`LOCAL_SHARED_LIBRARIES` / `shared_libs`, then those dependencies will trigger
them to be installed when necessary. Adding unnecessary libraries into
`PRODUCT_PACKAGES` will force them to always be installed, wasting space.
