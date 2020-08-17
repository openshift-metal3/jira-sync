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

## Configuring

To create a single JIRA issue from a BZ, create a file in your home
directory called `~/.jira_sync_settings`:

```
jira_user=janedoe
jira_password='p0t4t03s'
jira_url=https://issues.redhat.com
bugzilla_token=your_token
bugzilla_url=https://bugzilla.redhat.com
```

## Using link_one.sh

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

## Using check_pr.sh

To check the status of pull requests associated with a Jira ticket
tree, create the `~/.jira_sync_settings` file as described above and
then run `check_pr.sh` with the Jira ticket IDs as argument.

```
$ ./check_pr.sh KNIDEPLOY-3417 KNIDEPLOY-2109

Bug (Code Review) https://issues.redhat.com/browse/KNIDEPLOY-3417 "root device hints implementation in installer ties installer to BMO API"
  downstream on master merged: https://github.com/openshift/installer/pull/3952 "baremetal: set the boot mode for hosts based on the input"
  downstream on master OPEN: https://github.com/openshift/installer/pull/4002 "Bug 1864092: baremetal: copy the implementation of rootdevicehints from baremetal-operator"

Epic (Done) https://issues.redhat.com/browse/KNIDEPLOY-2109 "Enable choosing/overriding install and cleaning disks"
  upstream on master closed: https://github.com/metal3-io/baremetal-operator/pull/442 "Rootdevicehints added to BMH spec"
  upstream on master merged: https://github.com/metal3-io/baremetal-operator/pull/495 "root device hints"
    downstream on master merged: https://github.com/openshift/baremetal-operator/pull/73 "Merge upstream 2020 06 04"
  downstream on master closed: https://github.com/openshift/installer/pull/3348 "[WIP] baremetal: Allow rootHints to override Host profiles"

  Story (ASSIGNED) https://issues.redhat.com/browse/OSDOCS-1308 "D/S documentation: Admin & Operations"
    no github links found

  Story (ASSIGNED) https://issues.redhat.com/browse/OSDOCS-1307 "D/S documentation - Install, Configure, Test"
    no github links found

  Story (ASSIGNED) https://issues.redhat.com/browse/OSDOCS-1306 "D/S documentation: Planning and Pre-reqs"
    no github links found

  Story (Done) https://issues.redhat.com/browse/KNIDEPLOY-1669 "Support complete set of root device hints [github:metal3-io:baremetal-operator:400]"
    upstream on master merged: https://github.com/metal3-io/baremetal-operator/pull/495 "root device hints"
      downstream on master merged: https://github.com/openshift/baremetal-operator/pull/73 "Merge upstream 2020 06 04"

    Sub-task (Done) https://issues.redhat.com/browse/KNIDEPLOY-2283 "upstream metal3 changes in BMO"
      upstream on master merged: https://github.com/metal3-io/baremetal-operator/pull/495 "root device hints"
        downstream on master merged: https://github.com/openshift/baremetal-operator/pull/73 "Merge upstream 2020 06 04"
      upstream on master merged: https://github.com/metal3-io/baremetal-operator/pull/544 "add minimum validation for root device hint size"
        downstream on master merged: https://github.com/openshift/baremetal-operator/pull/73 "Merge upstream 2020 06 04"

    Sub-task (Done) https://issues.redhat.com/browse/KNIDEPLOY-2284 "merge BMO changes downstream"
      downstream on master merged: https://github.com/openshift/baremetal-operator/pull/73 "Merge upstream 2020 06 04"

    Sub-task (Done) https://issues.redhat.com/browse/KNIDEPLOY-2285 "installer changes to add root device hints"
      downstream on master closed: https://github.com/openshift/installer/pull/3348 "[WIP] baremetal: Allow rootHints to override Host profiles"
      downstream on master merged: https://github.com/openshift/installer/pull/3795 "Bug 1805237: baremetal: Allow rootDeviceHints to override Host profiles"

    Sub-task (Done) https://issues.redhat.com/browse/KNIDEPLOY-2301 "update BMO in CAPBM in master"
      downstream on master merged: https://github.com/openshift/cluster-api-provider-baremetal/pull/74 "update baremetalhost type with root device hints fields"

    Sub-task (Done) https://issues.redhat.com/browse/KNIDEPLOY-2302 "backport BMO changes to 4.5 branch"
      no github links found

    Sub-task (Done) https://issues.redhat.com/browse/KNIDEPLOY-2303 "backport CAPBM changes to 4.5"
      no github links found

    Sub-task (Done) https://issues.redhat.com/browse/KNIDEPLOY-2319 "update CAPBM to look at BMO in openshift org"
      downstream on master merged: https://github.com/openshift/cluster-api-provider-baremetal/pull/76 "update baremetal-operator module source location"

    Sub-task (Done) https://issues.redhat.com/browse/KNIDEPLOY-2320 "test provisioning control plane and workers with non-default root hints"
      no github links found
```
