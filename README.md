# jira-sync

This repo contains tools for syncing data from public sources into a
Jira instance.

For example, this command

```
~/go/bin/github-to-jira \
    -jira-user you -jira-password secret \
    -github-token too-long-to-type \
    -jira-url https://project-managers.bigco.com \
    -jira-project MY_THING \
    -github-org upstream
```

scans all of the repositories in the "upstream" org on github.com
looking for open tickets. For each one it finds, it looks for a ticket
in the jira instance with a slug made up of the github org, repo, and
issue number. If no such ticket is found, a new ticket is created in
the "MY_THING" jira project.

The following command scans only the "upstream/reponame" repository:

```
~/go/bin/github-to-jira \
    -jira-user you -jira-password secret \
    -jira-url https://project-managers.bigco.com \
    -jira-project MY_THING \
    -github-token too-long-to-type \
    -github-org upstream \
    reponame
```

Bugzilla tickets can be imported as bugs using

```
~/go/bin/bugzilla-to-jira \
    -jira-user you -jira-password secret \
    -jira-url https://project-managers.bigco.com \
    -jira-project MY_THING \
    -bugzilla-token garbled-hash \
    -bugzilla-url https://bugs.bigco.com \
    -bugzilla-product 'My Thing'
```

To find jira tickets associated with closed github or bugzilla tickets
and mark them as closeable, use "find-closed":

```
~/go/bin/find-closed \
    -jira-user you -jira-password secret \
    -jira-url https://project-managers.bigco.com \
    -jira-project MY_THING \
    -bugzilla-token garbled-hash \
    -bugzilla-url https://bugs.bigco.com \
    -github-token too-long-to-type
```

## Installing

```
go get github.com/dhellmann/jira-sync/github-to-jira
go get github.com/dhellmann/jira-sync/bugzilla-to-jira
go get github.com/dhellmann/jira-sync/find-closed
```
