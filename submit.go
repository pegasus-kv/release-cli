package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v28/github"
	"github.com/hashicorp/go-version"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	"golang.org/x/oauth2"
	git "gopkg.in/src-d/go-git.v4"
)

// var repoArg = ""
var accessToken = ""

// ./release-cli submit
var submitCommand *cli.Command = &cli.Command{
	Name:  "submit",
	Usage: "To submit the pull requests to the given release branch",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "repo",
			Usage:       "The path where the git repository locates. /home/pegasus eg.",
			Required:    true,
			Destination: &repoArg,
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

		latestVer := getLatestVersion(repo)
		latestVerObj, err := version.NewSemver(latestVer)
		if err != nil {
			return fatalError("latest version is invalid to be released: %s, %s", latestVer, err)
		}
		if latestVerObj.Prerelease() != "" {
			return fatalError("repo is still in pre-released state: %s", latestVer)
		}

		versions := getAllVersions(repo, nil)
		sort.Sort(sort.Reverse(version.Collection(versions)))
		var pastReleasedVer *version.Version
		for _, v := range versions[1:] {
			if v.Prerelease() == "" {
				pastReleasedVer = v
				break
			}
		}
		infoLog("submitting PRs between %s and %s", pastReleasedVer, latestVer)

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"PR", "Title"})
		table.SetBorder(false)
		table.SetColWidth(120)
		var prs []int
		for _, c := range getAllCommitsPickedForUpcomingRelease(repo, pastReleasedVer.String()) {
			prID, err := getPrIDInt(c.title)
			if err != nil {
				warnLog("unable to get PR ID from commit \"%s\"", c.title)
				continue
			}
			table.Append([]string{fmt.Sprintf("#%d", prID), c.title})
			prs = append(prs, prID)
		}
		infoLog("submit %d commits to %s\n", len(prs), getLatestVersion(repo))
		table.Render()
		println()

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
		newLabel := latestVer[1:] // remove prefixed 'v'
		ctx, cancel = context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		_, resp, err := client.Issues.GetLabel(ctx, owner, repoName, newLabel)
		if err != nil {
			if resp.StatusCode == 404 {
				fmt.Printf("info: create github label %s\n", latestVer)
				_, _, err = client.Issues.CreateLabel(ctx, owner, repoName, &github.Label{Name: &newLabel})
			}
			fatalExitIfNotNil(err)
		}

		// Add release label to the specific PR
		releaseBranchRaw := getBranch(latestVer)[1:] // remove prefixed 'v'
		for _, prID := range prs {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
			defer cancel()
			pr, _, err := client.PullRequests.Get(ctx, owner, repoName, prID)
			fatalExitIfNotNil(err)
			labelAdded := false
			for _, label := range pr.Labels {
				if strings.HasPrefix(label.GetName(), releaseBranchRaw) {
					fmt.Printf("info: #%d is already labeled to %s\n", prID, newLabel)
					labelAdded = true
					break
				}
			}
			if !labelAdded {
				_, _, err := client.Issues.AddLabelsToIssue(ctx, owner, repoName, prID, []string{newLabel})
				fatalExitIfNotNil(err)
				fmt.Printf("info: add github label %s to #%d\n", latestVer, prID)
			}
		}
		return nil
	},
}
