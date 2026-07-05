package main

import (
	"os"

	"github.com/jo-cube/pbl/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
