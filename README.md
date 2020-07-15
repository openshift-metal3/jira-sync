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
go get github.com/openshift-metal3/jira-sync/github-to-jira
go get github.com/openshift-metal3/jira-sync/bugzilla-to-jira
go get github.com/openshift-metal3/jira-sync/bugzilla-one
go get github.com/openshift-metal3/jira-sync/find-closed
```

## Using link_one.sh

To create a single JIRA issue from a BZ, create a file in your home
directory called `~/.jira_sync_settings`:

```
jira_user=janedoe
jira_password='p0t4t03s'
jira_url=https://issues.redhat.com
bugzilla_token=your_token
bugzilla_url=https://bugzilla.redhat.com
```

To get a bugzilla token, login and go to preferences, API Keys, and
create one.

*Note*: For `jira_user` field use the content of the field `Username` in your JIRA -> Profile Summary section (in case you're using the email to login since it could be slightly different)

Then to create a JIRA from BZ, pass the bug id to link_one.sh:

```
$ /link_one.sh 1823359
https://bugzilla.redhat.com/show_bug.cgi?id=1823359 "Openshift 4.4
Baremetal IPI install fails using external DHCP server on provisioning
network" CREATED KNIDEPLOY-2069
https://issues.redhat.com/browse/KNIDEPLOY-2069 Openshift 4.4 Baremetal
IPI install fails using external DHCP server on provisioning network
[bugzilla:1823359]
```
