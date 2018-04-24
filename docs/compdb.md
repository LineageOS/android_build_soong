# Compdb (compile\_commands.json) Generator

Soong can generate compdb files. This is intended for use with editing tools
such as YouCompleteMe and other libclang based completers.

compdb file generation is enabled via environment variable:

```bash
$ export SOONG_GEN_COMPDB=1
$ export SOONG_GEN_COMPDB_DEBUG=1
```

One can make soong generate a symlink to the compdb file using an environment
variable:

```bash
$ export SOONG_LINK_COMPDB_TO=$ANDROID_HOST_OUT
```

You can then trigger an empty build:

```bash
$ make nothing
```

Note that if you build using mm or other limited makes with these environment
variables set the compdb will only include files in included modules.
