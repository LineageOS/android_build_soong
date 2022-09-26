import runpy

# The purpose of this file is to implement python 3.11+'s
# PYTHON_SAFE_PATH / -P option on older python versions.

runpy._run_module_as_main("ENTRY_POINT", alter_argv=False)
