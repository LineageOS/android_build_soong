#!/bin/bash

echo '==== ERROR: bootstrap.bash & ./soong are obsolete ====' >&2
echo 'Use `m --skip-make` with a standalone OUT_DIR instead.' >&2
echo 'Without envsetup.sh, use:' >&2
echo '  build/soong/soong_ui.bash --make-mode --skip-make' >&2
echo '======================================================' >&2
exit 1

