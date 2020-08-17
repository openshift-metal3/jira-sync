#!/bin/bash

source $HOME/.jira_sync_settings

cd $(dirname $0)

go run ./pr-check/main.go \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -bugzilla-token "$github_token" \
    $@
