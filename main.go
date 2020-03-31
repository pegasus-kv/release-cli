package main

import (
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli"
)

func main() {
	app := &cli.App{
		Name:  "release-cli",
		Usage: "Release in Pegasus's convention",
		Commands: []cli.Command{
			*addCommand,
			*showCommand,
			*submitCommand,
		},
		Action: func(c *cli.Context) error {
			return cli.ShowAppHelp(c)
		},
		Compiled:    time.Now(),
		HideVersion: true,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
