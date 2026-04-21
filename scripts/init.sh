#!/usr/bin/env bash

CFG_DIR="$HOME/.config/stormdrain"

mkdir -p $CFG_DIR/profiles
[[ -d example_profiles ]] && cp example_profiles/* $CFG_DIR/profiles/.
[[ -f Dockerfile.base ]] && cp Dockerfile.base $CFG_DIR/.

