#!/usr/bin/env -S python -u
#
# Copyright (C) 2022 The Android Open Source Project
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Analyze bootclasspath_fragment usage."""
import argparse
import dataclasses
import enum
import json
import logging
import os
import re
import shutil
import subprocess
import tempfile
import textwrap
import typing
from enum import Enum

import sys

from signature_trie import signature_trie

_STUB_FLAGS_FILE = "out/soong/hiddenapi/hiddenapi-stub-flags.txt"

_FLAGS_FILE = "out/soong/hiddenapi/hiddenapi-flags.csv"

_INCONSISTENT_FLAGS = "ERROR: Hidden API flags are inconsistent:"


class BuildOperation:

    def __init__(self, popen):
        self.popen = popen
        self.returncode = None

    def lines(self):
        """Return an iterator over the lines output by the build operation.

        The lines have had any trailing white space, including the newline
        stripped.
        """
        return newline_stripping_iter(self.popen.stdout.readline)

    def wait(self, *args, **kwargs):
        self.popen.wait(*args, **kwargs)
        self.returncode = self.popen.returncode


@dataclasses.dataclass()
class FlagDiffs:
    """Encapsulates differences in flags reported by the build"""

    # Map from member signature to the (module flags, monolithic flags)
    diffs: typing.Dict[str, typing.Tuple[str, str]]


@dataclasses.dataclass()
class ModuleInfo:
    """Provides access to the generated module-info.json file.

    This is used to find the location of the file within which specific modules
    are defined.
    """

    modules: typing.Dict[str, typing.Dict[str, typing.Any]]

    @staticmethod
    def load(filename):
        with open(filename, "r", encoding="utf8") as f:
            j = json.load(f)
            return ModuleInfo(j)

    def _module(self, module_name):
        """Find module by name in module-info.json file"""
        if module_name in self.modules:
            return self.modules[module_name]

        raise Exception(f"Module {module_name} could not be found")

    def module_path(self, module_name):
        module = self._module(module_name)
        # The "path" is actually a list of paths, one for each class of module
        # but as the modules are all created from bp files if a module does
        # create multiple classes of make modules they should all have the same
        # path.
        paths = module["path"]
        unique_paths = set(paths)
        if len(unique_paths) != 1:
            raise Exception(f"Expected module '{module_name}' to have a "
                            f"single unique path but found {unique_paths}")
        return paths[0]


def extract_indent(line):
    return re.match(r"([ \t]*)", line).group(1)


_SPECIAL_PLACEHOLDER: str = "SPECIAL_PLACEHOLDER"


@dataclasses.dataclass
class BpModifyRunner:

    bpmodify_path: str

    def add_values_to_property(self, property_name, values, module_name,
                               bp_file):
        cmd = [
            self.bpmodify_path, "-a", values, "-property", property_name, "-m",
            module_name, "-w", bp_file, bp_file
        ]

        logging.debug(" ".join(cmd))
        subprocess.run(
            cmd,
            stderr=subprocess.STDOUT,
            stdout=log_stream_for_subprocess(),
            check=True)


@dataclasses.dataclass
class FileChange:
    path: str

    description: str

    def __lt__(self, other):
        return self.path < other.path


class PropertyChangeAction(Enum):
    """Allowable actions that are supported by HiddenApiPropertyChange."""

    # New values are appended to any existing values.
    APPEND = 1

    # New values replace any existing values.
    REPLACE = 2


@dataclasses.dataclass
class HiddenApiPropertyChange:

    property_name: str

    values: typing.List[str]

    property_comment: str = ""

    # The action that indicates how this change is applied.
    action: PropertyChangeAction = PropertyChangeAction.APPEND

    def snippet(self, indent):
        snippet = "\n"
        snippet += format_comment_as_text(self.property_comment, indent)
        snippet += f"{indent}{self.property_name}: ["
        if self.values:
            snippet += "\n"
            for value in self.values:
                snippet += f'{indent}    "{value}",\n'
            snippet += f"{indent}"
        snippet += "],\n"
        return snippet

    def fix_bp_file(self, bcpf_bp_file, bcpf, bpmodify_runner: BpModifyRunner):
        # Add an additional placeholder value to identify the modification that
        # bpmodify makes.
        bpmodify_values = [_SPECIAL_PLACEHOLDER]

        if self.action == PropertyChangeAction.APPEND:
            # If adding the values to the existing values then pass the new
            # values to bpmodify.
            bpmodify_values.extend(self.values)
        elif self.action == PropertyChangeAction.REPLACE:
            # If replacing the existing values then it is not possible to use
            # bpmodify for that directly. It could be used twice to remove the
            # existing property and then add a new one but that does not remove
            # any related comments and loses the position of the existing
            # property as the new property is always added to the end of the
            # containing block.
            #
            # So, instead of passing the new values to bpmodify this this just
            # adds an extra placeholder to force bpmodify to format the list
            # across multiple lines to ensure a consistent structure for the
            # code that removes all the existing values and adds the new ones.
            #
            # This placeholder has to be different to the other placeholder as
            # bpmodify dedups values.
            bpmodify_values.append(_SPECIAL_PLACEHOLDER + "_REPLACE")
        else:
            raise ValueError(f"unknown action {self.action}")

        packages = ",".join(bpmodify_values)
        bpmodify_runner.add_values_to_property(
            f"hidden_api.{self.property_name}", packages, bcpf, bcpf_bp_file)

        with open(bcpf_bp_file, "r", encoding="utf8") as tio:
            lines = tio.readlines()
            lines = [line.rstrip("\n") for line in lines]

        if self.fixup_bpmodify_changes(bcpf_bp_file, lines):
            with open(bcpf_bp_file, "w", encoding="utf8") as tio:
                for line in lines:
                    print(line, file=tio)

    def fixup_bpmodify_changes(self, bcpf_bp_file, lines):
        """Fixup the output of bpmodify.

        The bpmodify tool does not support all the capabilities that this needs
        so it is used to do what it can, including marking the place in the
        Android.bp file where it makes its changes and then this gets passed a
        list of lines from that file which it then modifies to complete the
        change.

        This analyzes the list of lines to find the indices of the significant
        lines and then applies some changes. As those changes can insert and
        delete lines (changing the indices of following lines) the changes are
        generally done in reverse order starting from the end and working
        towards the beginning. That ensures that the changes do not invalidate
        the indices of following lines.
        """

        # Find the line containing the placeholder that has been inserted.
        place_holder_index = -1
        for i, line in enumerate(lines):
            if _SPECIAL_PLACEHOLDER in line:
                place_holder_index = i
                break
        if place_holder_index == -1:
            logging.debug("Could not find %s in %s", _SPECIAL_PLACEHOLDER,
                          bcpf_bp_file)
            return False

        # Remove the place holder. Do this before inserting the comment as that
        # would change the location of the place holder in the list.
        place_holder_line = lines[place_holder_index]
        if place_holder_line.endswith("],"):
            place_holder_line = place_holder_line.replace(
                f'"{_SPECIAL_PLACEHOLDER}"', "")
            lines[place_holder_index] = place_holder_line
        else:
            del lines[place_holder_index]

        # Scan forward to the end of the property block to remove a blank line
        # that bpmodify inserts.
        end_property_array_index = -1
        for i in range(place_holder_index, len(lines)):
            line = lines[i]
            if line.endswith("],"):
                end_property_array_index = i
                break
        if end_property_array_index == -1:
            logging.debug("Could not find end of property array in %s",
                          bcpf_bp_file)
            return False

        # If bdmodify inserted a blank line afterwards then remove it.
        if (not lines[end_property_array_index + 1] and
                lines[end_property_array_index + 2].endswith("},")):
            del lines[end_property_array_index + 1]

        # Scan back to find the preceding property line.
        property_line_index = -1
        for i in range(place_holder_index, 0, -1):
            line = lines[i]
            if line.lstrip().startswith(f"{self.property_name}: ["):
                property_line_index = i
                break
        if property_line_index == -1:
            logging.debug("Could not find property line in %s", bcpf_bp_file)
            return False

        # If this change is replacing the existing values then they need to be
        # removed and replaced with the new values. That will change the lines
        # after the property but it is necessary to do here as the following
        # code operates on earlier lines.
        if self.action == PropertyChangeAction.REPLACE:
            # This removes the existing values and replaces them with the new
            # values.
            indent = extract_indent(lines[property_line_index + 1])
            insert = [f'{indent}"{x}",' for x in self.values]
            lines[property_line_index + 1:end_property_array_index] = insert
            if not self.values:
                # If the property has no values then merge the ], onto the
                # same line as the property name.
                del lines[property_line_index + 1]
                lines[property_line_index] = lines[property_line_index] + "],"

        # Only insert a comment if the property does not already have a comment.
        line_preceding_property = lines[(property_line_index - 1)]
        if (self.property_comment and
                not re.match("([ \t]+)// ", line_preceding_property)):
            # Extract the indent from the property line and use it to format the
            # comment.
            indent = extract_indent(lines[property_line_index])
            comment_lines = format_comment_as_lines(self.property_comment,
                                                    indent)

            # If the line before the comment is not blank then insert an extra
            # blank line at the beginning of the comment.
            if line_preceding_property:
                comment_lines.insert(0, "")

            # Insert the comment before the property.
            lines[property_line_index:property_line_index] = comment_lines
        return True


@dataclasses.dataclass()
class Result:
    """Encapsulates the result of the analysis."""

    # The diffs in the flags.
    diffs: typing.Optional[FlagDiffs] = None

    # The bootclasspath_fragment hidden API properties changes.
    property_changes: typing.List[HiddenApiPropertyChange] = dataclasses.field(
        default_factory=list)

    # The list of file changes.
    file_changes: typing.List[FileChange] = dataclasses.field(
        default_factory=list)


class ClassProvider(enum.Enum):
    """The source of a class found during the hidden API processing"""
    BCPF = "bcpf"
    OTHER = "other"


# A fake member to use when using the signature trie to compute the package
# properties from hidden API flags. This is needed because while that
# computation only cares about classes the trie expects a class to be an
# interior node but without a member it makes the class a leaf node. That causes
# problems when analyzing inner classes as the outer class is a leaf node for
# its own entry but is used as an interior node for inner classes.
_FAKE_MEMBER = ";->fake()V"


@dataclasses.dataclass()
class BcpfAnalyzer:
    # Path to this tool.
    tool_path: str

    # Directory pointed to by ANDROID_BUILD_OUT
    top_dir: str

    # Directory pointed to by OUT_DIR of {top_dir}/out if that is not set.
    out_dir: str

    # Directory pointed to by ANDROID_PRODUCT_OUT.
    product_out_dir: str

    # The name of the bootclasspath_fragment module.
    bcpf: str

    # The name of the apex module containing {bcpf}, only used for
    # informational purposes.
    apex: str

    # The name of the sdk module containing {bcpf}, only used for
    # informational purposes.
    sdk: str

    # If true then this will attempt to automatically fix any issues that are
    # found.
    fix: bool = False

    # All the signatures, loaded from all-flags.csv, initialized by
    # load_all_flags().
    _signatures: typing.Set[str] = dataclasses.field(default_factory=set)

    # All the classes, loaded from all-flags.csv, initialized by
    # load_all_flags().
    _classes: typing.Set[str] = dataclasses.field(default_factory=set)

    # Information loaded from module-info.json, initialized by
    # load_module_info().
    module_info: ModuleInfo = None

    @staticmethod
    def reformat_report_test(text):
        return re.sub(r"(.)\n([^\s])", r"\1 \2", text)

    def report(self, text, **kwargs):
        # Concatenate lines that are not separated by a blank line together to
        # eliminate formatting applied to the supplied text to adhere to python
        # line length limitations.
        text = self.reformat_report_test(text)
        logging.info("%s", text, **kwargs)

    def run_command(self, cmd, *args, **kwargs):
        cmd_line = " ".join(cmd)
        logging.debug("Running %s", cmd_line)
        subprocess.run(
            cmd,
            *args,
            check=True,
            cwd=self.top_dir,
            stderr=subprocess.STDOUT,
            stdout=log_stream_for_subprocess(),
            text=True,
            **kwargs)

    @property
    def signatures(self):
        if not self._signatures:
            raise Exception("signatures has not been initialized")
        return self._signatures

    @property
    def classes(self):
        if not self._classes:
            raise Exception("classes has not been initialized")
        return self._classes

    def load_all_flags(self):
        all_flags = self.find_bootclasspath_fragment_output_file(
            "all-flags.csv")

        # Extract the set of signatures and a separate set of classes produced
        # by the bootclasspath_fragment.
        with open(all_flags, "r", encoding="utf8") as f:
            for line in newline_stripping_iter(f.readline):
                signature = self.line_to_signature(line)
                self._signatures.add(signature)
                class_name = self.signature_to_class(signature)
                self._classes.add(class_name)

    def load_module_info(self):
        module_info_file = os.path.join(self.product_out_dir,
                                        "module-info.json")
        self.report(f"""
