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

// var versionArg = ""
// var repoArg = ""
var accessToken = ""

// ./release-cli submit
var submitCommand *cli.Command = &cli.Command{
	Name:  "submit",
	Usage: "To submit the pull requests to the given release branch",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "repo",
			Usage:       "The path where the git repository locates",
			Required:    true,
			Destination: &repoArg,
		},
		&cli.StringFlag{
			Name:        "version",
			Usage:       "The new release version to submit",
			Required:    true,
			Destination: &versionArg,
		},
		&cli.StringFlag{
			Name:        "access",
			Usage:       "The access token to github, see https://github.com/settings/tokens",
			EnvVar:      "ACCESS_TOKEN",
			Required:    true,
			Destination: &accessToken,
		},
	},
	Action: func(c *cli.Context) error {
		var err error
		var repo *git.Repository

		// validate --repo
		if repo, err = git.PlainOpen(repoArg); err != nil {
			return fatalError("cannot open repo '%s': %s", repoArg, err)
		}

		// validate --version
		parts := strings.Split(versionArg, ".")
		if len(parts) != 3 {
			return fatalError("invalid version: %s", versionArg)
		}
		patchVer, err := strconv.Atoi(parts[2])
		if err != nil {
			return err
		}
		if patchVer == 0 {
			return fatalError("currently patch version == 0 is not supported")
		}

		releaseBranch := fmt.Sprintf("v%s.%s", parts[0], parts[1])
		releaseBranchRaw := fmt.Sprintf("%s.%s", parts[0], parts[1])
		lastestVer := getLatestVersionInReleaseBranch(repo, releaseBranch)
		lastestVerCommit := getCommitForTag(repo, lastestVer)

		checkoutBranch(repoArg, releaseBranch)
		iter, _ := repo.Log(&git.LogOptions{})
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"PR", "Title"})
		table.SetBorder(false)
		table.SetColWidth(120)
		var prs []int
		err = iter.ForEach(func(c *gitobj.Commit) error {
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
		if err != nil {
			return fatalError("unable to scan git log: %s", err)
		}
		fmt.Printf("info: submit %d commits to %s\n\n", len(prs), versionArg)
		table.Render()
		fmt.Println()

		origin, err := repo.Remote("origin")
		fatalExitIfNotNil(err)
		owner, repoName := getOwnerAndRepoFromURL(origin.Config().URLs[0])

		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		ts := oauth2.StaticTokenSource(
			&oauth2.Token{AccessToken: accessToken},
		)
		tc := oauth2.NewClient(ctx, ts)
		client := github.NewClient(tc)

		// find existing label for version
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
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
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
			defer cancel()
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
