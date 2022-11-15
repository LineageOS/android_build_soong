#!/bin/bash

# Copyright 2022 Google Inc. All rights reserved.
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

# Script to detect and report an attempt to access an invalid implementation
# jar.

MOD=$1

cat <<EOF

    $MOD is a java_library that generates a jar file which must not be accessed
    from outside the mainline module that provides it. If you are seeing this
    message it means that you are incorrectly attempting to use the jar file
    from a java_import prebuilt of $MOD.

    This is most likely due to an incorrect dependency on $MOD in an Android.mk
    or Android.bp file. Please remove that dependency and replace with
    something more appropriate, e.g. a dependency on an API provided by the
    module.

    If you do not know where the extraneous dependency was added then you can
    run the following command to find a list of all the paths from the target
    which you are trying to build to the target which produced this error.

        prebuilts/build-tools/linux-x86/bin/ninja -f out/combined-\${TARGET_PRODUCT}.ninja -t path <target> <invalid-jar>

    Where <target> is the build target you specified on the command line which
    produces this error and <invalid-jar> is the rule that failed with this
    message. If you are specifying multiple build targets then you will need to
    run the above command for every target until you find the cause.

    The command will output one (of the possibly many) dependency paths from
    <target> to <invalid-jar>, one file/phony target per line. e.g. it may
    output something like this:

        ....
        out/soong/.intermediates/acme/broken/android_common/combined/broken.jar
        out/soong/.intermediates/prebuilts/module_sdk/art/current/sdk/prebuilt_core-libart/android_common/combined/core-libart.jar
        out/soong/.intermediates/prebuilts/module_sdk/art/current/sdk/art-module-sdk_core-libart-error/gen/this-file-will-never-be-created.jar

    The last line is the failing target, the second to last line is a dependency
    from the core-libart java_import onto the failing target, the third to last
    line is the source of the dependency so you should look in acme/Android.bp
    file for the "broken" module.

EOF

exit 1
