#!/bin/bash

set -e

ARCHS=("x86_64" "aarch64")
OUTPUT_DIR="packages/archlinux"

mkdir -p "$OUTPUT_DIR"

for ARCH in "${ARCHS[@]}"; do
    echo "Packaging for Arch Linux ($ARCH)..."
    cd build/archlinux
    CARCH="$ARCH" makepkg --syncdeps --clean --force --noconfirm
    mv *.pkg.tar.zst "../../$OUTPUT_DIR/"
    cd ../../
done

echo "Arch Linux packages are in the $OUTPUT_DIR directory."
