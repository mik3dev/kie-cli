package main

import (
	"os"
	"time"

	"github.com/mik3dev/kie-cli/internal/cli"
)

func main() {
	os.Exit(cli.Execute(cli.Deps{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Stdin:  os.Stdin,
		Env:    os.Getenv,
		Now:    time.Now,
	}, os.Args[1:]))
}
