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

// ./release-cli show
var showCommand *cli.Command = &cli.Command{
	Name:  "show",
	Usage: "To show the pull requests that are not released comparing to the given version",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Usage: "The path where the git repository locates",
		},
		&cli.StringFlag{
			Name:  "version",
			Usage: "The released version to compare",
		},
		&cli.BoolFlag{
			Name:  "short",
			Usage: "Print PR ID and title only",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NumFlags() == 0 {
			// show help if no flags given
			return cli.ShowCommandHelp(c, "show")
		}

		var err error
		var repo *git.Repository

		repoArg := c.String("repo")
		versionArg := c.String("version")

		// validate --repo
		if len(repoArg) == 0 {
			return fatalError("--repo is required")
		}
		if repo, err = git.PlainOpen(repoArg); err != nil {
			return fatalError("cannot open repo '%s': %s", repoArg, err)
		}

		// validate --version
		if len(versionArg) == 0 {
			return fatalError("--version is required")
		}
		parts := strings.Split(versionArg, ".")
		if len(parts) != 2 && len(parts) != 3 {
			return fatalError("invalid version: %s, version should have 2 or 3 parts, like \"1.11\" or \"1.12.1\"", versionArg)
		}
		if len(parts) == 2 {
			parts = append(parts, "0")
		}

		// Find the initial commit in current minor version, and find the commits
		// afterwards in master branch.
		// The unreleased commits are which in the master branch, but not in the
		// minor version branch.

		releaseBranch := fmt.Sprintf("v%s.%s", parts[0], parts[1])
		initialVer := fmt.Sprintf("v%s.%s.%s", parts[0], parts[1], parts[2])
		commit := getCommitForTag(repo, initialVer)
		checkoutBranch(repoArg, "master")
		cpCommit, has := hasEqualCommitInRepo(repo, commit)
		tryTimes := 0
		for !has {
			fmt.Printf("info: commit \"%s\" in branch %s has not counterpart in master, step back\n",
				strings.TrimSpace(commit.Message), releaseBranch)
			parent, err := commit.Parent(0)
			if err != nil {
				return fatalError("unable to find parent for commit: %s", commit.Hash)
			}
			commit = parent
			if tryTimes++; tryTimes > 10 {
				return fatalError("stop. unable to find the equal commits both in master and %s", releaseBranch)
			}
			cpCommit, has = hasEqualCommitInRepo(repo, commit)
		}

		// the committed number in release branch is limited, no worry for OOM
		checkoutBranch(repoArg, releaseBranch)
		commitsMap := make(map[string]*gitobj.Commit)
		iter, _ := repo.Log(&git.LogOptions{})
		iterCount := 0
		fmt.Printf("info: start scanning %s branch from commit \"%s\"\n", releaseBranch, getCommitTitle(commit.Message))
		err = iter.ForEach(func(c *gitobj.Commit) error {
			if is, _ := c.IsAncestor(commit); is {
				return gitstorer.ErrStop
			}
			commitsMap[getCommitTitle(c.Message)] = c
			iterCount++
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Printf("info: there are in total %d commits in %s branch\n", iterCount, releaseBranch)

		// obtain the official owner and name of this repo
		origin, err := repo.Remote("origin")
		fatalExitIfNotNil(err)
		owner, repoName := getOwnerAndRepoFromURL(origin.Config().URLs[0])

		// find the counterpart in master branch, if not, print it in the table
		checkoutBranch(repoArg, "master")
		// TODO(wutao1): compare the current commit revision with the origin.
		iter, _ = repo.Log(&git.LogOptions{})
		table := tablewriter.NewWriter(os.Stdout)
		short := c.Bool("short")
		header := []string{"PR", "Title"}
		if !short { // print other details
			header = append(header, "Days after commit")
		}
		table.SetHeader(header)
		table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
		table.SetColWidth(120)
		table.SetCenterSeparator("|")

		iterCount = 0
		fmt.Printf("info: start scanning master branch\n")
		var tableBulk [][]string
		err = iter.ForEach(func(c *gitobj.Commit) error {
			commitTitle := getCommitTitle(c.Message)
			if commitsMap[commitTitle] != nil {
				return nil
			}
			if is, _ := c.IsAncestor(cpCommit); is {
				return gitstorer.ErrStop
			}
			row := []string{
				fmt.Sprintf("%s/%s%s", owner, repoName, getPrID(commitTitle)),
				commitTitle[:strings.LastIndex(commitTitle, "(")]} // drop the PrID part, because the PR column has included
			if !short {
				daysAfterMerged := time.Since(c.Committer.When).Hours() / 24
				row = append(row, fmt.Sprintf("%.2f", daysAfterMerged))
			}
			tableBulk = append(tableBulk, row)
			iterCount++
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Printf("info: there are %d commits unmerged to %s branch\n\n", iterCount, releaseBranch)
		table.AppendBulk(tableBulk)
		table.Render()
		fmt.Println()
		return nil
	},
}

func hasEqualCommitInRepo(repo *git.Repository, commit *gitobj.Commit) (cpCommit *gitobj.Commit, result bool) {
	iter, err := repo.Log(&git.LogOptions{})
	if err != nil {
		fatalExit(fatalError("unable perform git log"))
	}

	result = false
	err = iter.ForEach(func(c *gitobj.Commit) error {
		if strings.Compare(getCommitTitle(c.Message), getCommitTitle(commit.Message)) == 0 {
			result = true
			cpCommit = c // find the counterpart
			return gitstorer.ErrStop
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return
}
