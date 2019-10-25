# Pegasus Release Cli

Release in Pegasus's convention.

> We branch from the mainline at a specific revision and never merge changes
> from the branch back into the mainline. Bug fixes are submitted to the mainline
> and then cherry picked into the branch for inclusion in the release.
>
> -- from Google's "Site Reliability Engineering"

In Pegasus, every our pull-requests will be squashed as a single commit
into master branch, which is the "mainline". To make a patch release we first pick up
the pull-requests that are not going to bring instability to our system.
For example, a feature that is not ready but some parts of it were committed to master
shall not be included in current release. After some cherry-picks to the release branch,
we tag the `HEAD` revision to the version, 1.11.7, e.g. Usually we will make one or more
'RC' versions (aka Release Candidate) before the final release. To identify whether a merged
pull-request is released or pre-released we usually use Github Labels, for example a
label 'release-note' to the PR.

So the entire procedure of release can be concluded:

1. Cherry-pick some pull-requests
2. Tag this release.
3. Label the included pull-requests.

## Installation

```sh
make
```

Or you can download [pre-built release] that is suitable for your platform.

## Usage

### To specify the pull requests to 1.11 of Pegasus

```sh
./release-cli add --repo '/home/wutao1/pegasus' --branch '1.11' --pr-list='242,243,246'
./release-cli add --repo '/home/wutao1/pegasus' --branch '1.11' --pr=245
```

This command will cherry-pick the corresponding commits of the PRs to the 1.11 branch.
Note that we assume the `origin` remote is where the official repository are.
In our above example, the origin must be "<https://github.com/XiaoMi/pegasus.git"> or
"git@github.com:XiaoMi/pegasus.git".

### To submit the cherry-picks and make a new release 1.11.7

```sh
./release-cli submit --repo '/home/wutao1/pegasus' --branch '1.11' --version='1.11.7'
```

This command will tag the `HEAD` revision to '1.11.7', label the submitted cherry-picks
with `release-1.11.7` on Github as well, so that which version the PR was picked can be
located easily.

### To show the pull requests that are not released, and how much time after the changes was committed (the 'Release velocity').

```sh
./release-cli show --repo '/home/wutao1/pegasus' --version '1.11'
```

Outputs:

```txt
384 | "fix: unit test failed and update rdsn" | 45 Days
```

If only the pull-request-ID is wanted:

```sh
./release-cli show --repo '/home/wutao1/pegasus' --version '1.11' --pr-only
```
