FROM openshift/origin-release:golang-1.13 as builder
RUN mkdir -p /go/src/github.com/openshift-metal3/jira-sync
WORKDIR /go/src/github.com/openshift-metal3/jira-sync
COPY . .
RUN make build

FROM centos:8

ENV HOME /home/jira-sync
RUN mkdir -p /home/jira-sync && \
    chgrp -R 0 /home/jira-sync && \
    chmod -R g=u /home/jira-sync

COPY --from=builder /go/src/github.com/openshift-metal3/jira-sync/bin/* /home/jira-sync/bin/
COPY sync.sh /home/jira-sync/bin/sync.sh

ENTRYPOINT ["/home/jira-sync/bin/sync.sh"]