Making sure that {module_info_file} is up to date.
""")
        output = self.build_file_read_output(module_info_file)
        lines = output.lines()
        for line in lines:
            logging.debug("%s", line)
        output.wait(timeout=10)
        if output.returncode:
            raise Exception(f"Error building {module_info_file}")
        abs_module_info_file = os.path.join(self.top_dir, module_info_file)
        self.module_info = ModuleInfo.load(abs_module_info_file)

    @staticmethod
    def line_to_signature(line):
        return line.split(",")[0]

    @staticmethod
    def signature_to_class(signature):
        return signature.split(";->")[0]

    @staticmethod
    def to_parent_package(pkg_or_class):
        return pkg_or_class.rsplit("/", 1)[0]

    def module_path(self, module_name):
        return self.module_info.module_path(module_name)

    def module_out_dir(self, module_name):
        module_path = self.module_path(module_name)
        return os.path.join(self.out_dir, "soong/.intermediates", module_path,
                            module_name)

    def find_bootclasspath_fragment_output_file(self, basename, required=True):
        # Find the output file of the bootclasspath_fragment with the specified
        # base name.
        found_file = ""
        bcpf_out_dir = self.module_out_dir(self.bcpf)
        for (dirpath, _, filenames) in os.walk(bcpf_out_dir):
            for f in filenames:
                if f == basename:
                    found_file = os.path.join(dirpath, f)
                    break
        if not found_file and required:
            raise Exception(f"Could not find {basename} in {bcpf_out_dir}")
        return found_file

    def analyze(self):
        """Analyze a bootclasspath_fragment module.

        Provides help in resolving any existing issues and provides
        optimizations that can be applied.
        """
        self.report(f"Analyzing bootclasspath_fragment module {self.bcpf}")
        self.report(f"""
