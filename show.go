package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	git "gopkg.in/src-d/go-git.v4"
	gitobj "gopkg.in/src-d/go-git.v4/plumbing/object"
	gitstorer "gopkg.in/src-d/go-git.v4/plumbing/storer"
)

// command flags
var includeReleased = true
var short = false
var versionArg = ""

// var repoArg = ""

// ./release-cli show
var showCommand *cli.Command = &cli.Command{
	Name:  "show",
	Usage: "To show the pull requests that are not released comparing to the given version",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "repo",
			Usage:       "The path where the git repository locates, ~/pegasus e.g",
			Required:    true,
			Destination: &repoArg,
		},
		&cli.StringFlag{
			Name:        "version",
			Usage:       "The released version to compare, 1.12 e.g",
			Required:    true,
			Destination: &versionArg,
		},
		&cli.BoolFlag{
			Name:        "short",
			Usage:       "Print PR ID and title only",
			Destination: &short,
		},
		&cli.BoolFlag{
			Name:        "released",
			Usage:       "Show also the commits that are already merged in release branch",
			Destination: &includeReleased,
		},
	},
	Action: func(ctx *cli.Context) error {
		if ctx.NumFlags() == 0 {
			// show help if no flags given
			return cli.ShowCommandHelp(ctx, "show")
		}

		var err error
		var repo *git.Repository
		if repo, err = git.PlainOpen(repoArg); err != nil {
			return fatalError("cannot open repo '%s': %s", repoArg, err)
		}

		// validate --version
		parts := strings.Split(versionArg, ".")
		if len(parts) != 2 && len(parts) != 3 {
			return fatalError("invalid version: %s, version should have 2 or 3 parts, like \"1.11\" or \"1.12.1\"", versionArg)
		}

		// obtain the official owner and name of this repo
		origin, err := repo.Remote("origin")
		fatalExitIfNotNil(err)
		owner, repoName := getOwnerAndRepoFromURL(origin.Config().URLs[0])

		if len(parts) == 3 {
			fmt.Printf("If the given version is released before, please find the commits here: https://github.com/%s/%s/labels/%s\n", owner, repoName, versionArg)
			return nil
		}

		// Find the initial commit in current minor version, and find the commits
		// afterwards in master branch.
		// The unreleased commits are which in the master branch, but not in the
		// minor version branch.

		releaseBranch := fmt.Sprintf("v%s.%s", parts[0], parts[1])
		initialVer := fmt.Sprintf("v%s.%s.0", parts[0], parts[1])
		initialCommit := getCommitForTag(repo, initialVer)

		checkoutBranch(repoArg, "master")
		cpCommit, has := findEqualCommitInRepo(repo, initialCommit)
		tryTimes := 0
		for !has {
			// trace back to the first commit of the release branch: `initialCommit`, this is where the master branch
			// and the release branch are diverged.

			fmt.Printf("info: commit \"%s\" in branch %s has not counterpart in master, step back\n",
				strings.TrimSpace(initialCommit.Message), releaseBranch)
			parent, err := initialCommit.Parent(0)
			if err != nil {
				return fatalError("unable to find parent for commit: %s", initialCommit.Hash)
			}
			initialCommit = parent
			if tryTimes++; tryTimes > 10 {
				return fatalError("stop. unable to find the equal commits both in master and %s", releaseBranch)
			}
			cpCommit, has = findEqualCommitInRepo(repo, initialCommit)
		}

		// the committed number in release branch is limited, no worry for OOM
		checkoutBranch(repoArg, releaseBranch)
		commitsMap := make(map[string]*gitobj.Commit)
		iter, _ := repo.Log(&git.LogOptions{})
		releasedCount := 0
		fmt.Printf("info: start scanning %s branch from commit \"%s\"\n", releaseBranch, getCommitTitle(initialCommit.Message))
		err = iter.ForEach(func(c *gitobj.Commit) error {
			if is, _ := c.IsAncestor(initialCommit); is {
				return gitstorer.ErrStop
			}
			// find the released commits, which are the commits in release branch
			commitsMap[getCommitTitle(c.Message)] = c
			releasedCount++
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Printf("info: there are in total %d commits in %s branch\n", releasedCount, releaseBranch)

		// find the counterpart in master branch, if not, print it in the table
		checkoutBranch(repoArg, "master")
		// TODO(wutao1): compare the current commit revision with the origin.
		iter, _ = repo.Log(&git.LogOptions{})
		unreleasedCount := 0
		releasedCount = 0
		fmt.Printf("info: start scanning master branch\n")
		var tableBulk [][]string
		err = iter.ForEach(func(c *gitobj.Commit) error {
			if is, _ := c.IsAncestor(cpCommit); is {
				return gitstorer.ErrStop
			}
			commitTitle := getCommitTitle(c.Message)
			released := true
			if commitsMap[commitTitle] != nil {
				// this commit in master branch has counterpart in release branch, which means this commit is released
				if !includeReleased {
					// skip those that are released already
					return nil
				}
				releasedCount++
			} else {
				released = false
				unreleasedCount++
			}
			row := []string{
				fmt.Sprintf("%s/%s%s", owner, repoName, getPrID(commitTitle)),
				commitTitle[:strings.LastIndex(commitTitle, "(")]} // drop the PrID part, because the PR column has included
			if !short {
				daysAfterMerged := time.Since(c.Committer.When).Hours() / 24
				row = append(row, fmt.Sprintf("%.2f", daysAfterMerged))
				if includeReleased && released {
					row = append(row, "RELEASED")
				}
			}
			tableBulk = append(tableBulk, row)
			return nil
		})
		if err != nil {
			return err
		}

		printTable(tableBulk, unreleasedCount, releasedCount)
		return nil
	},
}

func printTable(tableBulk [][]string, unreleasedCount, releasedCount int) {
	table := tablewriter.NewWriter(os.Stdout)
	var header []string
	if !includeReleased {
		header = []string{fmt.Sprintf("PR (%d TOTAL)", unreleasedCount), "TITLE"}
	} else {
		header = []string{fmt.Sprintf("PR (%d RELEASED, %d TOTAL)", releasedCount, unreleasedCount+releasedCount), "TITLE"}
	}
	if !short { // print other details
		header = append(header, "Days after commit")
	}
	fmt.Println()
	table.SetHeader(header)
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetColWidth(120)
	table.SetCenterSeparator("|")
	table.AppendBulk(tableBulk)
	table.Render()
	fmt.Println()
}
