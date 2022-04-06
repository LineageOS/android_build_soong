#!/usr/bin/env python
#
# Copyright (C) 2022 The Android Open Source Project
#
# Licensed under the Apache License, Version 2.0 (the 'License');
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an 'AS IS' BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Unit tests for analyzing bootclasspath_fragment modules."""
import os.path
import shutil
import tempfile
import unittest
import unittest.mock

import sys

import analyze_bcpf as ab

_FRAMEWORK_HIDDENAPI = "frameworks/base/boot/hiddenapi"
_MAX_TARGET_O = f"{_FRAMEWORK_HIDDENAPI}/hiddenapi-max-target-o.txt"
_MAX_TARGET_P = f"{_FRAMEWORK_HIDDENAPI}/hiddenapi-max-target-p.txt"
_MAX_TARGET_Q = f"{_FRAMEWORK_HIDDENAPI}/hiddenapi-max-target-q.txt"
_MAX_TARGET_R = f"{_FRAMEWORK_HIDDENAPI}/hiddenapi-max-target-r-loprio.txt"

_MULTI_LINE_COMMENT = """
Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut arcu justo,
bibendum eu malesuada vel, fringilla in odio. Etiam gravida ultricies sem
tincidunt luctus.""".replace("\n", " ").strip()


class FakeBuildOperation(ab.BuildOperation):

    def __init__(self, lines, return_code):
        ab.BuildOperation.__init__(self, None)
        self._lines = lines
        self.returncode = return_code

    def lines(self):
        return iter(self._lines)

    def wait(self, *args, **kwargs):
        return


class TestAnalyzeBcpf(unittest.TestCase):

    def setUp(self):
        # Create a temporary directory
        self.test_dir = tempfile.mkdtemp()

    def tearDown(self):
        # Remove the directory after the test
        shutil.rmtree(self.test_dir)

    @staticmethod
    def write_abs_file(abs_path, contents):
        os.makedirs(os.path.dirname(abs_path), exist_ok=True)
        with open(abs_path, "w", encoding="utf8") as f:
            print(contents.removeprefix("\n"), file=f, end="")

    def populate_fs(self, fs):
        for path, contents in fs.items():
            abs_path = os.path.join(self.test_dir, path)
            self.write_abs_file(abs_path, contents)

    def create_analyzer_for_test(self,
                                 fs=None,
                                 bcpf="bcpf",
                                 apex="apex",
                                 sdk="sdk",
                                 fix=False):
        if fs:
            self.populate_fs(fs)

        top_dir = self.test_dir
        out_dir = os.path.join(self.test_dir, "out")
        product_out_dir = "out/product"

        bcpf_dir = f"{bcpf}-dir"
        modules = {bcpf: {"path": [bcpf_dir]}}
        module_info = ab.ModuleInfo(modules)

        analyzer = ab.BcpfAnalyzer(
            tool_path=os.path.join(out_dir, "bin"),
            top_dir=top_dir,
            out_dir=out_dir,
            product_out_dir=product_out_dir,
            bcpf=bcpf,
            apex=apex,
            sdk=sdk,
            fix=fix,
            module_info=module_info,
        )
        analyzer.load_all_flags()
        return analyzer

    def test_reformat_report_text(self):
        lines = """
99. An item in a numbered list
that traverses multiple lines.

   An indented example
   that should not be reformatted.
"""
        reformatted = ab.BcpfAnalyzer.reformat_report_test(lines)
        self.assertEqual(
            """
99. An item in a numbered list that traverses multiple lines.

   An indented example
   that should not be reformatted.
""", reformatted)

    def do_test_build_flags(self, fix):
        lines = """
ERROR: Hidden API flags are inconsistent:
< out/soong/.intermediates/bcpf-dir/bcpf-dir/filtered-flags.csv
> out/soong/hiddenapi/hiddenapi-flags.csv

< Lacme/test/Class;-><init>()V,blocked
> Lacme/test/Class;-><init>()V,max-target-o

< Lacme/test/Other;->getThing()Z,blocked
> Lacme/test/Other;->getThing()Z,max-target-p

< Lacme/test/Widget;-><init()V,blocked
> Lacme/test/Widget;-><init()V,max-target-q

< Lacme/test/Gadget;->NAME:Ljava/lang/String;,blocked
> Lacme/test/Gadget;->NAME:Ljava/lang/String;,lo-prio,max-target-r
16:37:32 ninja failed with: exit status 1
""".strip().splitlines()
        operation = FakeBuildOperation(lines=lines, return_code=1)

        fs = {
            _MAX_TARGET_O:
                """
