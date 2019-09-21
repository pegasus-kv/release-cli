package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	git "gopkg.in/src-d/go-git.v4"
	gitobj "gopkg.in/src-d/go-git.v4/plumbing/object"
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

func getOwnerAndRepoFromURL(url string) (owner string, repo string) {
	url = strings.TrimSuffix(url, ".git")
	urlParts := strings.Split(url, "/")
	owner = urlParts[len(urlParts)-2]
	repo = urlParts[len(urlParts)-1]
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
	return strings.Split(strings.TrimSpace(commitMsg), "\n")[0] // get the first line
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
		fmt.Printf("warn: tag %s is possibly a lightweight tag, not an annotated tag\n", tagName)
		commit, err = repo.CommitObject(tag.Hash())
		fatalExitIfNotNil(err)
	} else {
		commit, err = tagObj.Commit()
		fatalExitIfNotNil(err)
	}
	return commit
}