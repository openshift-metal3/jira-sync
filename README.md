# jira-sync

This repo contains tools for syncing data from public sources into a
Jira instance.

For example, this command

```
github-to-jira \
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