Lacme/items/Magnet;->size:I
Lacme/test/Class;-><init>()V
""",
            _MAX_TARGET_P:
                """
Lacme/items/Rocket;->size:I
Lacme/test/Other;->getThing()Z
""",
            _MAX_TARGET_Q:
                """
Lacme/items/Rock;->size:I
Lacme/test/Widget;-><init()V
""",
            _MAX_TARGET_R:
                """
Lacme/items/Lever;->size:I
Lacme/test/Gadget;->NAME:Ljava/lang/String;
""",
            "bcpf-dir/hiddenapi/hiddenapi-max-target-p.txt":
                """
Lacme/old/Class;->getWidget()Lacme/test/Widget;
""",
            "out/soong/.intermediates/bcpf-dir/bcpf/all-flags.csv":
                """
Lacme/test/Gadget;->NAME:Ljava/lang/String;,blocked
Lacme/test/Widget;-><init()V,blocked
Lacme/test/Class;-><init>()V,blocked
Lacme/test/Other;->getThing()Z,blocked
""",
        }

        analyzer = self.create_analyzer_for_test(fs, fix=fix)

        # Override the build_file_read_output() method to just return a fake
        # build operation.
        analyzer.build_file_read_output = unittest.mock.Mock(
            return_value=operation)

        # Override the run_command() method to do nothing.
        analyzer.run_command = unittest.mock.Mock()

        result = ab.Result()

        analyzer.build_monolithic_flags(result)
        expected_diffs = {
            "Lacme/test/Gadget;->NAME:Ljava/lang/String;":
                (["blocked"], ["lo-prio", "max-target-r"]),
            "Lacme/test/Widget;-><init()V": (["blocked"], ["max-target-q"]),
            "Lacme/test/Class;-><init>()V": (["blocked"], ["max-target-o"]),
            "Lacme/test/Other;->getThing()Z": (["blocked"], ["max-target-p"])
        }
        self.assertEqual(expected_diffs, result.diffs, msg="flag differences")

        expected_property_changes = [
            ab.HiddenApiPropertyChange(
                property_name="max_target_o_low_priority",
                values=["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],
                property_comment=""),
            ab.HiddenApiPropertyChange(
                property_name="max_target_p",
                values=["hiddenapi/hiddenapi-max-target-p.txt"],
                property_comment=""),
            ab.HiddenApiPropertyChange(
                property_name="max_target_q",
                values=["hiddenapi/hiddenapi-max-target-q.txt"],
                property_comment=""),
            ab.HiddenApiPropertyChange(
                property_name="max_target_r_low_priority",
                values=["hiddenapi/hiddenapi-max-target-r-low-priority.txt"],
                property_comment=""),
        ]
        self.assertEqual(
            expected_property_changes,
            result.property_changes,
            msg="property changes")

        return result

    def test_build_flags_report(self):
        result = self.do_test_build_flags(fix=False)

        expected_file_changes = [
            ab.FileChange(
                path="bcpf-dir/hiddenapi/"
                "hiddenapi-max-target-o-low-priority.txt",
                description="""Add the following entries:
            Lacme/test/Class;-><init>()V
""",
            ),
            ab.FileChange(
                path="bcpf-dir/hiddenapi/hiddenapi-max-target-p.txt",
                description="""Add the following entries:
            Lacme/test/Other;->getThing()Z
""",
            ),
            ab.FileChange(
                path="bcpf-dir/hiddenapi/hiddenapi-max-target-q.txt",
                description="""Add the following entries:
            Lacme/test/Widget;-><init()V
"""),
            ab.FileChange(
                path="bcpf-dir/hiddenapi/"
                "hiddenapi-max-target-r-low-priority.txt",
                description="""Add the following entries:
            Lacme/test/Gadget;->NAME:Ljava/lang/String;
"""),
            ab.FileChange(
                path="frameworks/base/boot/hiddenapi/"
                "hiddenapi-max-target-o.txt",
                description="""Remove the following entries:
            Lacme/test/Class;-><init>()V
"""),
            ab.FileChange(
                path="frameworks/base/boot/hiddenapi/"
                "hiddenapi-max-target-p.txt",
                description="""Remove the following entries:
            Lacme/test/Other;->getThing()Z
"""),
            ab.FileChange(
                path="frameworks/base/boot/hiddenapi/"
                "hiddenapi-max-target-q.txt",
                description="""Remove the following entries:
            Lacme/test/Widget;-><init()V
"""),
            ab.FileChange(
                path="frameworks/base/boot/hiddenapi/"
                "hiddenapi-max-target-r-loprio.txt",
                description="""Remove the following entries:
            Lacme/test/Gadget;->NAME:Ljava/lang/String;
