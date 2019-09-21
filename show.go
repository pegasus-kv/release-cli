package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	git "gopkg.in/src-d/go-git.v4"
	gitobj "gopkg.in/src-d/go-git.v4/plumbing/object"
	gitstorer "gopkg.in/src-d/go-git.v4/plumbing/storer"
	"gopkg.in/urfave/cli.v2"
)

// ./release-cli show
var showCommand *cli.Command = &cli.Command{
	Name:  "show",
	Usage: "To show the pull requests that are not released comparing to the given version",
	Flags: []cli.Flag{
		&cli.PathFlag{
			Name:  "repo",
			Usage: "The path where the git repository locates",
		},
		&cli.StringFlag{
			Name:  "version",
			Usage: "The released version to compare",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NumFlags() == 0 {
			// show help if no flags given
			cli.ShowCommandHelp(c, "show")
			return nil
		}

		var err error
		var repo *git.Repository

		repoArg := c.Path("repo")
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
		if len(parts) != 3 {
			return fatalError("invalid version: %s", versionArg)
		}

		// Find the initial commit in current minor version, and find the commits
		// afterwards in master branch.
		// The unreleased commits are which in the master branch, but not in the
		// minor version branch.

		releaseBranch := fmt.Sprintf("v%s.%s", parts[0], parts[1])
		initialVer := fmt.Sprintf("v%s.%s.0", parts[0], parts[1])
		tag, err := repo.Tag(initialVer)
		if err != nil {
			return fatalError("no such version tag: %s", initialVer)
		}
		tagObj, err := repo.TagObject(tag.Hash())
		var commit *gitobj.Commit
		if err != nil {
			fmt.Printf("warn: tag %s is possibly a lightweight tag, not an annotated tag\n", initialVer)
			commit, err = repo.CommitObject(tag.Hash())
			fatalExitIfNotNil(err)
		} else {
			commit, err = tagObj.Commit()
			fatalExitIfNotNil(err)
		}
		checkoutBranch(repoArg, "master")
		cpCommit, has := hasEqualCommitInRepo(repo, commit)
		tryTimes := 0
		for !has {
			fmt.Printf("info: commit \"%s\" does not appear in master, use its parent instead\n",
				strings.TrimSpace(commit.Message))
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
		iter.ForEach(func(c *gitobj.Commit) error {
			if is, _ := c.IsAncestor(commit); is {
				return gitstorer.ErrStop
			}
			commitsMap[getCommitTitle(c.Message)] = c
			iterCount++
			return nil
		})
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
		table.SetHeader([]string{"PR", "Title", "Days after commit"})
		table.SetBorder(false)
		table.SetColWidth(120)
		iterCount = 0
		fmt.Printf("info: start scanning master branch\n")
		iter.ForEach(func(c *gitobj.Commit) error {
			commitTitle := getCommitTitle(c.Message)
			if commitsMap[commitTitle] != nil {
				return nil
			}
			if is, _ := c.IsAncestor(cpCommit); is {
				return gitstorer.ErrStop
			}
			daysAfterMerged := time.Now().Sub(c.Committer.When).Hours() / 24
			table.Append([]string{
				fmt.Sprintf("%s/%s%s", owner, repoName, getPrID(commitTitle)),
				commitTitle,
				fmt.Sprintf("%.2f", daysAfterMerged)})
			iterCount++
			return nil
		})
		fmt.Printf("info: there are %d commits unmerged to %s branch\n\n", iterCount, releaseBranch)
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
	iter.ForEach(func(c *gitobj.Commit) error {
		if strings.Compare(getCommitTitle(c.Message), getCommitTitle(commit.Message)) == 0 {
			result = true
			cpCommit = c // find the counterpart
			return gitstorer.ErrStop
		}
		return nil
	})
	return
}
