package main

import (
	"fmt"
	"os"
	"strings"
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

func isCurrentBranch(repo string, branch string) bool {
	curBranch, err := executeCommandAndGet("cd %s; git rev-parse --abbrev-ref HEAD", repo)
	fatalExitIfNotNil(err)
	curBranch = strings.TrimSpace(curBranch)
	return strings.Compare(curBranch, branch) == 0
}
