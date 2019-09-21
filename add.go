package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/google/go-github/v28/github"
	"github.com/olekukonko/tablewriter"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/urfave/cli.v2"
)

// ./release-cli add
var addCommand *cli.Command = &cli.Command{
	Name:  "add",
	Usage: "Specify the pull-requests to merge to release branch",
	Flags: []cli.Flag{
		&cli.PathFlag{
			Name:  "repo",
			Usage: "The path where the git repository locates",
		},
		&cli.StringFlag{
			Name:  "branch",
			Usage: "The release branch for cherry-picks",
		},
		&cli.IntSliceFlag{
			Name:  "pr-list",
			Usage: "The pull-request IDs intended to be merged",
		},
		&cli.IntFlag{
			Name:  "pr",
			Usage: "The pull-request ID intended to be merged",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NumFlags() == 0 {
			// show help if no flags given
			cli.ShowCommandHelp(c, "add")
			return nil
		}

		var err error
		var repo *git.Repository

		pathArg := c.Path("repo")
		branchArg := c.String("branch")
		if len(pathArg) == 0 {
			return fatalError("--repo is required")
		}
		if len(branchArg) == 0 {
			return fatalError("--branch is required")
		}
		if repo, err = git.PlainOpen(pathArg); err != nil {
			return fatalError("cannot open repo '%s': %s", pathArg, err)
		}
		if _, err = repo.Branch(branchArg); err != nil {
			return fatalError("invalid branch '%s': %s", branchArg, err)
		}
		// obtain the pull-requests to merge
		var prIDs []int
		if c.IsSet("pr") {
			prIDs = append(prIDs, c.Int("pr"))
		}
		if c.IsSet("pr-list") {
			prIDs = append(prIDs, c.IntSlice("pr-list")...)
		}
		if len(prIDs) == 0 {
			return fatalError("no pull-request is specified")
		}
		sort.Ints(prIDs) // in ascending order

		// obtain the official owner and name of this repo
		origin, err := repo.Remote("origin")
		fatalExitIfNotNil(err)
		owner, repoName := getOwnerAndRepoFromURL(origin.Config().URLs[0])
		fmt.Printf("Making release on '%s'...\n\n", origin.Config().URLs[0])

		// obtain the real commit id of the pull-requests
		client := github.NewClient(nil)
		var prs []*github.PullRequest
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"PR", "Commit SHA", "Title"})
		table.SetBorder(false)
		table.SetColWidth(60)
		for _, prID := range prIDs {
			// TODO(wutao1): remove duplicates
			ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
			pr, _, err := client.PullRequests.Get(ctx, owner, repoName, prID)
			fatalExitIfNotNil(err)
			table.Append([]string{getPrName(owner, repoName, prID), pr.GetMergeCommitSHA()[:10], pr.GetTitle()})
			prs = append(prs, pr)
		}
		table.Render()
		fmt.Println()

		if err = cherryPickCommits(pathArg, branchArg, prs); err != nil {
			return err
		}
		return nil
	},
}

// cherry-pick the corresponding commits to the release branch
func cherryPickCommits(repo string, branch string, prs []*github.PullRequest) error {
	if !isCurrentBranch(repo, branch) {
		checkoutBranch(repo, branch)
	}
	for _, pr := range prs {
		if hasCommitInBranch(repo, branch, pr.GetMergeCommitSHA()) {
			fmt.Println("ignore pull-request '%d' since it has been cherry-picked", pr.GetID())
			continue
		}
		if err := executeCommand("cd %s; git cherry-pick %s", repo, pr.GetMergeCommitSHA()); err != nil {
			return err
		}
	}
	return nil
}

func hasCommitInBranch(repo string, branch string, commitID string) bool {
	output, err := executeCommandAndGet("cd %s; git branch --contains %s | grep -o %s", repo, commitID, branch)
	if err != nil && len(output) != 0 {
		fatalExit(err)
	}
	return err == nil
}
