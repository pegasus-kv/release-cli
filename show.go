package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	gitobj "gopkg.in/src-d/go-git.v4/plumbing/object"
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
		var err error
		var repo *git.Repository
		if repo, err = git.PlainOpen(repoArg); err != nil {
			return fatalError("cannot open repo '%s': %s", repoArg, err)
		}

		// obtain the official owner and name of this repo
		origin, err := repo.Remote("origin")
		fatalExitIfNotNil(err)
		owner, repoName := getOwnerAndRepoFromURL(origin.Config().URLs[0])

		latestReleasedVer := getLatestReleasedVersion(repo)
		parts := strings.Split(latestReleasedVer.Original(), ".")
		releaseBranch := fmt.Sprintf("%s.%s", parts[0], parts[1])
		infoLog("searching starts from the latest released version in %s", latestReleasedVer.Original())

		// Find the initial commit of the minor version, and find the commits
		// afterwards in master branch.

		divergedVer := getInitialVersionInReleaseBranch(repo, releaseBranch) // the version where master and release branch are diverged.
		divergedCommit := getCommitForTag(repo, divergedVer)
		infoLog("the diverged point of master and %s is %s: %s", releaseBranch, divergedVer, divergedCommit.ID().String()[:10])
		checkoutBranch(repoArg, releaseBranch)
		commitToVersionMap := make(map[string]string)
		if latestVer := getLatestVersion(repo); !strings.HasPrefix(latestVer, releaseBranch) {
			// If it's an upcoming minor release (branched from master).

			// The commits between the two diverged points (1.11.0-RC1, 1.12.0-RC1 e.g)
			// but not in the previous release branch,
			// are certainly in the new minor version (1.12.0).
			//
			// |/
			// |------- 1.12.0-RC1
			// |
			// | /----- 1.11.6
			// |/
			// |------- 1.11.0-RC1
			// |

			// Firstly tag all the commits in the previous release branch (1.11 in the above example)
			forEachGitLogUntil(repo, func(c *gitobj.Commit) {
				// tag commits between 1.11.0-RC1 ~ 1.11.6 to "v1.11"
				commitToVersionMap[getCommitTitle(c.Message)] = releaseBranch
			}, divergedCommit)

			parts = strings.Split(latestVer, ".")
			releaseBranch = fmt.Sprintf("%s.%s", parts[0], parts[1])
			newDivergedVer := getInitialVersionInReleaseBranch(repo, releaseBranch) // 1.12.0-RC1 in the above example
			newDivergedCommit := getCommitForTag(repo, newDivergedVer)
			checkoutBranch(repoArg, releaseBranch) // checkout v1.12
			forEachGitLogUntil(repo, func(c *gitobj.Commit) {
				if is, _ := c.IsAncestor(newDivergedCommit); !is {
					return
				}
				commitTitle := getCommitTitle(c.Message)
				if _, ok := commitToVersionMap[commitTitle]; !ok {
					// if not tagged "v1.11", it must belong to "v1.12.0-RC1"
					commitToVersionMap[commitTitle] = newDivergedVer
				}
			}, divergedCommit)
			infoLog("found the new diverged point %s: %s", newDivergedVer, newDivergedCommit.ID().String()[:10])
		}
		versions := mapCommitTitleToVersion(repo, releaseBranch)
		currentVersion := "cherry-picked" // not this commit is not released but is cherry-picked
		forEachGitLogUntil(repo, func(c *gitobj.Commit) {
			commitTitle := getCommitTitle(c.Message)
			if ver, ok := versions[commitTitle]; ok {
				currentVersion = ver
			}
			if _, ok := commitToVersionMap[commitTitle]; !ok {
				commitToVersionMap[commitTitle] = currentVersion
			}
		}, divergedCommit)

		checkoutBranch(repoArg, "master")
		masterDivergedCommit, has := findEqualCommitInRepo(repo, divergedCommit)
		tryTimes := 0
		for !has {
			// trace back to the first commit of the release branch: `initialCommit`, this is where the master branch
			// and the release branch are diverged.
			//
			// master    1.11
			// |          |
			// |/---------| => where the branch is forked (`masterDivergedCommit`)
			// |

			debugLog("commit \"%s\" in branch %s has not counterpart in master, step back",
				strings.TrimSpace(divergedCommit.Message), releaseBranch)
			parent, err := divergedCommit.Parent(0)
			if err != nil {
				return fatalError("unable to find parent for commit: %s", divergedCommit.Hash)
			}
			divergedCommit = parent
			if tryTimes++; tryTimes > 10 {
				return fatalError("stop. unable to find the equal commits both in master and %s", releaseBranch)
			}
			masterDivergedCommit, has = findEqualCommitInRepo(repo, divergedCommit)
		}

		// find the counterpart in master branch, if not, print it in the table
		checkoutBranch(repoArg, "master")
		// TODO(wutao1): compare the current commit revision with the origin.
		unreleasedCount := 0
		releasedCount := 0
		debugLog("start scanning master branch")
		var tableBulk [][]string
		forEachGitLogUntil(repo, func(c *gitobj.Commit) {
			commitTitle := getCommitTitle(c.Message)
			if _, err := getPrIDInt(commitTitle); err != nil {
				warnLog("ignore invalid commit: \"%s\"", commitTitle)
				return
			}
			ver, ok := commitToVersionMap[commitTitle]
			verObj, _ := version.NewVersion(ver)
			if ver != "" && ver != "cherry-picked" && !latestReleasedVer.LessThan(verObj) {
				// has released in versions lower than --version
				debugLog("skip \"%s\" of version %s", commitTitle, ver)
				return
			}
			if ok {
				if ver != "" && ver != "cherry-picked" && !includeReleased {
					// skip those that are released already
					return
				}
				releasedCount++
			} else {
				unreleasedCount++
			}
			row := []string{
				fmt.Sprintf("%s/%s%s", owner, repoName, getPrID(commitTitle)),
				commitTitle[:strings.LastIndex(commitTitle, "(")]} // drop the PrID part, because the PR column has included
			if !short {
				daysAfterMerged := time.Since(c.Committer.When).Hours() / 24
				row = append(row, fmt.Sprintf("%.2f", daysAfterMerged))
				row = append(row, ver)
			}
			tableBulk = append(tableBulk, row)
		}, masterDivergedCommit)

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
		if includeReleased {
			header = append(header, "Version")
		}
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
	versions := getAllVersions(repo, func(ver string) bool {
		return strings.HasPrefix(ver, releaseBranch+".")
	})
	if len(versions) == 0 {
		fatalExit(fatalError("there's no version in \"%s\" branch", releaseBranch))
	}
	sort.Sort(sort.Reverse(version.Collection(versions)))
	return versions[0].Original()
}

