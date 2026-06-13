---
title: "Installation"
description: "Install hn2 from a release, with go install, or from source."
weight: 20
---

## Prebuilt binaries

Every [release](https://github.com/tamnd/hackernoon-cli/releases) carries archives for Linux, macOS,
and Windows on amd64 and arm64, plus deb, rpm, and apk packages for Linux.
Download, unpack, put `hn2` on your `PATH`, done. The `checksums.txt`
on each release is signed with keyless [cosign](https://docs.sigstore.dev/) if
you want to verify before running.

## With Go

```bash
go install github.com/tamnd/hackernoon-cli/cmd/hn2@latest
```

That puts `hn2` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless
you moved it. Make sure that directory is on your `PATH`.

## From source

```bash
git clone https://github.com/tamnd/hackernoon-cli
cd hackernoon-cli
make build        # produces ./bin/hn2
./bin/hn2 version
```

## Container image

```bash
docker run --rm ghcr.io/tamnd/hn2:latest --help
```

## Checking the install

```bash
hn2 version
```

prints the version and exits.
