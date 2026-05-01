#!/usr/bin/env bash

[[ ! -d "cmd" ]] && echo "run from project root" && exit 1

mkdir -p bin
go build -o bin/stormdrain cmd/stormdrain/stormdrain.go
go build -o bin/stormdrain_attach cmd/stormdrain_attach/stormdrain_attach.go