func getAllVersions(repo *git.Repository, filter func(string) bool) []*version.Version {
	tagIter, err := repo.Tags()
	fatalExitIfNotNil(err)
	var versions []*version.Version
	err = tagIter.ForEach(func(ref *plumbing.Reference) error {
		rawVer := ref.Name().Short()
		if filter != nil && !filter(rawVer) {
			return nil
		}
		v, err := version.NewVersion(rawVer)
		if err == nil {
			versions = append(versions, v)
		}
		return nil
	})
	fatalExitIfNotNil(err)
	return versions
}

// In Pegasus's convention, the initial version of `1.12` is `1.12.0-RC1`. If there's no RC versions, the initial version is 1.12.0.
func getInitialVersionInReleaseBranch(repo *git.Repository, releaseBranch string) string {
	versions := getAllVersions(repo, func(ver string) bool {
		return strings.HasPrefix(ver, releaseBranch+".")
	})
	if len(versions) == 0 {
		fatalExit(fatalError("there's no version in \"%s\" branch", releaseBranch))
	}
	sort.Sort(version.Collection(versions))
	return versions[0].Original()
}

func getLatestReleasedVersion(repo *git.Repository) *version.Version {
	versions := getAllVersions(repo, nil)
	sort.Sort(sort.Reverse(version.Collection(versions)))
	for _, v := range versions {
		if len(v.Prerelease()) == 0 {
			return v
		}
	}
	return nil
}

