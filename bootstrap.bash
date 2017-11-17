#!/bin/bash

# Copyright 2017 Google Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

echo '==== ERROR: bootstrap.bash & ./soong are obsolete ====' >&2
echo 'Use `m --skip-make` with a standalone OUT_DIR instead.' >&2
echo 'Without envsetup.sh, use:' >&2
echo '  build/soong/soong_ui.bash --make-mode --skip-make' >&2
echo '======================================================' >&2
exit 1

