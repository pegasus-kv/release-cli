package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v28/github"
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
		tagObj, _ := repo.TagObject(tag.Hash())
		commit, _ := tagObj.Commit()
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
		fmt.Printf("info: start scanning from commit \"%s\"\n", strings.TrimSpace(commit.Message))
		fmt.Printf("info: commit-sha is %s in %s, %s in master branch\n",
			commit.Hash.String()[:10], releaseBranch, cpCommit.Hash.String()[:10])

		// the committed number in release branch is limited, no worry for OOM
		checkoutBranch(repoArg, releaseBranch)
		commitsMap := make(map[string]*gitobj.Commit)
		iter, _ := repo.Log(&git.LogOptions{})
		iter.ForEach(func(c *gitobj.Commit) error {
			if is, _ := c.IsAncestor(commit); is {
				return gitstorer.ErrStop
			}
			commitsMap[c.Message] = c
			return nil
		})

		// obtain the official owner and name of this repo
		origin, err := repo.Remote("origin")
		fatalExitIfNotNil(err)
		owner, repoName := getOwnerAndRepoFromURL(origin.Config().URLs[0])

		// find the counterpart in master branch, if not, print it in the table
		checkoutBranch(repoArg, "master")
		iter, _ = repo.Log(&git.LogOptions{})
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"PR", "Title", "Days after commit"})
		table.SetBorder(false)
		table.SetColWidth(60)
		client := github.NewClient(nil)
		iter.ForEach(func(c *gitobj.Commit) error {
			if commitsMap[c.Message] != nil {
				return nil
			}
			if is, _ := c.IsAncestor(cpCommit); is {
				return gitstorer.ErrStop
			}
			ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
			prs, _, err := client.PullRequests.ListPullRequestsWithCommit(ctx, owner, repoName, c.Hash.String(), nil)
			fatalExitIfNotNil(err)
			if len(prs) != 1 {
				fatalExit(fatalError("multiple pull requests associated with commit %s", c))
			}
			pr := prs[0]
			daysAfterMerged := time.Now().Sub(pr.GetMergedAt()).Round(24 * time.Hour)
			table.Append([]string{getPrName(owner, repoName, int(pr.GetID())), pr.GetTitle(), daysAfterMerged.String()})
			return nil
		})
		table.Render()
		return nil
	},
}

func hasEqualCommitInRepo(repo *git.Repository, commit *gitobj.Commit) (cpCommit *gitobj.Commit, result bool) {
	iter, err := repo.Log(&git.LogOptions{Order: git.LogOrderCommitterTime})
	if err != nil {
		fatalExit(fatalError("unable perform git log"))
	}

	result = false
	iter.ForEach(func(c *gitobj.Commit) error {
		if strings.Compare(c.Message, commit.Message) == 0 {
			result = true
			cpCommit = c // find the counterpart
			return gitstorer.ErrStop
		}
		return nil
	})
	return
}
