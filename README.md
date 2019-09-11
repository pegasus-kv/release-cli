# Pegasus Release Cli

Release Pegasus in our convention.

> We branch from the mainline at a specific revision and never merge changes
> from the branch back into the mainline. Bug fixes are submitted to the mainline
> and then cherry picked into the branch for inclusion in the release.
>
> -- from Google's "Site Reliability Engineering"

In Pegasus, every our pull-requests will be squashed as a single commit
into master branch, which is the "mainline". To make a patch release we first pick up
the pull-requests that are not going to bring instability to our system.
For example, a feature that is not ready but some parts of it were committed to master
shall not be included. After some cherry-picks to the release branch,
we tag the `HEAD` commit to the version, 1.11.7, e.g. Usually we will make one or more
'RC' versions (aka Release Candidate) before the final release. To identify whether a merged
pull-request is released or pre-released we usually use Github Labels, for example a
label 'release-note' to the PR.

1. Cherry-pick some pull-requests
2. Tag this release.
3. Label the included pull-requests.

## Installation

```sh
make
```

Or you can download [pre-built release] that is suitable for your platform.

## Usage

To specify multiple pull requests to version 1.11.7 of Pegasus:

```sh
./release-cli --repo '/home/wutao1/pegasus' --version '1.11.7' --pr-list='242,243,246'
```

To specify a single pull request to version 1.11.7:

```sh
./release-cli --repo '/home/wutao1/pegasus' --version '1.11.7' --pr=245
```

To show the pull requests that are not released, and how much time after
the changes was committed (the 'Release velocity').

```sh
./release-cli --repo '/home/wutao1/pegasus' --show-unreleased
```

To show the pull requests that are included in 1.11.7:

```sh
./release-cli --repo '/home/wutao1/pegasus' --version '1.11.7'
```
