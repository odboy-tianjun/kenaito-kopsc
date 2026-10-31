// Command kopsc is a cross-platform command assistant: one intent, one
// platform-appropriate command.
package main

import (
	"os"

	"gitea.odboy.cn/odboy/kenaito-kopsc/internal/cli"
)

// version can be overridden at build time:
//
//	go build -ldflags "-X main.version=1.2.3" -o bin/kopsc ./cmd/kopsc
var version = "dev"

func main() {
	os.Exit(cli.New(version).Execute(os.Args[1:]))
}