""")
        ]
        result.file_changes.sort()
        self.assertEqual(
            expected_file_changes, result.file_changes, msg="file_changes")

    def test_build_flags_fix(self):
        result = self.do_test_build_flags(fix=True)

        expected_file_changes = [
            ab.FileChange(
                path="bcpf-dir/hiddenapi/"
                "hiddenapi-max-target-o-low-priority.txt",
                description="Created with 'bcpf' specific entries"),
            ab.FileChange(
                path="bcpf-dir/hiddenapi/hiddenapi-max-target-p.txt",
                description="Added 'bcpf' specific entries"),
            ab.FileChange(
                path="bcpf-dir/hiddenapi/hiddenapi-max-target-q.txt",
                description="Created with 'bcpf' specific entries"),
            ab.FileChange(
                path="bcpf-dir/hiddenapi/"
                "hiddenapi-max-target-r-low-priority.txt",
                description="Created with 'bcpf' specific entries"),
            ab.FileChange(
                path=_MAX_TARGET_O,
                description="Removed 'bcpf' specific entries"),
            ab.FileChange(
                path=_MAX_TARGET_P,
                description="Removed 'bcpf' specific entries"),
            ab.FileChange(
                path=_MAX_TARGET_Q,
                description="Removed 'bcpf' specific entries"),
            ab.FileChange(
                path=_MAX_TARGET_R,
                description="Removed 'bcpf' specific entries")
        ]

        result.file_changes.sort()
        self.assertEqual(
            expected_file_changes, result.file_changes, msg="file_changes")

        expected_file_contents = {
            "bcpf-dir/hiddenapi/hiddenapi-max-target-o-low-priority.txt":
                """
Lacme/test/Class;-><init>()V
""",
            "bcpf-dir/hiddenapi/hiddenapi-max-target-p.txt":
                """
Lacme/old/Class;->getWidget()Lacme/test/Widget;
Lacme/test/Other;->getThing()Z
""",
            "bcpf-dir/hiddenapi/hiddenapi-max-target-q.txt":
                """
Lacme/test/Widget;-><init()V
""",
            "bcpf-dir/hiddenapi/hiddenapi-max-target-r-low-priority.txt":
                """
Lacme/test/Gadget;->NAME:Ljava/lang/String;
""",
            _MAX_TARGET_O:
                """
Lacme/items/Magnet;->size:I
""",
            _MAX_TARGET_P:
                """
Lacme/items/Rocket;->size:I
""",
            _MAX_TARGET_Q:
                """
Lacme/items/Rock;->size:I
""",
            _MAX_TARGET_R:
                """
Lacme/items/Lever;->size:I
""",
        }
        for file_change in result.file_changes:
            path = file_change.path
            expected_contents = expected_file_contents[path].lstrip()
            abs_path = os.path.join(self.test_dir, path)
            with open(abs_path, "r", encoding="utf8") as tio:
                contents = tio.read()
                self.assertEqual(
                    expected_contents, contents, msg=f"{path} contents")

    def test_compute_hiddenapi_package_properties(self):
        fs = {
            "out/soong/.intermediates/bcpf-dir/bcpf/all-flags.csv":
                """
La/b/C;->m()V
La/b/c/D;->m()V
La/b/c/E;->m()V
Lb/c/D;->m()V
Lb/c/E;->m()V
Lb/c/d/E;->m()V
""",
            "out/soong/hiddenapi/hiddenapi-flags.csv":
                """
