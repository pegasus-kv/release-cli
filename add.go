package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	git "gopkg.in/src-d/go-git.v4"
	gitobj "gopkg.in/src-d/go-git.v4/plumbing/object"
)

var repoArg = ""
var branchArg = ""

// ./release-cli add
var addCommand *cli.Command = &cli.Command{
	Name:  "add",
	Usage: "Specify the pull-requests to merge to release branch",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:        "repo",
			Usage:       "The path where the git repository locates. /home/pegasus eg.",
			Required:    true,
			Destination: &repoArg,
		},
		cli.StringFlag{
			Name:        "branch",
			Usage:       "The release branch for cherry-picks. v1.12 eg.",
			Required:    true,
			Destination: &branchArg,
		},
		cli.IntSliceFlag{
			Name:  "pr-list",
			Usage: "The pull-request IDs intended to be merged (233,266,257 format)",
		},
	},
	Action: func(c *cli.Context) error {
		var err error
		var repo *git.Repository
		if repo, err = git.PlainOpen(repoArg); err != nil {
			return fatalError("cannot open repo '%s': %s", repoArg, err)
		}
		// obtain the pull-requests to merge
		var prIDs []int
		for _, arg := range c.Args() {
			pr, err := strconv.Atoi(arg)
			if err != nil {
				return fatalError("invalid PR number '%s'", arg)
			}
			prIDs = append(prIDs, pr)
		}
		if len(prIDs) == 0 {
			return fatalError("no pull-request is specified")
		}

		// obtain the official owner and name of this repo
		origin, err := repo.Remote("origin")
		fatalExitIfNotNil(err)
		owner, repoName := getOwnerAndRepoFromURL(origin.Config().URLs[0])
		fmt.Printf("Cherry-picking PRs on '%s'...\n\n", origin.Config().URLs[0])

		// obtain the real commit id of the pull-requests
		checkoutBranch(repoArg, "master")
		var prs []*gitobj.Commit
		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"PR", "Commit SHA", "Title"})
		table.SetBorder(false)
		table.SetColWidth(60)
		for _, prID := range prIDs {
			commit, has := findCommitWithPRNumberInRepo(repo, prID)
			if !has {
				return fatalError("no such PR in the repo #%d", prID)
			}
			table.Append([]string{getPrName(owner, repoName, prID), commit.ID().String()[:10], getCommitTitle(commit.Message)})
			prs = append(prs, commit)
		}
		table.Render()
		fmt.Println()

		checkoutBranch(repoArg, branchArg)
		if err = cherryPickCommits(repo, prs); err != nil {
			return fatalError(err.Error())
		}
		return nil
	},
}

// cherry-pick the corresponding commits to the release branch
func cherryPickCommits(repo *git.Repository, prs []*gitobj.Commit) error {
	for _, pr := range prs {
		if _, found := findEqualCommitInRepo(repo, pr); found {
			fmt.Printf("ignore pull-request '%s' since it has been cherry-picked\n", getCommitTitle(pr.Message))
			continue
		}
		if err := executeCommand("cd %s; git cherry-pick %s", repoArg, pr.ID().String()); err != nil {
			return fmt.Errorf("unable to cherry pick [%s] \"%s\"\n%s", pr.ID().String()[:10], getCommitTitle(pr.Message), err)
		}
	}
	return nil
}

func findCommitWithPRNumberInRepo(repo *git.Repository, prNumber int) (*gitobj.Commit, bool) {
	return findCommitContainsStrInRepo(repo, fmt.Sprintf("(#%d)", prNumber))
}