Run this tool to help initialize a bootclasspath_fragment module. Before you
start make sure that:

1. The current checkout is up to date.

2. The environment has been initialized using lunch, e.g.
   lunch aosp_arm64-userdebug

3. You have added a bootclasspath_fragment module to the appropriate Android.bp
file. Something like this:

   bootclasspath_fragment {{
     name: "{self.bcpf}",
     contents: [
       "...",
     ],

     // The bootclasspath_fragments that provide APIs on which this depends.
     fragments: [
       {{
         apex: "com.android.art",
         module: "art-bootclasspath-fragment",
       }},
     ],
   }}

4. You have added it to the platform_bootclasspath module in
frameworks/base/boot/Android.bp. Something like this:

   platform_bootclasspath {{
     name: "platform-bootclasspath",
     fragments: [
       ...
       {{
         apex: "{self.apex}",
         module: "{self.bcpf}",
       }},
     ],
   }}

5. You have added an sdk module. Something like this:

   sdk {{
     name: "{self.sdk}",
     bootclasspath_fragments: ["{self.bcpf}"],
   }}
""")

        # Make sure that the module-info.json file is up to date.
        self.load_module_info()

        self.report("""
Cleaning potentially stale files.
""")
        # Remove the out/soong/hiddenapi files.
        shutil.rmtree(f"{self.out_dir}/soong/hiddenapi", ignore_errors=True)

        # Remove any bootclasspath_fragment output files.
        shutil.rmtree(self.module_out_dir(self.bcpf), ignore_errors=True)

        self.build_monolithic_stubs_flags()

        result = Result()

        self.build_monolithic_flags(result)
        self.analyze_hiddenapi_package_properties(result)
        self.explain_how_to_check_signature_patterns()

        # If there were any changes that need to be made to the Android.bp
        # file then either apply or report them.
        if result.property_changes:
            bcpf_dir = self.module_info.module_path(self.bcpf)
            bcpf_bp_file = os.path.join(self.top_dir, bcpf_dir, "Android.bp")
            if self.fix:
                tool_dir = os.path.dirname(self.tool_path)
                bpmodify_path = os.path.join(tool_dir, "bpmodify")
                bpmodify_runner = BpModifyRunner(bpmodify_path)
                for property_change in result.property_changes:
                    property_change.fix_bp_file(bcpf_bp_file, self.bcpf,
                                                bpmodify_runner)

                result.file_changes.append(
                    self.new_file_change(
                        bcpf_bp_file,
                        f"Updated hidden_api properties of '{self.bcpf}'"))

            else:
                hiddenapi_snippet = ""
                for property_change in result.property_changes:
                    hiddenapi_snippet += property_change.snippet("        ")

                # Remove leading and trailing blank lines.
                hiddenapi_snippet = hiddenapi_snippet.strip("\n")

                result.file_changes.append(
                    self.new_file_change(
                        bcpf_bp_file, f"""
