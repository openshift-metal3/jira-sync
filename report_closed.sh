#!/bin/bash

set -eu

BINDIR=$(dirname $0)
source ${BINDIR}/lib.sh

echo "Starting $(date +%F-%H%M%S)"

SETTINGS_FILE=${SETTINGS_FILE:-$HOME/.jira_sync_settings}
source ${SETTINGS_FILE}

if [ -z "$jira_url" ]; then
    echo "NO JIRA URL SET"
    exit 1
fi

header "Reporting on items closed upstream but not in jira"
go run ./find-closed/main.go \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -bugzilla-url "$bugzilla_url" \
    -github-token "$github_token" \
    -jira-project KNIDEPLOY
