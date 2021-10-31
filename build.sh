#!/usr/bin/env sh

go build -tags osusergo,netgo -ldflags='-extldflags=-static' .
