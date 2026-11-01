#!/bin/sh
set -e

# 构建 kopsc 并安装到 /usr/local/bin，供本机直接使用。
cd "$(dirname "$0")"

go build -o bin/kopsc ./cmd/kopsc
mv ./bin/kopsc /usr/local/bin
chmod +x /usr/local/bin/kopsc

echo "已安装: /usr/local/bin/kopsc"