La/b/C;->m()V
La/b/D;->m()V
La/b/E;->m()V
La/b/c/D;->m()V
La/b/c/E;->m()V
La/b/c/d/E;->m()V
La/b/c/d/e/F;->m()V
Lb/c/D;->m()V
Lb/c/E;->m()V
Lb/c/d/E;->m()V
"""
        }
        analyzer = self.create_analyzer_for_test(fs)
        analyzer.load_all_flags()

        result = ab.Result()
        analyzer.compute_hiddenapi_package_properties(result)
        self.assertEqual(["a.b"], list(result.split_packages.keys()))

        reason = result.split_packages["a.b"]
        self.assertEqual(["a.b.C"], reason.bcpf)
        self.assertEqual(["a.b.D", "a.b.E"], reason.other)

        self.assertEqual(["a.b.c"], list(result.single_packages.keys()))

        reason = result.single_packages["a.b.c"]
        self.assertEqual(["a.b.c"], reason.bcpf)
        self.assertEqual(["a.b.c.d", "a.b.c.d.e"], reason.other)

        self.assertEqual(["b"], result.package_prefixes)


class TestHiddenApiPropertyChange(unittest.TestCase):

    def setUp(self):
        # Create a temporary directory
        self.test_dir = tempfile.mkdtemp()

    def tearDown(self):
        # Remove the directory after the test
        shutil.rmtree(self.test_dir)

    def check_change_fix(self, change, bpmodify_output, expected):
        file = os.path.join(self.test_dir, "Android.bp")

        with open(file, "w", encoding="utf8") as tio:
            tio.write(bpmodify_output.strip("\n"))

        bpmodify_runner = ab.BpModifyRunner(
            os.path.join(os.path.dirname(sys.argv[0]), "bpmodify"))
        change.fix_bp_file(file, "bcpf", bpmodify_runner)

        with open(file, "r", encoding="utf8") as tio:
            contents = tio.read()
            self.assertEqual(expected.lstrip("\n"), contents)

    def check_change_snippet(self, change, expected):
        snippet = change.snippet("        ")
        self.assertEqual(expected, snippet)

    def test_change_property_with_value_no_comment(self):
        change = ab.HiddenApiPropertyChange(
            property_name="split_packages",
            values=["android.provider"],
        )

        self.check_change_snippet(
            change, """
        split_packages: [
            "android.provider",
        ],
""")

        self.check_change_fix(
            change, """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],
        split_packages: [
            "android.provider",
        ],
    },
}
""", """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],
        split_packages: [
            "android.provider",
        ],
    },
}
""")

    def test_change_property_with_value_and_comment(self):
        change = ab.HiddenApiPropertyChange(
            property_name="split_packages",
            values=["android.provider"],
            property_comment=_MULTI_LINE_COMMENT,
        )

        self.check_change_snippet(
            change, """
        // Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut arcu
        // justo, bibendum eu malesuada vel, fringilla in odio. Etiam gravida
        // ultricies sem tincidunt luctus.
        split_packages: [
            "android.provider",
        ],
""")

        self.check_change_fix(
            change, """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],
        split_packages: [
            "android.provider",
        ],

        single_packages: [
            "android.system",
        ],

    },
}
""", """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],

        // Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut arcu
        // justo, bibendum eu malesuada vel, fringilla in odio. Etiam gravida
        // ultricies sem tincidunt luctus.
        split_packages: [
            "android.provider",
        ],

        single_packages: [
            "android.system",
        ],

    },
}
""")

    def test_set_property_with_value_and_comment(self):
        change = ab.HiddenApiPropertyChange(
            property_name="split_packages",
            values=["another.provider", "other.system"],
            property_comment=_MULTI_LINE_COMMENT,
            action=ab.PropertyChangeAction.REPLACE,
        )

        self.check_change_snippet(
            change, """
        // Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut arcu
        // justo, bibendum eu malesuada vel, fringilla in odio. Etiam gravida
        // ultricies sem tincidunt luctus.
        split_packages: [
            "another.provider",
            "other.system",
        ],
""")

        self.check_change_fix(
            change, """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],
        split_packages: [
            "another.provider",
            "other.system",
        ],
    },
}
""", """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],

        // Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut arcu
        // justo, bibendum eu malesuada vel, fringilla in odio. Etiam gravida
        // ultricies sem tincidunt luctus.
        split_packages: [
            "another.provider",
            "other.system",
        ],
    },
}
""")

    def test_set_property_with_no_value_or_comment(self):
        change = ab.HiddenApiPropertyChange(
            property_name="split_packages",
            values=[],
            action=ab.PropertyChangeAction.REPLACE,
        )

        self.check_change_snippet(change, """
        split_packages: [],
""")

        self.check_change_fix(
            change, """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],
        split_packages: [
            "another.provider",
            "other.system",
        ],
        package_prefixes: ["android.provider"],
    },
}
""", """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],
        split_packages: [],
        package_prefixes: ["android.provider"],
    },
}
""")

    def test_set_empty_property_with_no_value_or_comment(self):
        change = ab.HiddenApiPropertyChange(
            property_name="split_packages",
            values=[],
            action=ab.PropertyChangeAction.REPLACE,
        )

        self.check_change_snippet(change, """
        split_packages: [],
""")

        self.check_change_fix(
            change, """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],
        split_packages: [],
        package_prefixes: ["android.provider"],
    },
}
""", """
bootclasspath_fragment {
    name: "bcpf",

    // modified by the Soong or platform compat team.
    hidden_api: {
        max_target_o_low_priority: ["hiddenapi/hiddenapi-max-target-o-low-priority.txt"],
        split_packages: [],
        package_prefixes: ["android.provider"],
    },
}
""")


if __name__ == "__main__":
    unittest.main(verbosity=3)
