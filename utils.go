package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	git "gopkg.in/src-d/go-git.v4"
	gitobj "gopkg.in/src-d/go-git.v4/plumbing/object"
	gitstorer "gopkg.in/src-d/go-git.v4/plumbing/storer"
)

func fatalError(format string, a ...interface{}) error {
	return fmt.Errorf("fatal: %s", fmt.Sprintf(format, a...))
}

func fatalExit(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func fatalExitIfNotNil(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func executeCommandAndGet(format string, a ...interface{}) (result string, err error) {
	cmd := fmt.Sprintf(format, a...)
	output, err := exec.Command("bash", "-c", cmd).CombinedOutput()
	if err != nil {
		err = fatalError("failed to execute command:\n  %s\nerror: %s", cmd, output)
		return
	}
	result = string(output)
	return
}

func executeCommand(format string, a ...interface{}) error {
	_, err := executeCommandAndGet(format, a...)
	return err
}

// for example, for xiaomi/pegasus, the owner is "xiaomi", the repoName is "pegasus"
func getOwnerAndRepoFromURL(url string) (owner string, repo string) {
	url = strings.TrimSuffix(url, ".git")
	urlParts := strings.Split(url, "/")
	owner = urlParts[len(urlParts)-2]
	repo = urlParts[len(urlParts)-1]

	colon := strings.Index(owner, ":") // in case it's a ssh url
	if colon != -1 {
		owner = owner[colon+1:]
	}
	return
}

func getPrName(owner, repoName string, prID int) string {
	return fmt.Sprintf("%s/%s#%d", owner, repoName, prID)
}

func getPrID(commitMsg string) string {
	pleft := strings.LastIndex(commitMsg, "(")
	pright := strings.LastIndex(commitMsg, ")")
	commitMsg = commitMsg[:pright]
	commitMsg = commitMsg[pleft+1:]
	return commitMsg
}

func getPrIDInt(commitMsg string) (int, error) {
	pleft := strings.LastIndex(commitMsg, "(")
	pright := strings.LastIndex(commitMsg, ")")
	if pleft == -1 || pright == -1 {
		return -1, fmt.Errorf("invalid commit message \"%s\"", commitMsg)
	}
	commitMsg = commitMsg[:pright]
	commitMsg = commitMsg[pleft+1:]
	return strconv.Atoi(commitMsg[1:])
}

func getCommitTitle(commitMsg string) string {
	title := strings.Split(strings.TrimSpace(commitMsg), "\n")[0] // get the first line
	return strings.TrimSpace(title)
}

func checkoutBranch(repo, branch string) {
	fatalExitIfNotNil(executeCommand("cd %s; git checkout %s", repo, branch))
}

func getCommitForTag(repo *git.Repository, tagName string) *gitobj.Commit {
	tag, err := repo.Tag(tagName)
	if err != nil {
		fatalExitIfNotNil(fatalError("no such version tag: %s", tagName))
	}
	tagObj, err := repo.TagObject(tag.Hash())
	var commit *gitobj.Commit
	if err != nil {
		commit, err = repo.CommitObject(tag.Hash())
		fatalExitIfNotNil(err)
	} else {
		commit, err = tagObj.Commit()
		fatalExitIfNotNil(err)
	}
	return commit
}

func findEqualCommitInRepo(repo *git.Repository, commit *gitobj.Commit) (cpCommit *gitobj.Commit, result bool) {
	// must have the same title
	return findCommitContainsStrInRepo(repo, getCommitTitle(commit.Message))
}

func findCommitContainsStrInRepo(repo *git.Repository, substr string) (cpCommit *gitobj.Commit, result bool) {
	iter, err := repo.Log(&git.LogOptions{})
	if err != nil {
		fatalExit(fatalError("unable to perform git log"))
	}

	result = false
	err = iter.ForEach(func(c *gitobj.Commit) error {
		if strings.Contains(getCommitTitle(c.Message), substr) {
			result = true
			cpCommit = c // find the counterpart
			return gitstorer.ErrStop
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return
}

func debugLog(format string, a ...interface{}) {
	if debug {
		fmt.Println("debug:", fmt.Sprintf(format, a...))
	}
}

func infoLog(format string, a ...interface{}) {
	fmt.Println("info :", fmt.Sprintf(format, a...))
}

func errorLog(format string, a ...interface{}) {
	fmt.Println("error:", fmt.Sprintf(format, a...))
}

func warnLog(format string, a ...interface{}) {
	fmt.Println("warn :", fmt.Sprintf(format, a...))
}
