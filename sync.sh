#!/bin/bash

set -eu

BINDIR=$(dirname $0)

echo "Starting $(date +%F-%H%M%S)"

function finished {
    set +x
    echo
    echo "Finished $(date +%F-%H%M%S)"
}
trap finished EXIT

function header {
    local msg="$1"
    echo
    echo "$msg" | sed 's/./=/g'
    echo $msg
    echo "$msg" | sed 's/./=/g'
    echo
}

# header "Removing old logs"
# find $LOGDIR -ctime 7 -print -exec rm '{}' \;

github_to_jira=$BINDIR/github-to-jira
bugzilla_to_jira=$BINDIR/bugzilla-to-jira
find_closed=$BINDIR/find-closed

SETTINGS_FILE=${SETTINGS_FILE:-/etc/jira-sync/settings.sh}

source ${SETTINGS_FILE}

if [ -z "$jira_url" ]; then
    echo "NO JIRA URL SET"
    exit 1
fi

header "Importing all items from openshift forks of metal3 repos for the hardware team"
$github_to_jira \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -github-token "$github_token" \
    -jira-project KNIDEPLOY \
    -jira-component 'KNI Deploy HW Mgmt' \
    \
    -github-org openshift \
    ironic \
    ironic-hardware-inventory-recorder-image \
    ironic-image \
    ironic-inspector \
    ironic-inspector-image \
    ironic-ipa-downloader \
    ironic-lib \
    ironic-prometheus-exporter \
    ironic-rhcos-downloader \
    ironic-static-ip-manager \
    metal3-smart-exporter

header "Importing all items from openshift forks of metal3 repos for installer team"
$github_to_jira \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -github-token "$github_token" \
    -jira-project KNIDEPLOY \
    -jira-component 'KNI Deploy Install' \
    \
    -github-org openshift \
    baremetal-operator \
    cluster-api-provider-baremetal

header "Importing openshift items tagged platform/baremetal"
$github_to_jira \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -github-token "$github_token" \
    -jira-project KNIDEPLOY \
    -jira-component 'KNI Deploy Install' \
    \
    -github-org openshift \
    -github-label 'platform/baremetal'

header "Importing metal3-io items for the hardware team"
$github_to_jira \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -github-token "$github_token" \
    -jira-project KNIDEPLOY \
    -jira-component 'KNI Deploy HW Mgmt' \
    \
    -github-org metal3-io \
    ironic \
    ironic-hardware-inventory-recorder-image \
    ironic-image \
    ironic-inspector-image \
    ironic-ipa-downloader \
    ironic-prometheus-exporter \
    metal3-smart-exporter

header "Importing metal3-io items"
$github_to_jira \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -github-token "$github_token" \
    -jira-project KNIDEPLOY \
    -jira-component 'KNI Deploy Install' \
    \
    -github-org metal3-io \
    -github-ignore metal3-io.github.io,cluster-api-provider-metal3,hardware-classification-controller

header "Importing openshift-metal3 items for the UX team"
$github_to_jira \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -github-token "$github_token" \
    -jira-project KNIDEPLOY \
    -jira-component 'KNI Deploy UI & Validations' \
    \
    -github-org openshift-metal3 \
    facet

header "Importing openshift-metal3 items"
$github_to_jira \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -github-token "$github_token" \
    -jira-project KNIDEPLOY \
    -jira-component 'KNI Deploy Install' \
    \
    -github-org openshift-metal3

# header "Importing bugzilla 'KNI Deploy Install' items"
# $bugzilla_to_jira \
#     -jira-user "$jira_user" \
#     -jira-password "$jira_password" \
#     -jira-url "$jira_url" \
#     -bugzilla-token "$bugzilla_token" \
#     -bugzilla-url "$bugzilla_url" \
#     -jira-project KNIDEPLOY \
#     -jira-component 'KNI Deploy Install' \
#     \
#     -bugzilla-product 'Kubernetes-native Infrastructure' \
#     -bugzilla-component 'Deployment'

header "Reporting on items closed upstream but not in jira"
$find_closed \
    -jira-user "$jira_user" \
    -jira-password "$jira_password" \
    -jira-url "$jira_url" \
    -bugzilla-url "$bugzilla_url" \
    -github-token "$github_token" \
    -jira-project KNIDEPLOY