Add the following snippet into the {self.bcpf} bootclasspath_fragment module
in the {bcpf_dir}/Android.bp file. If the hidden_api block already exists then
merge these properties into it.

    hidden_api: {{
{hiddenapi_snippet}
    }},
"""))

        if result.file_changes:
            if self.fix:
                file_change_message = """
The following files were modified by this script:"""
            else:
                file_change_message = """
The following modifications need to be made:"""

            self.report(f"""
{file_change_message}""")
            result.file_changes.sort()
            for file_change in result.file_changes:
                self.report(f"""
    {file_change.path}
        {file_change.description}
""".lstrip("\n"))

            if not self.fix:
                self.report("""
Run the command again with the --fix option to automatically make the above
changes.
""".lstrip())

    def new_file_change(self, file, description):
        return FileChange(
            path=os.path.relpath(file, self.top_dir), description=description)

    def check_inconsistent_flag_lines(self, significant, module_line,
                                      monolithic_line, separator_line):
        if not (module_line.startswith("< ") and
                monolithic_line.startswith("> ") and not separator_line):
            # Something went wrong.
            self.report(f"""Invalid build output detected:
  module_line: "{module_line}"
  monolithic_line: "{monolithic_line}"
  separator_line: "{separator_line}"
""")
            sys.exit(1)

        if significant:
            logging.debug("%s", module_line)
            logging.debug("%s", monolithic_line)
            logging.debug("%s", separator_line)

    def scan_inconsistent_flags_report(self, lines):
        """Scans a hidden API flags report

        The hidden API inconsistent flags report which looks something like
        this.

        < out/soong/.intermediates/.../filtered-stub-flags.csv
        > out/soong/hiddenapi/hiddenapi-stub-flags.txt

        < Landroid/compat/Compatibility;->clearOverrides()V
        > Landroid/compat/Compatibility;->clearOverrides()V,core-platform-api

        """

        # The basic format of an entry in the inconsistent flags report is:
        #   <module specific flag>
        #   <monolithic flag>
        #   <separator>
        #
        # Wrap the lines iterator in an iterator which returns a tuple
        # consisting of the three separate lines.
        triples = zip(lines, lines, lines)

        module_line, monolithic_line, separator_line = next(triples)
        significant = False
        bcpf_dir = self.module_info.module_path(self.bcpf)
        if os.path.join(bcpf_dir, self.bcpf) in module_line:
            # These errors are related to the bcpf being analyzed so
            # keep them.
            significant = True
        else:
            self.report(f"Filtering out errors related to {module_line}")

        self.check_inconsistent_flag_lines(significant, module_line,
                                           monolithic_line, separator_line)

        diffs = {}
        for module_line, monolithic_line, separator_line in triples:
            self.check_inconsistent_flag_lines(significant, module_line,
                                               monolithic_line, "")

            module_parts = module_line.removeprefix("< ").split(",")
            module_signature = module_parts[0]
            module_flags = module_parts[1:]

            monolithic_parts = monolithic_line.removeprefix("> ").split(",")
            monolithic_signature = monolithic_parts[0]
            monolithic_flags = monolithic_parts[1:]

            if module_signature != monolithic_signature:
                # Something went wrong.
                self.report(f"""Inconsistent signatures detected:
  module_signature: "{module_signature}"
  monolithic_signature: "{monolithic_signature}"