func getLatestVersion(repo *git.Repository) string {
	versions := getAllVersions(repo, nil)
	sort.Sort(sort.Reverse(version.Collection(versions)))
	return versions[0].Original()
}

func hasVersion(repo *git.Repository, releaseBranch string, version string) bool {
	versions := getAllVersionsInMinorVersion(repo, releaseBranch)
	_, ok := versions[version]
	return ok
}

func mapCommitTitleToVersion(repo *git.Repository, releaseBranch string) map[string]string {
	commitTitleToVersion := make(map[string]string)
	for version, title := range getAllVersionsInMinorVersion(repo, releaseBranch) {
		commitTitleToVersion[title] = version
	}
	return commitTitleToVersion
}

func getAllVersionsInMinorVersion(repo *git.Repository, releaseBranch string) map[string]string {
	tagIter, err := repo.Tags()
	if err != nil {
		fatalExit(fatalError("unable to get tags in release branch: %s", releaseBranch))
	}

	versions := make(map[string]string)
	err = tagIter.ForEach(func(ref *plumbing.Reference) error {
		ver := ref.Name().Short()
		if strings.HasPrefix(ver, releaseBranch) {
			commit, err := repo.CommitObject(ref.Hash())
			if err != nil {
				warnLog("unable to find commit for tag %s: %s", ver, err)
				return nil
			}
			versions[ver] = getCommitTitle(commit.Message)
		}
		return nil
	})
	fatalExitIfNotNil(err)
	if len(versions) == 0 {
		fatalExit(fatalError("no version tagged in release branch %s", releaseBranch))
	}
	return versions
}

func getAllBranches(repo *git.Repository) []string {
	iter, err := repo.Branches()
	if err != nil {
		fatalExit(fatalError("unable to get branches"))
	}
	var branches []string
	fatalExitIfNotNil(iter.ForEach(func(ref *plumbing.Reference) error {
		branchName := ref.Name().Short()
		if strings.HasPrefix(branchName, "v") {
			branches = append(branches, branchName)
		}
		return nil
	}))
	return branches
}

func hasBranch(repo *git.Repository, branch string) bool {
	branches := getAllBranches(repo)
	sort.Strings(branches)
	return sort.SearchStrings(branches, branch) != len(branches)
}

func getPreviousReleaseBranch(repo *git.Repository, branch string) string {
	branches := getAllBranches(repo)
	sort.Strings(branches)
	idx := sort.SearchStrings(branches, branch)
	if idx == 0 {
		fatalExit(fatalError("cannot find any branch that released before %s: %s", branch, branches))
	}
	return branches[idx-1]
}

func validateVersionArg(repo *git.Repository) error {
	if matched, _ := regexp.MatchString(`\d+\.\d+(\.\d+)?(-RC\d+)?`, versionArg); !matched {
		return fatalError("invalid version: %s, version should have 2 or 3 parts, like \"1.11\" or \"1.12.1\" or \"2.0.0-RC1\"", versionArg)
	}
	parts := strings.Split(versionArg, ".")
	releaseBranch := fmt.Sprintf("v%s.%s", parts[0], parts[1])
	if !hasBranch(repo, releaseBranch) {
		return fatalError("no such branch \"%s\", please check `git branch` for all branches", releaseBranch)
	}
	if len(parts) == 3 && !hasVersion(repo, releaseBranch, "v"+versionArg) {
		return fatalError("no such version tagged \"%s\", please check `git tag` for all tags", "v"+versionArg)
	}
	return nil
}
