package main

import (
	"fmt"
	"os"
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

		// Find the initial commit of the minor version, and find the commits
		// afterwards in master branch.

		pickedCommits := getAllCommitsPickedForUpcomingRelease(repo, getLatestReleasedVersion(repo).Original())
		notPickedCommits := getAllCommitsNotPicked(repo)
		commits := append(notPickedCommits, pickedCommits...)
		var tableBulk [][]string
		for _, c := range commits {
			row := &rowForCommit{
				owner:           owner,
				repoName:        repoName,
				version:         c.version,
				title:           c.title,
				daysAfterMerged: c.daysAfterMerged,
			}
			tableBulk = append(tableBulk, row.toColumns())
		}
		printTable(tableBulk, len(pickedCommits), len(notPickedCommits))
		return nil
	},
}

type rowForCommit struct {
	owner           string
	repoName        string
	version         string
	title           string
	daysAfterMerged float64
}

func (row *rowForCommit) toColumns() []string {
	if _, err := getPrIDInt(row.title); err != nil {
		warnLog("ignore invalid commit: \"%s\"", row.title)
		return nil
	}
	columns := []string{
		fmt.Sprintf("%s/%s%s", row.owner, row.repoName, getPrID(row.title)),
		row.title[:strings.LastIndex(row.title, "(")]} // drop the PrID part, because the PR column has included
	if !short {
		columns = append(columns, fmt.Sprintf("%.2f", row.daysAfterMerged))
		columns = append(columns, row.version)
	}
	return columns
}

func printTable(tableBulk [][]string, pickedCount, notPickedCount int) {
	table := tablewriter.NewWriter(os.Stdout)
	var header []string
	header = []string{fmt.Sprintf("PR (%d TOTAL, %d PICKED)", pickedCount+notPickedCount, pickedCount), "TITLE"}
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

// Returns the latest version, not including pre-released versions.
// For example, given v1.11.1, v1.11.2, v1.11.3-RC1, this function returns v1.11.2.
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

func getBranch(ver string) string {
	parts := strings.Split(ver, ".")
	branch := fmt.Sprintf("%s.%s", parts[0], parts[1])
	if branch[0] == 'v' {
		return branch
	}
	return "v" + branch
}

type simpleCommit struct {
	title           string
	version         string
	daysAfterMerged float64
}

// Gets commits reside in master but not cherry-picked to release branch
func getAllCommitsNotPicked(repo *git.Repository) []*simpleCommit {
	releaseBranch := getBranch(getLatestVersion(repo))
	divergedCommit := getCommitForTag(repo, getInitialVersionInReleaseBranch(repo, releaseBranch))

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
			fatalExit(fatalError("unable to find parent for commit: %s", divergedCommit.Hash))
		}
		divergedCommit = parent
		if tryTimes++; tryTimes > 10 {
			fatalExit(fatalError("stop. unable to find the equal commits both in master and %s", releaseBranch))
		}
		masterDivergedCommit, has = findEqualCommitInRepo(repo, divergedCommit)
	}

	debugLog("start scanning master branch")
	commits := getAllCommitsInBranchFrom(repo, "master", masterDivergedCommit)

	pickedCommits := getAllCommitsInReleaseBranch(repo, releaseBranch)
	pickedSet := map[string]bool{}
	for _, c := range pickedCommits {
		pickedSet[c.title] = true
	}
	var notPicked []*simpleCommit
	for _, c := range commits {
		if _, ok := pickedSet[c.title]; !ok {
			notPicked = append(notPicked, c)
		}
	}
	return notPicked
}

// getAllCommitsPickedForUpcomingRelease returns all commits that are cherry-picked in the latest version.
// `pastReleasedVer` must not be a pre-released version.
func getAllCommitsPickedForUpcomingRelease(repo *git.Repository, pastReleasedVer string) []*simpleCommit {
	releaseBranch := getBranch(pastReleasedVer)
	latestVer := getLatestVersion(repo)

	var result []*simpleCommit
	if !strings.HasPrefix(latestVer, releaseBranch) {
		// If it's an upcoming minor release (branched from master).

		// The commits between the two diverged points (1.11.0-RC1, 1.12.0-RC1 e.g)
		// but not in the previous release branch (1.11), are certainly in the new minor version (1.12.0).
		//
		//                <-|/
		//                <-|------- 1.12.0-RC1
		//                <-|
		// 1.12.0 commits <-| /----- 1.11.6
		//                  |/
		//                  |------- 1.11.0-RC1
		//                  |

		commits := getAllCommitsInReleaseBranch(repo, releaseBranch)
		commitToVersionMap := make(map[string]string)
		for _, c := range commits {
			// Firstly tag all the commits in the previous release branch (1.11 in the above example),
			// aka commits between 1.11.0-RC1 ~ 1.11.6, to "v1.11"
			commitToVersionMap[c.title] = releaseBranch
		}
		divergedVer := getInitialVersionInReleaseBranch(repo, releaseBranch)
		divergedCommit := getCommitForTag(repo, divergedVer)
		infoLog("the diverged point of master and %s is %s: %s", releaseBranch, divergedVer, divergedCommit.ID().String()[:10])

		releaseBranch = getBranch(latestVer)
		newCommits := getAllCommitsInBranchFrom(repo, releaseBranch, divergedCommit)
		for _, c := range newCommits {
			// those not tagged v1.11 are certainly belong to v1.12
			if _, ok := commitToVersionMap[c.title]; !ok {
				result = append(result, c)
			}
		}
	} else {
		startingCommit := getCommitForTag(repo, pastReleasedVer)
		result = getAllCommitsInBranchFrom(repo, releaseBranch, startingCommit)
	}

	return result
}

// Get commits (sorted by time order) within release branch.
func getAllCommitsInReleaseBranch(repo *git.Repository, branch string) []*simpleCommit {
	initialVer := getInitialVersionInReleaseBranch(repo, branch)
	divergedCommit := getCommitForTag(repo, initialVer)
	return getAllCommitsInBranchFrom(repo, branch, divergedCommit)
}

// Get commits starting from `startingCommit` (sorted by time order) within branch (could be a master branch).
func getAllCommitsInBranchFrom(repo *git.Repository, branch string, startingCommit *gitobj.Commit) []*simpleCommit {
	checkoutBranch(repoArg, branch)

	currentVersion := ""
	var versions map[string]string
	if branch != "master" {
		versions = mapCommitTitleToVersion(repo, branch)
		currentVersion = "cherry-picked"
	}

	var commits []*simpleCommit
	forEachGitLogUntil(repo, func(c *gitobj.Commit) {
		commitTitle := getCommitTitle(c.Message)
		if ver, ok := versions[commitTitle]; ok {
			currentVersion = ver
		}
		commits = append(commits, &simpleCommit{
			title:           commitTitle,
			version:         currentVersion,
			daysAfterMerged: time.Since(c.Committer.When).Hours() / 24,
		})
	}, startingCommit)
	return commits
}