""")
                sys.exit(1)

            diffs[module_signature] = (module_flags, monolithic_flags)

            if separator_line:
                # If the separator line is not blank then it is the end of the
                # current report, and possibly the start of another.
                return separator_line, diffs

        return "", diffs

    def build_file_read_output(self, filename):
        # Make sure the filename is relative to top if possible as the build
        # may be using relative paths as the target.
        rel_filename = filename.removeprefix(self.top_dir)
        cmd = ["build/soong/soong_ui.bash", "--make-mode", rel_filename]
        cmd_line = " ".join(cmd)
        logging.debug("%s", cmd_line)
        # pylint: disable=consider-using-with
        output = subprocess.Popen(
            cmd,
            cwd=self.top_dir,
            stderr=subprocess.STDOUT,
            stdout=subprocess.PIPE,
            text=True,
        )
        return BuildOperation(popen=output)

    def build_hiddenapi_flags(self, filename):
        output = self.build_file_read_output(filename)

        lines = output.lines()
        diffs = None
        for line in lines:
            logging.debug("%s", line)
            while line == _INCONSISTENT_FLAGS:
                line, diffs = self.scan_inconsistent_flags_report(lines)

        output.wait(timeout=10)
        if output.returncode != 0:
            logging.debug("Command failed with %s", output.returncode)
        else:
            logging.debug("Command succeeded")

        return diffs

    def build_monolithic_stubs_flags(self):
        self.report(f"""
Attempting to build {_STUB_FLAGS_FILE} to verify that the
bootclasspath_fragment has the correct API stubs available...
""")

        # Build the hiddenapi-stubs-flags.txt file.
        diffs = self.build_hiddenapi_flags(_STUB_FLAGS_FILE)
        if diffs:
            self.report(f"""
There is a discrepancy between the stub API derived flags created by the
bootclasspath_fragment and the platform_bootclasspath. See preceding error
messages to see which flags are inconsistent. The inconsistencies can occur for
a couple of reasons:

If you are building against prebuilts of the Android SDK, e.g. by using
TARGET_BUILD_APPS then the prebuilt versions of the APIs this
bootclasspath_fragment depends upon are out of date and need updating. See
go/update-prebuilts for help.

Otherwise, this is happening because there are some stub APIs that are either
provided by or used by the contents of the bootclasspath_fragment but which are
not available to it. There are 4 ways to handle this:

1. A java_sdk_library in the contents property will automatically make its stub
   APIs available to the bootclasspath_fragment so nothing needs to be done.

