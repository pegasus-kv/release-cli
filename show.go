package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	gitobj "gopkg.in/src-d/go-git.v4/plumbing/object"
	gitstorer "gopkg.in/src-d/go-git.v4/plumbing/storer"
)

// command flags
var includeReleased = true
var short = false
var versionArg = ""
var debug = false

// var repoArg = ""

// ./release-cli show
var showCommand *cli.Command = &cli.Command{
	Name:  "show",
	Usage: "To show the pull requests that are not released comparing to the given version",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:        "repo",
			Usage:       "The path where the git repository locates, '~/pegasus' e.g",
			Required:    true,
			Destination: &repoArg,
		},
		&cli.StringFlag{
			Name:        "version",
			Usage:       "The minor version to release, 1.12 e.g",
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
		&cli.BoolFlag{
			Name:        "debug",
			Usage:       "Put release-cli show in a debug mode.",
			Destination: &debug,
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

		if len(parts) == 3 && !includeReleased {
			fmt.Println("Use --released if you want to check what have changed after this version")
			fmt.Printf("If the given version is released before, please find the commits here: https://github.com/%s/%s/labels/%s\n", owner, repoName, versionArg)
			fmt.Println("NOTE: RC(Release Candidate) versions are not labelled.")
			return nil
		}

		// Find the initial commit of the minor version, and find the commits
		// afterwards in master branch.

		releaseBranch := fmt.Sprintf("v%s.%s", parts[0], parts[1])
		initialVer := fmt.Sprintf("v%s.%s.0", parts[0], parts[1])
		var startVer string
		if len(parts) == 3 { // && includeReleased
			startVer = "v" + versionArg
			infoLog("searching PRs that are committed to %s after version %s", releaseBranch, startVer)
		} else { // len(parts) == 2
			startVer = getLatestVersionInReleaseBranch(repo, releaseBranch)
			infoLog("searching PRs that are not released in %s", releaseBranch)
			infoLog("searching starts from the latest version in %s: %s", releaseBranch, startVer)
		}
		debugLog("will skip commits before version %s", startVer)
		checkoutBranch(repoArg, releaseBranch)
		initialCommit := getCommitForTag(repo, initialVer)

		checkoutBranch(repoArg, "master")
		masterInitialCommit, has := findEqualCommitInRepo(repo, initialCommit)
		tryTimes := 0
		for !has {
			// trace back to the first commit of the release branch: `initialCommit`, this is where the master branch
			// and the release branch are diverged.
			//
			// master    1.11
			// |          |
			// |/---------| => the diverged point
			// |

			debugLog("commit \"%s\" in branch %s has not counterpart in master, step back",
				strings.TrimSpace(initialCommit.Message), releaseBranch)
			parent, err := initialCommit.Parent(0)
			if err != nil {
				return fatalError("unable to find parent for commit: %s", initialCommit.Hash)
			}
			initialCommit = parent
			if tryTimes++; tryTimes > 10 {
				return fatalError("stop. unable to find the equal commits both in master and %s", releaseBranch)
			}
			masterInitialCommit, has = findEqualCommitInRepo(repo, initialCommit)
		}

		// the committed number in release branch is limited, no worry for OOM
		checkoutBranch(repoArg, releaseBranch)
		iter, _ := repo.Log(&git.LogOptions{})
		releasedCount := 0
		commitToVersionMap := make(map[string]string)
		versions := getAllVersionsInMinorVersion(repo, releaseBranch)
		debugLog("all versions after %s in minor version %s", startVer, releaseBranch)
		for _, ver := range versions {
			if strings.Compare(ver, startVer) >= 0 {
				debugLog("%s", ver)
			}
		}
		currentVersion := "" // by default empty, means this commit is not released in any version
		debugLog("start scanning %s branch from commit that's tagged %s \"%s\"", releaseBranch, initialVer, getCommitTitle(initialCommit.Message))
		err = iter.ForEach(func(c *gitobj.Commit) error {
			if is, _ := c.IsAncestor(initialCommit); is {
				return gitstorer.ErrStop
			}
			commitTitle := getCommitTitle(c.Message)
			if ver, ok := versions[commitTitle]; ok {
				currentVersion = ver
			}
			commitToVersionMap[commitTitle] = currentVersion
			if _, err := getPrIDInt(commitTitle); err != nil {
				warnLog("ignore invalid commit %s: \"%s\"", c.ID().String()[:10], commitTitle)
				return nil
			}
			releasedCount++
			return nil
		})
		if err != nil {
			return err
		}
		infoLog("there are in total %d commits after %s", releasedCount, initialVer)

		// find the counterpart in master branch, if not, print it in the table
		checkoutBranch(repoArg, "master")
		// TODO(wutao1): compare the current commit revision with the origin.
		iter, _ = repo.Log(&git.LogOptions{})
		unreleasedCount := 0
		releasedCount = 0
		debugLog("start scanning master branch")
		var tableBulk [][]string
		err = iter.ForEach(func(c *gitobj.Commit) error {
			commitTitle := getCommitTitle(c.Message)
			if is, _ := c.IsAncestor(masterInitialCommit); is {
				debugLog("hit the diverged point of master branch and %s branch: %s", releaseBranch, commitTitle)
				return gitstorer.ErrStop
			}
			if _, err := getPrIDInt(commitTitle); err != nil {
				warnLog("ignore invalid commit %s: \"%s\"", c.ID().String()[:10], commitTitle)
				return nil
			}
			ver, ok := commitToVersionMap[commitTitle]
			if ver != "" && strings.Compare(startVer, ver) > 0 {
				// has released in versions lower than --version
				debugLog("skip \"%s\" of version %s", commitTitle, ver)
				return nil
			}
			released := true
			if ok {
				if ver != "" && !includeReleased {
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
					row = append(row, ver)
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

func getLatestVersionInReleaseBranch(repo *git.Repository, releaseBranch string) string {
	versions := getAllVersionsInMinorVersion(repo, releaseBranch)
	latestVer := ""
	for _, ver := range versions {
		if strings.Compare(ver, latestVer) > 0 {
			latestVer = ver
		}
	}
	return latestVer
}

func getAllVersionsInMinorVersion(repo *git.Repository, releaseBranch string) map[string]string {
	tagIter, err := repo.Tags()
	if err != nil {
		fatalExit(fatalError("unable to get tags in release branch: %s", releaseBranch))
	}

	versions := make(map[string]string)
	err = tagIter.ForEach(func(ref *plumbing.Reference) error {
		ver := strings.TrimPrefix(ref.Name().String(), "refs/tags/")
		if strings.HasPrefix(ver, releaseBranch) {
			commit, err := repo.CommitObject(ref.Hash())
			if err != nil {
				warnLog("unable to find commit for tag %s: %s", ver, err)
				return nil
			}
			versions[getCommitTitle(commit.Message)] = ver
		}
		return nil
	})
	fatalExitIfNotNil(err)
	return versions
}
