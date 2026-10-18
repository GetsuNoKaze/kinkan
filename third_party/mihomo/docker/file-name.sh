#!/bin/sh
set -eu

os="citrus-linux-"
case "$TARGETPLATFORM" in
    "linux/amd64")
        arch="amd64-v1"
        ;;
    "linux/386")
        arch="386"
        ;;
    "linux/arm64")
        arch="arm64"
        ;;
    "linux/arm/v7")
        arch="armv7"
        ;;
    "linux/riscv64")
        arch="riscv64"
        ;;
    *)
        echo "Unsupported platform: $TARGETPLATFORM" >&2
        exit 1
        ;;        
esac
file_name="$os$arch-$(cat bin/version.txt)"
printf '%s\n' "$file_name"
