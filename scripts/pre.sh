#!/usr/bin/env bash

# manually runnable pre-commit script

go test -v ./internal/...
gofmt -w .

