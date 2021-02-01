# See pkg/version/version.go for details
GIT_COMMIT=$(shell git rev-parse --verify 'HEAD^{commit}')
LDFLAGS=-ldflags "-X github.com/openshift/hive/pkg/version.Raw=$(shell git describe --always --abbrev=40 --dirty) -X github.com/openshift/hive/pkg/version.Commit=${GIT_COMMIT}"

.PHONY: build
build: bin/github-to-jira bin/bugzilla-to-jira bin/find-closed bin/bugzilla-one bin/pr-check bin/jira-to-github

bin/%: %/main.go
	mkdir -p bin
	go build -o $@ $<
