#!/usr/bin/env python3
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

import hashlib
import logging
import os
import subprocess
import tempfile
import unittest

import google.protobuf.text_format as text_format
import provenance_metadata_pb2

logger = logging.getLogger(__name__)

def run(args, verbose=None, **kwargs):
  """Creates and returns a subprocess.Popen object.

  Args:
    args: The command represented as a list of strings.
    verbose: Whether the commands should be shown. Default to the global
        verbosity if unspecified.
    kwargs: Any additional args to be passed to subprocess.Popen(), such as env,
        stdin, etc. stdout and stderr will default to subprocess.PIPE and
        subprocess.STDOUT respectively unless caller specifies any of them.
        universal_newlines will default to True, as most of the users in
        releasetools expect string output.

  Returns:
    A subprocess.Popen object.
  """
  if 'stdout' not in kwargs and 'stderr' not in kwargs:
    kwargs['stdout'] = subprocess.PIPE
    kwargs['stderr'] = subprocess.STDOUT
  if 'universal_newlines' not in kwargs:
    kwargs['universal_newlines'] = True
  if verbose:
    logger.info("  Running: \"%s\"", " ".join(args))
  return subprocess.Popen(args, **kwargs)


def run_and_check_output(args, verbose=None, **kwargs):
  """Runs the given command and returns the output.

  Args:
    args: The command represented as a list of strings.
    verbose: Whether the commands should be shown. Default to the global
        verbosity if unspecified.
    kwargs: Any additional args to be passed to subprocess.Popen(), such as env,
        stdin, etc. stdout and stderr will default to subprocess.PIPE and
        subprocess.STDOUT respectively unless caller specifies any of them.

  Returns:
    The output string.

  Raises:
    ExternalError: On non-zero exit from the command.
  """
  proc = run(args, verbose=verbose, **kwargs)
  output, _ = proc.communicate()
  if output is None:
    output = ""
  if verbose:
    logger.info("%s", output.rstrip())
  if proc.returncode != 0:
    raise RuntimeError(
        "Failed to run command '{}' (exit code {}):\n{}".format(
            args, proc.returncode, output))
  return output

def run_host_command(args, verbose=None, **kwargs):
  host_build_top = os.environ.get("ANDROID_BUILD_TOP")
  if host_build_top:
    host_command_dir = os.path.join(host_build_top, "out/host/linux-x86/bin")
    args[0] = os.path.join(host_command_dir, args[0])
  return run_and_check_output(args, verbose, **kwargs)

def sha256(s):
  h = hashlib.sha256()
  h.update(bytearray(s, 'utf-8'))
  return h.hexdigest()

class ProvenanceMetaDataToolTest(unittest.TestCase):

  def test_gen_provenance_metadata(self):
    artifact_content = "test artifact"
    artifact_file = tempfile.mktemp()
    with open(artifact_file,"wt") as f:
      f.write(artifact_content)
    metadata_file = tempfile.mktemp()
    cmd = ["gen_provenance_metadata"]
    cmd.extend(["--module_name", "a"])
    cmd.extend(["--artifact_path", artifact_file])
    cmd.extend(["--install_path", "b"])
    cmd.extend(["--metadata_path", metadata_file])
    output = run_host_command(cmd)
    self.assertEqual(output, "")

    with open(metadata_file,"rt") as f:
      data = f.read()
      provenance_metadata = provenance_metadata_pb2.ProvenanceMetadata()
      text_format.Parse(data, provenance_metadata)
      self.assertEqual(provenance_metadata.module_name, "a")
      self.assertEqual(provenance_metadata.artifact_path, artifact_file)
      self.assertEqual(provenance_metadata.artifact_install_path, "b")
      self.assertEqual(provenance_metadata.artifact_sha256, sha256(artifact_content))

    os.remove(artifact_file)
    os.remove(metadata_file)

if __name__ == '__main__':
  unittest.main(verbosity=2)