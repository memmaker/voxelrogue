#!/bin/sh
# Build VoxelRogue with voxellib's WebGPU backend and run it; any
# arguments go to the game (e.g. ./build_n_run.sh -seed 42).
set -e
cd "$(dirname "$0")"
go build -tags wgpu -o voxelrogue .
exec ./voxelrogue "$@"