2. If the API provided by the bootclasspath_fragment is created by an api_only
   java_sdk_library (or a java_library that compiles files generated by a
   separate droidstubs module then it cannot be added to the contents and
   instead must be added to the api.stubs property, e.g.

   bootclasspath_fragment {{
     name: "{self.bcpf}",
     ...
     api: {{
       stubs: ["$MODULE-api-only"],"
     }},
   }}

3. If the contents use APIs provided by another bootclasspath_fragment then
   it needs to be added to the fragments property, e.g.

   bootclasspath_fragment {{
     name: "{self.bcpf}",
     ...
     // The bootclasspath_fragments that provide APIs on which this depends.
     fragments: [
       ...
       {{
         apex: "com.android.other",
         module: "com.android.other-bootclasspath-fragment",
       }},
     ],
   }}

4. If the contents use APIs from a module that is not part of another
   bootclasspath_fragment then it must be added to the additional_stubs
   property, e.g.

   bootclasspath_fragment {{
     name: "{self.bcpf}",
     ...
     additional_stubs: ["android-non-updatable"],
   }}

   Like the api.stubs property these are typically java_sdk_library modules but
   can be java_library too.

   Note: The "android-non-updatable" is treated as if it was a java_sdk_library
   which it is not at the moment but will be in future.
""")

        return diffs

    def build_monolithic_flags(self, result):
        self.report(f"""
Attempting to build {_FLAGS_FILE} to verify that the
bootclasspath_fragment has the correct hidden API flags...
""")

        # Build the hiddenapi-flags.csv file and extract any differences in
        # the flags between this bootclasspath_fragment and the monolithic
        # files.
        result.diffs = self.build_hiddenapi_flags(_FLAGS_FILE)

        # Load information from the bootclasspath_fragment's all-flags.csv file.
        self.load_all_flags()

        if result.diffs:
            self.report(f"""
There is a discrepancy between the hidden API flags created by the
bootclasspath_fragment and the platform_bootclasspath. See preceding error
messages to see which flags are inconsistent. The inconsistencies can occur for
a couple of reasons:

If you are building against prebuilts of this bootclasspath_fragment then the
prebuilt version of the sdk snapshot (specifically the hidden API flag files)
are inconsistent with the prebuilt version of the apex {self.apex}. Please
ensure that they are both updated from the same build.

1. There are custom hidden API flags specified in the one of the files in
   frameworks/base/boot/hiddenapi which apply to the bootclasspath_fragment but
   which are not supplied to the bootclasspath_fragment module.

2. The bootclasspath_fragment specifies invalid "package_prefixes" or
   "split_packages" properties that match packages and classes that it does not
   provide.

""")

            # Check to see if there are any hiddenapi related properties that
            # need to be added to the
            self.report("""
Checking custom hidden API flags....
""")
            self.check_frameworks_base_boot_hidden_api_files(result)

    def report_hidden_api_flag_file_changes(self, result, property_name,
                                            flags_file, rel_bcpf_flags_file,
                                            bcpf_flags_file):
        matched_signatures = set()
        # Open the flags file to read the flags from.
        with open(flags_file, "r", encoding="utf8") as f:
            for signature in newline_stripping_iter(f.readline):
                if signature in self.signatures:
                    # The signature is provided by the bootclasspath_fragment so
                    # it will need to be moved to the bootclasspath_fragment
                    # specific file.
                    matched_signatures.add(signature)

        # If the bootclasspath_fragment specific flags file is not empty
        # then it contains flags. That could either be new flags just moved
        # from frameworks/base or previous contents of the file. In either
        # case the file must not be removed.
        if matched_signatures:
            insert = textwrap.indent("\n".join(matched_signatures),
                                     "            ")
            result.file_changes.append(
                self.new_file_change(
                    flags_file, f"""Remove the following entries:
{insert}
"""))

            result.file_changes.append(
                self.new_file_change(
                    bcpf_flags_file, f"""Add the following entries:
{insert}
"""))

            result.property_changes.append(
                HiddenApiPropertyChange(
                    property_name=property_name,
                    values=[rel_bcpf_flags_file],
                ))

    def fix_hidden_api_flag_files(self, result, property_name, flags_file,
                                  rel_bcpf_flags_file, bcpf_flags_file):
        # Read the file in frameworks/base/boot/hiddenapi/<file> copy any
        # flags that relate to the bootclasspath_fragment into a local
        # file in the hiddenapi subdirectory.
        tmp_flags_file = flags_file + ".tmp"

        # Make sure the directory containing the bootclasspath_fragment specific
        # hidden api flags exists.
        os.makedirs(os.path.dirname(bcpf_flags_file), exist_ok=True)

        bcpf_flags_file_exists = os.path.exists(bcpf_flags_file)

        matched_signatures = set()
        # Open the flags file to read the flags from.
        with open(flags_file, "r", encoding="utf8") as f:
            # Open a temporary file to write the flags (minus any removed
            # flags).
            with open(tmp_flags_file, "w", encoding="utf8") as t:
                # Open the bootclasspath_fragment file for append just in
                # case it already exists.
                with open(bcpf_flags_file, "a", encoding="utf8") as b:
                    for line in iter(f.readline, ""):
                        signature = line.rstrip()
                        if signature in self.signatures:
                            # The signature is provided by the
                            # bootclasspath_fragment so write it to the new
                            # bootclasspath_fragment specific file.
                            print(line, file=b, end="")
                            matched_signatures.add(signature)
                        else:
                            # The signature is NOT provided by the
                            # bootclasspath_fragment. Copy it to the new
                            # monolithic file.
                            print(line, file=t, end="")

        # If the bootclasspath_fragment specific flags file is not empty
        # then it contains flags. That could either be new flags just moved
        # from frameworks/base or previous contents of the file. In either
        # case the file must not be removed.
        if matched_signatures:
            # There are custom flags related to the bootclasspath_fragment
            # so replace the frameworks/base/boot/hiddenapi file with the
            # file that does not contain those flags.
            shutil.move(tmp_flags_file, flags_file)

            result.file_changes.append(
                self.new_file_change(flags_file,
                                     f"Removed '{self.bcpf}' specific entries"))

            result.property_changes.append(
                HiddenApiPropertyChange(
                    property_name=property_name,
                    values=[rel_bcpf_flags_file],
                ))

            # Make sure that the files are sorted.
            self.run_command([
                "tools/platform-compat/hiddenapi/sort_api.sh",
                bcpf_flags_file,
            ])

            if bcpf_flags_file_exists:
                desc = f"Added '{self.bcpf}' specific entries"
            else:
                desc = f"Created with '{self.bcpf}' specific entries"
            result.file_changes.append(
                self.new_file_change(bcpf_flags_file, desc))
        else:
            # There are no custom flags related to the
            # bootclasspath_fragment so clean up the working files.
            os.remove(tmp_flags_file)
            if not bcpf_flags_file_exists:
                os.remove(bcpf_flags_file)

    def check_frameworks_base_boot_hidden_api_files(self, result):
        hiddenapi_dir = os.path.join(self.top_dir,
                                     "frameworks/base/boot/hiddenapi")
        for basename in sorted(os.listdir(hiddenapi_dir)):
            if not (basename.startswith("hiddenapi-") and
                    basename.endswith(".txt")):
                continue

            flags_file = os.path.join(hiddenapi_dir, basename)

            logging.debug("Checking %s for flags related to %s", flags_file,
                          self.bcpf)

            # Map the file name in frameworks/base/boot/hiddenapi into a
            # slightly more meaningful name for use by the
            # bootclasspath_fragment.
            if basename == "hiddenapi-max-target-o.txt":
                basename = "hiddenapi-max-target-o-low-priority.txt"
            elif basename == "hiddenapi-max-target-r-loprio.txt":
                basename = "hiddenapi-max-target-r-low-priority.txt"

            property_name = basename.removeprefix("hiddenapi-")
            property_name = property_name.removesuffix(".txt")
            property_name = property_name.replace("-", "_")

            rel_bcpf_flags_file = f"hiddenapi/{basename}"
            bcpf_dir = self.module_info.module_path(self.bcpf)
            bcpf_flags_file = os.path.join(self.top_dir, bcpf_dir,
                                           rel_bcpf_flags_file)

            if self.fix:
                self.fix_hidden_api_flag_files(result, property_name,
                                               flags_file, rel_bcpf_flags_file,
                                               bcpf_flags_file)
            else:
                self.report_hidden_api_flag_file_changes(
                    result, property_name, flags_file, rel_bcpf_flags_file,
                    bcpf_flags_file)

    @staticmethod
    def split_package_comment(split_packages):
        if split_packages:
            return textwrap.dedent("""
                The following packages contain classes from other modules on the
                bootclasspath. That means that the hidden API flags for this
                module has to explicitly list every single class this module
                provides in that package to differentiate them from the classes
                provided by other modules. That can include private classes that
                are not part of the API.
            """).strip("\n")

        return "This module does not contain any split packages."

    @staticmethod
    def package_prefixes_comment():
        return textwrap.dedent("""
            The following packages and all their subpackages currently only
            contain classes from this bootclasspath_fragment. Listing a package
            here won't prevent other bootclasspath modules from adding classes
            in any of those packages but it will prevent them from adding those
            classes into an API surface, e.g. public, system, etc.. Doing so
            will result in a build failure due to inconsistent flags.
        """).strip("\n")

    def analyze_hiddenapi_package_properties(self, result):
        split_packages, single_packages, package_prefixes = \
            self.compute_hiddenapi_package_properties()

        # TODO(b/202154151): Find those classes in split packages that are not
        #  part of an API, i.e. are an internal implementation class, and so
        #  can, and should, be safely moved out of the split packages.

        result.property_changes.append(
            HiddenApiPropertyChange(
                property_name="split_packages",
                values=split_packages,
                property_comment=self.split_package_comment(split_packages),
                action=PropertyChangeAction.REPLACE,
            ))

        if split_packages:
            self.report(f"""
bootclasspath_fragment {self.bcpf} contains classes in packages that also
contain classes provided by other sources, those packages are called split
packages. Split packages should be avoided where possible but are often
unavoidable when modularizing existing code.

The hidden api processing needs to know which packages are split (and conversely
which are not) so that it can optimize the hidden API flags to remove
unnecessary implementation details.
""")

        self.report("""
By default (for backwards compatibility) the bootclasspath_fragment assumes that
all packages are split unless one of the package_prefixes or split_packages
properties are specified. While that is safe it is not optimal and can lead to
unnecessary implementation details leaking into the hidden API flags. Adding an
empty split_packages property allows the flags to be optimized and remove any
unnecessary implementation details.
""")

        if single_packages:
            result.property_changes.append(
                HiddenApiPropertyChange(
                    property_name="single_packages",
                    values=single_packages,
                    property_comment=textwrap.dedent("""
                    The following packages currently only contain classes from
                    this bootclasspath_fragment but some of their sub-packages
                    contain classes from other bootclasspath modules. Packages
                    should only be listed here when necessary for legacy
                    purposes, new packages should match a package prefix.
                """),
                    action=PropertyChangeAction.REPLACE,
                ))

        if package_prefixes:
            result.property_changes.append(
                HiddenApiPropertyChange(
                    property_name="package_prefixes",
                    values=package_prefixes,
                    property_comment=self.package_prefixes_comment(),
                    action=PropertyChangeAction.REPLACE,
                ))

    def explain_how_to_check_signature_patterns(self):
        signature_patterns_files = self.find_bootclasspath_fragment_output_file(
            "signature-patterns.csv", required=False)
        if signature_patterns_files:
            signature_patterns_files = signature_patterns_files.removeprefix(
                self.top_dir)

            self.report(f"""
The purpose of the hiddenapi split_packages and package_prefixes properties is
to allow the removal of implementation details from the hidden API flags to
reduce the coupling between sdk snapshots and the APEX runtime. It cannot
eliminate that coupling completely though. Doing so may require changes to the
code.

This tool provides support for managing those properties but it cannot decide
whether the set of package prefixes suggested is appropriate that needs the
input of the developer.

Please run the following command:
    m {signature_patterns_files}

And then check the '{signature_patterns_files}' for any mention of
implementation classes and packages (i.e. those classes/packages that do not
contain any part of an API surface, including the hidden API). If they are
found then the code should ideally be moved to a package unique to this module
that is contained within a package that is part of an API surface.

The format of the file is a list of patterns:

* Patterns for split packages will list every class in that package.

* Patterns for package prefixes will end with .../**.

* Patterns for packages which are not split but cannot use a package prefix
because there are sub-packages which are provided by another module will end
with .../*.
""")

    def compute_hiddenapi_package_properties(self):
        trie = signature_trie()
        # Populate the trie with the classes that are provided by the
        # bootclasspath_fragment tagging them to make it clear where they
        # are from.
        sorted_classes = sorted(self.classes)
        for class_name in sorted_classes:
            trie.add(class_name + _FAKE_MEMBER, ClassProvider.BCPF)

        monolithic_classes = set()
        abs_flags_file = os.path.join(self.top_dir, _FLAGS_FILE)
        with open(abs_flags_file, "r", encoding="utf8") as f:
            for line in iter(f.readline, ""):
                signature = self.line_to_signature(line)
                class_name = self.signature_to_class(signature)
                if (class_name not in monolithic_classes and
                        class_name not in self.classes):
                    trie.add(
                        class_name + _FAKE_MEMBER,
                        ClassProvider.OTHER,
                        only_if_matches=True)
                    monolithic_classes.add(class_name)

        split_packages = []
        single_packages = []
        package_prefixes = []
        self.recurse_hiddenapi_packages_trie(trie, split_packages,
                                             single_packages, package_prefixes)
        return split_packages, single_packages, package_prefixes

    def recurse_hiddenapi_packages_trie(self, node, split_packages,
                                        single_packages, package_prefixes):
        nodes = node.child_nodes()
        if nodes:
            for child in nodes:
                # Ignore any non-package nodes.
                if child.type != "package":
                    continue

                package = child.selector.replace("/", ".")

                providers = set(child.get_matching_rows("**"))
                if not providers:
                    # The package and all its sub packages contain no
                    # classes. This should never happen.
                    pass
                elif providers == {ClassProvider.BCPF}:
                    # The package and all its sub packages only contain
                    # classes provided by the bootclasspath_fragment.
                    logging.debug("Package '%s.**' is not split", package)
                    package_prefixes.append(package)
                    # There is no point traversing into the sub packages.
                    continue
                elif providers == {ClassProvider.OTHER}:
                    # The package and all its sub packages contain no
                    # classes provided by the bootclasspath_fragment.
                    # There is no point traversing into the sub packages.
                    logging.debug("Package '%s.**' contains no classes from %s",
                                  package, self.bcpf)
                    continue
                elif ClassProvider.BCPF in providers:
                    # The package and all its sub packages contain classes
                    # provided by the bootclasspath_fragment and other
                    # sources.
                    logging.debug(
                        "Package '%s.**' contains classes from "
                        "%s and other sources", package, self.bcpf)

                providers = set(child.get_matching_rows("*"))
                if not providers:
                    # The package contains no classes.
                    logging.debug("Package: %s contains no classes", package)
                elif providers == {ClassProvider.BCPF}:
                    # The package only contains classes provided by the
                    # bootclasspath_fragment.
                    logging.debug("Package '%s.*' is not split", package)
                    single_packages.append(package)
                elif providers == {ClassProvider.OTHER}:
                    # The package contains no classes provided by the
                    # bootclasspath_fragment. Child nodes make contain such
                    # classes.
                    logging.debug("Package '%s.*' contains no classes from %s",
                                  package, self.bcpf)
                elif ClassProvider.BCPF in providers:
                    # The package contains classes provided by both the
                    # bootclasspath_fragment and some other source.
                    logging.debug("Package '%s.*' is split", package)
                    split_packages.append(package)

                self.recurse_hiddenapi_packages_trie(child, split_packages,
                                                     single_packages,
                                                     package_prefixes)


def newline_stripping_iter(iterator):
    """Return an iterator over the iterator that strips trailing white space."""
    lines = iter(iterator, "")
    lines = (line.rstrip() for line in lines)
    return lines


def format_comment_as_text(text, indent):
    return "".join(
        [f"{line}\n" for line in format_comment_as_lines(text, indent)])


def format_comment_as_lines(text, indent):
    lines = textwrap.wrap(text.strip("\n"), width=77 - len(indent))
    lines = [f"{indent}// {line}" for line in lines]
    return lines


def log_stream_for_subprocess():
    stream = subprocess.DEVNULL
    for handler in logging.root.handlers:
        if handler.level == logging.DEBUG:
            if isinstance(handler, logging.StreamHandler):
                stream = handler.stream
    return stream


def main(argv):
    args_parser = argparse.ArgumentParser(
        description="Analyze a bootclasspath_fragment module.")
    args_parser.add_argument(
        "--bcpf",
        help="The bootclasspath_fragment module to analyze",
        required=True,
    )
    args_parser.add_argument(
        "--apex",
        help="The apex module to which the bootclasspath_fragment belongs. It "
        "is not strictly necessary at the moment but providing it will "
        "allow this script to give more useful messages and it may be"
        "required in future.",
        default="SPECIFY-APEX-OPTION")
    args_parser.add_argument(
        "--sdk",
        help="The sdk module to which the bootclasspath_fragment belongs. It "
        "is not strictly necessary at the moment but providing it will "
        "allow this script to give more useful messages and it may be"
        "required in future.",
        default="SPECIFY-SDK-OPTION")
    args_parser.add_argument(
        "--fix",
        help="Attempt to fix any issues found automatically.",
        action="store_true",
        default=False)
    args = args_parser.parse_args(argv[1:])
    top_dir = os.environ["ANDROID_BUILD_TOP"] + "/"
    out_dir = os.environ.get("OUT_DIR", os.path.join(top_dir, "out"))
    product_out_dir = os.environ.get("ANDROID_PRODUCT_OUT", top_dir)
    # Make product_out_dir relative to the top so it can be used as part of a
    # build target.
    product_out_dir = product_out_dir.removeprefix(top_dir)
    log_fd, abs_log_file = tempfile.mkstemp(
        suffix="_analyze_bcpf.log", text=True)

    with os.fdopen(log_fd, "w") as log_file:
        # Set up debug logging to the log file.
        logging.basicConfig(
            level=logging.DEBUG,
            format="%(levelname)-8s %(message)s",
            stream=log_file)

        # define a Handler which writes INFO messages or higher to the
        # sys.stdout with just the message.
        console = logging.StreamHandler()
        console.setLevel(logging.INFO)
        console.setFormatter(logging.Formatter("%(message)s"))
        # add the handler to the root logger
        logging.getLogger("").addHandler(console)

        print(f"Writing log to {abs_log_file}")
        try:
            analyzer = BcpfAnalyzer(
                tool_path=argv[0],
                top_dir=top_dir,
                out_dir=out_dir,
                product_out_dir=product_out_dir,
                bcpf=args.bcpf,
                apex=args.apex,
                sdk=args.sdk,
                fix=args.fix,
            )
            analyzer.analyze()
        finally:
            print(f"Log written to {abs_log_file}")


if __name__ == "__main__":
    main(sys.argv)
