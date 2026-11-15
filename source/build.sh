#!/bin/sh
# Cross-compile the Windows exes (needs Go 1.21+; 64-bit, Windows 10+). Run  python3 genrsrc.py  first if the icon/version changed.
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o SWTLUpdater.exe .
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o swtlpublish.exe ./cmd/swtlpublish
