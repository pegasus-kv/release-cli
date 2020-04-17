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

So the entire procedure of release can be concluded:

0. Review what're changed recently.
1. Cherry-pick some pull-requests.
2. Tag this release.
3. Label the included pull-requests.

## Installation

```sh
make
```

Or you can download [pre-built release](https://github.com/pegasus-kv/release-cli/releases/0.0.0) that is suitable for your platform.

## Usage

Please ensure your repo that you want to make release are as clean as possible.
It's recommended to re-clone the repo to different location with your development branch, if you have one.

### To specify the pull requests to 1.11 of Pegasus

```sh
./release-cli add --repo /home/wutao1/pegasus --branch 1.11 242 243 246
```

This command will cherry-pick the corresponding commits of the PRs to the 1.11 branch.
Note that we assume the `origin` remote is where the official repository are.
In our above example, the origin must be "<https://github.com/XiaoMi/pegasus.git"> or
"git@github.com:XiaoMi/pegasus.git".

### To submit the cherry-picks and make a new release 1.11.7

```sh
./release-cli submit --repo /home/wutao1/pegasus --version=1.11.7 --access <ACCESS_TOKEN>
```

This command will label the submitted but not released cherry-picks in branch 1.11
with `1.11.7` on Github as well, so that which version the PR was picked can be
located easily. For example, in <https://github.com/XiaoMi/rdsn/issues?q=label%3A1.12.3+is%3Aclosed>
you can find all 1.12.3 changes.

### To show the pull requests that are not released, and how much time after the changes was committed (the 'Release velocity').

```sh
./release-cli show --repo /home/wutao1/pegasus --version 1.12
```

Outputs:

```txt
|  PR (6 TOTAL)   |                                 TITLE                                  | DAYS AFTER COMMIT |
|-----------------|------------------------------------------------------------------------|-------------------|
| XiaoMi/rdsn#436 | refactor: simplify mutation_log write_pending_mutations                |              4.01 |
| XiaoMi/rdsn#434 | refactor(backup): move collect_backup_info to replica_backup_manager   |              8.94 |
| XiaoMi/rdsn#432 | refactor(backup): make backup clear decoupled from on_cold_backup      |             10.02 |
| XiaoMi/rdsn#419 | feat: add perf-counter for backup request                              |             28.84 |
| XiaoMi/rdsn#408 | feat: refine mlog_dump output                                          |             43.60 |
| XiaoMi/rdsn#255 | refactor(rpc): refactor request meta & add support for backup request  |             69.87 |
```

If want what changes after a specific version, take 1.12.3 for example.

```sh
./release-cli show --repo /home/wutao1/pegasus --version 1.12.3 --released
```

```
| PR (50 RELEASED, 56 TOTAL) |                                          TITLE                                          | DAYS AFTER COMMIT |             |
|----------------------------|-----------------------------------------------------------------------------------------|-------------------|-------------|
| XiaoMi/rdsn#436            | refactor: simplify mutation_log write_pending_mutations                                 |              4.01 |
| XiaoMi/rdsn#435            | feat: tcmalloc memory release improvements                                              |              6.93 | v1.12.3-RC3 |
| XiaoMi/rdsn#434            | refactor(backup): move collect_backup_info to replica_backup_manager                    |              8.94 |
| XiaoMi/rdsn#432            | refactor(backup): make backup clear decoupled from on_cold_backup                       |             10.02 |
| XiaoMi/rdsn#430            | refactor(backup): delay the removal of checkpoint files produced by cold backup         |             16.97 | v1.12.3-RC2 |
| XiaoMi/rdsn#431            | refactor: move log-block-writing-related codes from mutation_log to log_block           |             20.81 | v1.12.3-RC1 |
| XiaoMi/rdsn#429            | feat(dup): support multiple fail modes for duplication                                  |             20.91 | v1.12.3-RC1 |
| XiaoMi/rdsn#427            | refactor: move log_block class from mutation_log.h to separated file                    |             22.85 | v1.12.3-RC1 |
| XiaoMi/rdsn#428            | fix(backup): use https to access fds, instead of http                                   |             23.09 | v1.12.3-RC1 |
| XiaoMi/rdsn#425            | fix(dup): multiple fixes on replica duplication                                         |             23.75 | v1.12.3-RC1 |
| XiaoMi/rdsn#424            | feat(dup): add dsn_replica_dup_test to CI testing and fix http outputs                  |             23.98 | v1.12.3-RC1 |
| XiaoMi/rdsn#421            | feat: add a new app_env to limit scan time                                              |             24.73 | v1.12.3-RC1 |
| XiaoMi/rdsn#423            | feat(dup): rename change_dup_status to modify_dup                                       |             24.94 | v1.12.3-RC1 |
| XiaoMi/rdsn#422            | fix: fix memory leak in tls_transient_memory_t                                          |             25.06 | v1.12.3-RC1 |
| XiaoMi/rdsn#416            | feat: add query_disk_info api for shell command                                         |             28.52 | v1.12.3-RC1 |
| XiaoMi/rdsn#411            | feat(dup): handle non-idempotent writes during duplication                              |             28.56 | v1.12.3-RC1 |
| XiaoMi/rdsn#409            | feat: add query_disk_info support for replica_server                                    |             28.60 | v1.12.3-RC1 |
| XiaoMi/rdsn#420            | refactor: delete unused log in replica::on_config_sync                                  |             28.62 | v1.12.3-RC1 |
....
```
