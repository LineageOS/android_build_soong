import runpy
import sys

sys.argv[0] = __loader__.archive

# Set sys.executable to None. The real executable is available as
# sys.argv[0], and too many things assume sys.executable is a regular Python
# binary, which isn't available. By setting it to None we get clear errors
# when people try to use it.
sys.executable = None

runpy._run_module_as_main("ENTRY_POINT", alter_argv=False)
