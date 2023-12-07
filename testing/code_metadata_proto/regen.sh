#!/bin/bash

aprotoc --go_out=paths=source_relative:. code_metadata.proto
