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

import argparse
import hashlib
import os.path
import sys

import google.protobuf.text_format as text_format
import provenance_metadata_pb2

def Log(*info):
  if args.verbose:
    for i in info:
      print(i)

def ParseArgs(argv):
  parser = argparse.ArgumentParser(description='Create provenance metadata for a prebuilt artifact')
  parser.add_argument('-v', '--verbose', action='store_true', help='Print more information in execution')
  parser.add_argument('--module_name', help='Module name', required=True)
  parser.add_argument('--artifact_path', help='Relative path of the prebuilt artifact in source tree', required=True)
  parser.add_argument('--install_path', help='Absolute path of the artifact in the filesystem images', required=True)
  parser.add_argument('--metadata_path', help='Path of the provenance metadata file created for the artifact', required=True)
  return parser.parse_args(argv)

def main(argv):
  global args
  args = ParseArgs(argv)
  Log("Args:", vars(args))

  provenance_metadata = provenance_metadata_pb2.ProvenanceMetadata()
  provenance_metadata.module_name = args.module_name
  provenance_metadata.artifact_path = args.artifact_path
  provenance_metadata.artifact_install_path = args.install_path

  Log("Generating SHA256 hash")
  h = hashlib.sha256()
  with open(args.artifact_path, "rb") as artifact_file:
    h.update(artifact_file.read())
  provenance_metadata.artifact_sha256 = h.hexdigest()

  Log("Check if there is attestation for the artifact")
  attestation_file_name = args.artifact_path + ".intoto.jsonl"
  if os.path.isfile(attestation_file_name):
    provenance_metadata.attestation_path = attestation_file_name

  text_proto = [
      "# proto-file: build/soong/provenance/proto/provenance_metadata.proto",
      "# proto-message: ProvenanceMetaData",
      "",
      text_format.MessageToString(provenance_metadata)
  ]
  with open(args.metadata_path, "wt") as metadata_file:
    file_content = "\n".join(text_proto)
    Log("Writing provenance metadata in textproto:", file_content)
    metadata_file.write(file_content)

if __name__ == '__main__':
  main(sys.argv[1:])
