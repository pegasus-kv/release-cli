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
label '1.11.7' to the PR.

So the entire procedure of a release can be concluded:

0. Review what are changed recently.
1. Cherry-pick some pull-requests.
2. Tag this release.
3. Label the included pull-requests.

## Installation

```sh
make
```

Or you can download [pre-built release](https://github.com/pegasus-kv/release-cli/releases) that is suitable for your platform.

## Usage

Please ensure your repo where you want to make release is as clean as possible.
It's recommended to re-clone the repo to different location with your development branch, if you have one.

### To show the pull requests that are not released, and how much time after the changes were committed (the 'Release velocity')

```sh
./release-cli show --repo '/home/wutao1/pegasus/rdsn'
```

This command compares the master branch with the latest version (`v1.12.3` e.g), showing the commits that are not released.

Outputs:

```txt
| PR (26 TOTAL)   | TITLE                                               | DAYS AFTER COMMIT |
| --------------- | --------------------------------------------------- | ----------------- |
| XiaoMi/rdsn#459 | fix: fix the bug in restore                         | 0.01              |
| XiaoMi/rdsn#457 | feat(bulk-load): meta server send bulk load request | 0.17              |
| XiaoMi/rdsn#454 | feat(bulk-load): meta server start bulk load        | 4.20              |
| XiaoMi/rdsn#443 | feat(cold-backup): add rate limit for fds           | 4.89              |
| XiaoMi/rdsn#456 | feat: update rpc_holder                             | 4.95              |
...
```

If you want to view the commits that have been officially released in some version, 1.12.3 for example,
go check the github label <https://github.com/XiaoMi/pegasus/pulls?q=is%3Apr+label%3A1.12.3>.

### To specify the pull requests to 1.11 of Pegasus

```sh
./release-cli add --repo /home/wutao1/pegasus --branch 1.11 242 243 246
```

This command will cherry-pick the corresponding commits of the PRs to the 1.11 branch.
Note that we assume the `origin` remote is where the official repository are.
In our above example, the `origin` must be "<https://github.com/XiaoMi/pegasus.git"> or
"git@github.com:XiaoMi/pegasus.git".

### To submit the cherry-picks and make a new release 1.11.6

```sh
./release-cli submit --repo /home/wutao1/pegasus --version=1.11.7 --access <ACCESS_TOKEN>
```

This command will label the submitted but not released cherry-picks in branch 1.11
with `1.11.7` on Github as well, so that which version the PR was picked can be
located easily. For example, in <https://github.com/XiaoMi/rdsn/issues?q=label%3A1.12.3+is%3Aclosed>
you can find all 1.12.3 changes.

### To release a minor/major version (2.0 e.g.)

There's no many differences in the procedure between a minor/major release and a patch release, but first you need
to checkout a new branch for the version.

```sh
cd /home/wutao1/pegasus
git checkout master
git reset --hard <commit> # Optional. Sometimes you may want to create the branch out from a specific commit instead of <HEAD>.
git checkout -b v2.0
```

Tag this version as v2.0.0-RC0 when you confirm that it is stable enough.

```sh
git tag v2.0.0-RC0
git push origin v2.0 --tags
```
