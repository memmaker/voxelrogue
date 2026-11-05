#!/bin/sh
# Build VoxelRogue with voxellib's WebGPU backend and run it; any
# arguments go to the game (e.g. ./build_n_run.sh -seed 42).
set -e
cd "$(dirname "$0")"
go build -tags wgpu -o voxelrogue .
# stderr (a Go panic's trace) goes to crash_logs/<time>.log, kept only when the game fails
mkdir -p crash_logs
log=crash_logs/$(date +%Y-%m-%d_%H-%M-%S)_$$.log
status=0
./voxelrogue "$@" 2>"$log" || status=$?
if [ $status -eq 0 ]; then rm -f "$log"; else echo "exited with status $status, stderr in $(pwd)/$log" >&2; fi
exit $status
