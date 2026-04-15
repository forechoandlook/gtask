package main

import (
	"context"
	"fmt"
	"os"

	"github.com/forechoandlook/gtask/internal/app"
	"github.com/forechoandlook/gtask/internal/version"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "version":
			fmt.Fprintln(os.Stdout, version.String())
			return
		}
	}
	if err := app.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
