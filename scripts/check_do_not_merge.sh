#!/bin/bash

# Copyright (C) 2021 The Android Open Source Project
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

if git show -s --format=%s $1 | grep -qE '(DO NOT MERGE)|(RESTRICT AUTOMERGE)'; then
    cat >&2 <<EOF
DO NOT MERGE and RESTRICT AUTOMERGE very often lead to unintended results
and are not allowed to be used in this project.
Please use the Merged-In tag to be more explicit about where this change
should merge to. Google-internal documentation exists at go/merged-in

If this check is mis-triggering or you know Merged-In is incorrect in this
situation you can bypass this check with \`repo upload --no-verify\`.
EOF
    exit 1
fi
