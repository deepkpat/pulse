#!/bin/bash
set -e

GO_VERSION="1.26.2"
GO_TARBALL="go${GO_VERSION}.linux-amd64.tar.gz"
GO_URL="https://golang.org/dl/${GO_TARBALL}"
GO_DIST=".go_dist"

echo "running environment setup..."

# cleanup old artifacts if they exist (old directory name)
if [ -d "go_dist" ]; then
    echo "found legacy go_dist directory. removing..."
    rm -rf go_dist
fi

if [ ! -d "${GO_DIST}" ]; then
    echo "downloading go ${GO_VERSION}..."
    if command -v wget >/dev/null 2>&1; then
        wget -q "${GO_URL}"
    elif command -v curl >/dev/null 2>&1; then
        curl -sSOL "${GO_URL}"
    else
        echo "no network utility found. trying python fallback..."
        python3 -c "import urllib.request; urllib.request.urlretrieve('${GO_URL}', '${GO_TARBALL}')"
    fi

    mkdir -p "${GO_DIST}"
    echo "extracting go tarball..."
    tar -xzf "${GO_TARBALL}" -C "${GO_DIST}" --strip-components=1
    rm "${GO_TARBALL}"
else
    echo "cached go binary distribution found in ${GO_DIST}"
fi

echo "setup complete"
