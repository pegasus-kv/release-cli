package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v28/github"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	"golang.org/x/oauth2"
	git "gopkg.in/src-d/go-git.v4"
	gitobj "gopkg.in/src-d/go-git.v4/plumbing/object"
	gitstorer "gopkg.in/src-d/go-git.v4/plumbing/storer"
)

// ./release-cli submit
var submitCommand *cli.Command = &cli.Command{
	Name:  "submit",
	Usage: "To submit the pull requests to the given release branch",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "repo",
			Usage: "The path where the git repository locates",
		},
		&cli.StringFlag{
			Name:  "version",
			Usage: "The new release version to submit",
		},
		&cli.StringFlag{
			Name:   "access",
			Usage:  "The access token to github, see https://github.com/settings/tokens",
			EnvVar: "ACCESS_TOKEN",
		},
	},
	Action: func(c *cli.Context) error {
		if c.NumFlags() == 0 {
			// show help if no flags given
			cli.ShowCommandHelp(c, "submit")
			return nil
		}

		var err error
		var repo *git.Repository

		// validate --repo
		repoArg := c.String("repo")
		if len(repoArg) == 0 {
			return fatalError("--repo is required")
		}
		if repo, err = git.PlainOpen(repoArg); err != nil {
			return fatalError("cannot open repo '%s': %s", repoArg, err)
		}

		// validate --version
		versionArg := c.String("version")
		if len(versionArg) == 0 {
			return fatalError("--version is required")
		}
		parts := strings.Split(versionArg, ".")
		if len(parts) != 3 {
			return fatalError("invalid version: %s", versionArg)
		}

		// validate --access
		accessToken := c.String("access")
		if len(accessToken) == 0 {
			return fatalError("--access is required")
		}

		patchVer, err := strconv.Atoi(parts[2])
		lastestVer := fmt.Sprintf("v%s.%s.%d", parts[0], parts[1], patchVer-1)
		lastestVerCommit := getCommitForTag(repo, lastestVer)
		releaseBranch := fmt.Sprintf("v%s.%s", parts[0], parts[1])
		releaseBranchRaw := fmt.Sprintf("%s.%s", parts[0], parts[1])

		checkoutBranch(repoArg, releaseBranch)
		iter, _ := repo.Log(&git.LogOptions{})
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"PR", "Title"})
		table.SetBorder(false)
		table.SetColWidth(120)
		var prs []int
		iter.ForEach(func(c *gitobj.Commit) error {
			commitTitle := getCommitTitle(c.Message)
			if is, _ := c.IsAncestor(lastestVerCommit); is {
				return gitstorer.ErrStop
			}
			prID, err := getPrIDInt(commitTitle)
			if err != nil {
				fmt.Printf("warn: %s, step back\n", err)
				return nil
			}
			table.Append([]string{fmt.Sprintf("#%d", prID), commitTitle})
			prs = append(prs, prID)
			return nil
		})
		fmt.Printf("info: submit %d commits to %s\n\n", len(prs), versionArg)
		table.Render()
		fmt.Println()

		origin, err := repo.Remote("origin")
		fatalExitIfNotNil(err)
		owner, repoName := getOwnerAndRepoFromURL(origin.Config().URLs[0])

		ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: accessToken},
		)
		tc := oauth2.NewClient(ctx, ts)
		client := github.NewClient(tc)

		// find existing label for version
		ctx, _ = context.WithTimeout(context.Background(), time.Second*3)
		_, resp, err := client.Issues.GetLabel(ctx, owner, repoName, versionArg)
		if err != nil {
			if resp.StatusCode == 404 {
				fmt.Printf("info: create github label %s\n", versionArg)
				_, _, err = client.Issues.CreateLabel(ctx, owner, repoName, &github.Label{Name: &versionArg})
			}
			fatalExitIfNotNil(err)
		}

		// Add release label to the specific PR
		for _, prID := range prs {
			ctx, _ := context.WithTimeout(context.Background(), time.Second*3)
			pr, _, err := client.PullRequests.Get(ctx, owner, repoName, prID)
			fatalExitIfNotNil(err)
			labelAdded := false
			for _, label := range pr.Labels {
				if strings.HasPrefix(label.GetName(), releaseBranchRaw) {
					fmt.Printf("info: #%d is already labeled to %s\n", prID, versionArg)
					labelAdded = true
					break
				}
			}
			if !labelAdded {
				_, _, err := client.Issues.AddLabelsToIssue(ctx, owner, repoName, prID, []string{versionArg})
				fatalExitIfNotNil(err)
				fmt.Printf("info: add github label %s to #%d\n", versionArg, prID)
			}
		}
		return nil
	},
}
