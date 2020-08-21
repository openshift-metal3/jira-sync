package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/andygrunwald/go-jira"
	"github.com/google/go-github/v32/github"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
)

var pullRequestURLPattern *regexp.Regexp

func init() {
	pullRequestURLPattern = regexp.MustCompile("https://github.com/(?P<org>[^/]+)/(?P<repo>[^/]+)/pull/(?P<id>\\d+)")
}

type jiraSettings struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	URL      string `yaml:"url"`
}

type githubSettings struct {
	Token string `yaml:"token"`
}

type appSettings struct {
	Jira          jiraSettings   `yaml:"jira"`
	Github        githubSettings `yaml:"github"`
	DownstreamOrg string         `yaml:"downstreamOrg"`
	verbose       bool
}

func (settings *appSettings) JiraClient() (*jira.Client, error) {
	tp := jira.BasicAuthTransport{
		Username: settings.Jira.User,
		Password: settings.Jira.Password,
	}

	jiraClient, err := jira.NewClient(tp.Client(), settings.Jira.URL)
	if err != nil {
		return nil, errors.Wrap(err, "could not create jira client")
	}
	return jiraClient, nil
}

func (settings *appSettings) GithubClient() (*github.Client, error) {
	ctx := context.Background()
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: settings.Github.Token},
	)
	oauthClient := oauth2.NewClient(ctx, tokenSource)
	githubClient := github.NewClient(oauthClient)
	return githubClient, nil
}

type repoPRCache struct {
	pullRequests []*github.PullRequest
	commits      map[int][]*github.RepositoryCommit
}

type cache struct {
	pullRequestsByRepo map[string]repoPRCache
	mutex              sync.Mutex
}

type pullRequestWithStatus struct {
	pull   *github.PullRequest
	status string
}

func (pr pullRequestWithStatus) String() string {
	return fmt.Sprintf("on %s %s: %s \"%s\"",
		*pr.pull.Base.Ref,
		pr.status,
		*pr.pull.HTMLURL,
		*pr.pull.Title,
	)
}

type linkResult struct {
	url          string
	org          string
	repo         string
	id           int
	prWithStatus pullRequestWithStatus
	others       []*linkResult
}

type issueResult struct {
	issue       *jira.Issue
	linkResults []*linkResult
	children    []*issueResult
}

const githubPageSize int = 50

func (c *cache) getDetails(settings *appSettings, org, repo string) (*repoPRCache, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	ctx := context.Background()
	repoKey := fmt.Sprintf("%s/%s", org, repo)
	prCache, ok := c.pullRequestsByRepo[repoKey]

	ghClient, err := settings.GithubClient()
	if err != nil {
		return nil, errors.Wrap(err, "could not create github client")
	}

	if !ok {
		prCache = repoPRCache{
			pullRequests: []*github.PullRequest{},
			commits:      make(map[int][]*github.RepositoryCommit),
		}
		c.pullRequestsByRepo[repoKey] = prCache

		opts := &github.PullRequestListOptions{
			State: "all",
			ListOptions: github.ListOptions{
				PerPage: githubPageSize,
			},
		}

		fmt.Printf("building cache of PRs for %s/%s\n", org, repo)
		for {
			prs, response, err := ghClient.PullRequests.List(ctx, org, repo, opts)
			if err != nil {
				return nil, errors.Wrap(err,
					fmt.Sprintf("could not get pull requests for %s", repoKey))
			}
			prCache.pullRequests = append(prCache.pullRequests, prs...)

			for _, pr := range prs {
				commits, _, err := ghClient.PullRequests.ListCommits(
					ctx, org, repo, *pr.Number, nil)
				if err != nil {
					return nil, errors.Wrap(err,
						fmt.Sprintf("could not get commits for pull request %d", *pr.Number))
				}
				prCache.commits[*pr.Number] = commits
			}

			if response.NextPage == 0 {
				break
			}
			opts.Page = response.NextPage
		}
	}
	return &prCache, nil
}

func loadSettings(filename string) (*appSettings, error) {

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	result := appSettings{}
	err = yaml.Unmarshal(content, &result)
	if err != nil {
		return nil, err
	}

	if result.Jira.URL == "" {
		return nil, fmt.Errorf("No jira.url found in %s", filename)
	}
	if result.Jira.User == "" {
		return nil, fmt.Errorf("No jira.user found in %s", filename)
	}
	if result.Jira.Password == "" {
		return nil, fmt.Errorf("No jira.password found in %s", filename)
	}

	if result.Github.Token == "" {
		return nil, fmt.Errorf("No github.token found in %s", filename)
	}

	if result.DownstreamOrg == "" {
		return nil, fmt.Errorf("No downstreamOrg found in %s", filename)
	}

	return &result, nil
}

func issueTitleLine(issue *jira.Issue, jiraURL string) string {
	return fmt.Sprintf("%s (%s) %s/browse/%s %q",
		issue.Fields.Type.Name,
		issue.Fields.Status.Name,
		jiraURL,
		issue.Key,
		issue.Fields.Summary,
	)
}

func uniqueStrings(in []string) []string {
	out := []string{}
	keys := make(map[string]bool)
	for _, s := range in {
		if _, ok := keys[s]; ok {
			continue
		}
		keys[s] = true
		out = append(out, s)
	}
	return out
}

func getLinks(issue *jira.Issue) []string {
	results := []string{}

	results = append(results,
		pullRequestURLPattern.FindAllString(issue.Fields.Description, -1)...)

	if issue.Fields.Comments != nil {
		for _, comment := range issue.Fields.Comments.Comments {
			results = append(results,
				pullRequestURLPattern.FindAllString(comment.Body, -1)...)
		}
	}

	return results
}

func getPRStatus(settings *appSettings, ghClient *github.Client, pullRequest *github.PullRequest) (pullRequestWithStatus, error) {
	result := pullRequestWithStatus{pull: pullRequest}
	ctx := context.Background()
	result.status = *pullRequest.State
	isMerged, _, err := ghClient.PullRequests.IsMerged(ctx,
		*pullRequest.Base.Repo.Owner.Login, *pullRequest.Base.Repo.Name, *pullRequest.Number)
	if err != nil {
		return result, errors.Wrap(err,
			fmt.Sprintf("could not fetch merge status of pull request %d",
				*pullRequest.Number))
	}
	if isMerged {
		result.status = "merged"
	}
	if result.status == "open" {
		result.status = "OPEN"
	}
	return result, nil
}

func parsePRURL(url string) (org, repo string, id int, err error) {
	match := pullRequestURLPattern.FindStringSubmatch(url)
	org = match[1]
	repo = match[2]
	idStr := match[3]
	id, err = strconv.Atoi(idStr)
	if err != nil {
		err = errors.Wrap(err,
			fmt.Sprintf("could not convert pull request id %q to integer", idStr))
	}
	return
}

func processOneLink(settings *appSettings, cache *cache, url string) (*linkResult, error) {
	if settings.verbose {
		fmt.Fprintf(os.Stderr, "getting details for %s\n", url)
	}

	ghClient, err := settings.GithubClient()
	if err != nil {
		return nil, errors.Wrap(err, "could not create github client")
	}

	result := &linkResult{
		url: url,
	}
	ctx := context.Background()

	// parse the URL to find the args we need for interacting with
	// github's API
	org, repo, id, err := parsePRURL(url)
	if err != nil {
		return nil, errors.Wrap(err,
			fmt.Sprintf("could not parse pull request URL %q", url))
	}
	result.org = org
	result.repo = repo
	result.id = id

	pullRequest, _, err := ghClient.PullRequests.Get(ctx,
		result.org, result.repo, result.id)
	if err != nil {
		return nil, errors.Wrap(err,
			fmt.Sprintf("could not fetch pull request %q", url))
	}

	prWithStatus, err := getPRStatus(settings, ghClient, pullRequest)
	if err != nil {
		return nil, errors.Wrap(err,
			fmt.Sprintf("could not get status of %s", *pullRequest.HTMLURL))
	}
	result.prWithStatus = prWithStatus

	if result.org == settings.DownstreamOrg {
		return result, nil
	}

	if prWithStatus.status == "closed" {
		// We don't care if there is no matching downstream PR if
		// we closed the upstream one without merging it.
		return result, nil
	}

	commits, _, err := ghClient.PullRequests.ListCommits(
		ctx, result.org, result.repo, result.id, nil)
	if err != nil {
		return nil, errors.Wrap(err,
			fmt.Sprintf("could not list commits in pull request %q", url))
	}

	otherIDs := make(map[int]bool)
	otherLinks := []string{}
	for _, c := range commits {

		// look for pull requests containing the same commits via
		// the github API
		otherPRs, response, err := ghClient.PullRequests.ListPullRequestsWithCommit(
			ctx, settings.DownstreamOrg, result.repo, *c.SHA, nil)
		if err != nil {
			if response.StatusCode == http.StatusNotFound {
				// The repository hasn't been forked downstream. Treat
				// it as not an error and break out of this loop.
				if settings.verbose {
					fmt.Fprintf(os.Stderr, "no downstream repository %s/%s, skipping\n",
						settings.DownstreamOrg, result.repo)
				}
				break
			}
			return nil, errors.Wrap(err, "could not find downstream pull requests")
		}

		for _, otherPR := range otherPRs {
			if *otherPR.HTMLURL == url {
				// the API returns our own PR even when we ask
				// for the ones from the downstream PR
				continue
			}
			if _, ok := otherIDs[*otherPR.Number]; ok {
				// ignore duplicate PRs
				continue
			}
			otherIDs[*otherPR.Number] = true
			otherLinks = append(otherLinks, *otherPR.HTMLURL)
		}

		// look in the cache for commit messages that include the
		// SHA, indicating a reference during a cherry-pick
		if len(otherIDs) == 0 {
			cachedDetails, err := cache.getDetails(settings, settings.DownstreamOrg, repo)
			if err != nil {
				return nil, errors.Wrap(err,
					fmt.Sprintf("could not build cache of details for %s/%s",
						settings.DownstreamOrg, repo))
			}
			for _, pr := range cachedDetails.pullRequests {
				for _, otherCommit := range cachedDetails.commits[*pr.Number] {
					if strings.Contains(*otherCommit.Commit.Message, *c.SHA) {
						if _, ok := otherIDs[*pr.Number]; ok {
							// ignore duplicate PRs
							continue
						}
						otherLinks = append(otherLinks, *pr.HTMLURL)
					}
				}
			}
		}
	}
	if len(otherLinks) > 0 {
		otherResults, err := processLinks(settings, cache, otherLinks)
		if err != nil {
			return nil, errors.Wrap(err,
				fmt.Sprintf("could not process %s", url))
		}
		result.others = otherResults
	}

	return result, nil
}

func processLinks(settings *appSettings, cache *cache, links []string) ([]*linkResult, error) {

	var wg sync.WaitGroup
	resultChan := make(chan *linkResult)

	for _, url := range uniqueStrings(links) {
		wg.Add(1)
		go func(url string, ch chan<- *linkResult) {
			defer wg.Done()
			result, err := processOneLink(settings, cache, url)
			if err != nil {
				fmt.Printf("failed to get details for %s: %s\n", url, err)
				return
			}
			ch <- result
		}(url, resultChan)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	resultsByURL := make(map[string]*linkResult)
	for result := range resultChan {
		resultsByURL[result.url] = result
	}
	results := []*linkResult{}
	for _, url := range uniqueStrings(links) {
		if result, ok := resultsByURL[url]; ok {
			results = append(results, result)
		}
	}
	return results, nil
}

func showLinkResults(settings *appSettings, results []*linkResult, indent string) {
	for _, result := range results {

		if result.org == settings.DownstreamOrg {
			fmt.Printf("%sdownstream %s\n", indent, result.prWithStatus)
			continue
		}

		fmt.Printf("%supstream %s\n", indent, result.prWithStatus)

		if result.prWithStatus.status == "closed" {
			// We don't care if there is no matching downstream PR if
			// we closed the upstream one without merging it.
			continue
		}

		if len(result.others) == 0 {
			fmt.Printf("%s  downstream: no matching pull requests found in %s/%s\n",
				indent, settings.DownstreamOrg, result.repo,
			)
			continue
		}
		showLinkResults(settings, result.others, indent+"  ")
	}
}

func processOneIssue(settings *appSettings, cache *cache, issueID string) (*issueResult, error) {
	if settings.verbose {
		fmt.Fprintf(os.Stderr, "getting details for %s\n", issueID)
	}

	jiraClient, err := settings.JiraClient()
	if err != nil {
		return nil, errors.Wrap(err, "could not create jira client")
	}

	result := &issueResult{}

	issue, _, err := jiraClient.Issue.Get(issueID, nil)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("error processing issue %q", issueID))
	}
	result.issue = issue

	links := getLinks(issue)
	if len(links) != 0 {
		linkResults, err := processLinks(settings, cache, links)
		if err != nil {
			return nil, errors.Wrap(err,
				fmt.Sprintf("failed processing links in %s", issueID))
		}
		result.linkResults = linkResults
	}

	switch issue.Fields.Type.Name {
	case "Epic", "Feature":
		searchOptions := jira.SearchOptions{
			Expand: "comments",
		}
		searchTerm := "Epic Link"
		if issue.Fields.Type.Name == "Feature" {
			searchTerm = "Parent Link"
		}
		search := fmt.Sprintf("\"%s\" = %s", searchTerm, issueID)
		children, _, err := jiraClient.Issue.Search(search, &searchOptions)
		if err != nil {
			return nil, errors.Wrap(err,
				fmt.Sprintf("could not find sub-issues related to %s", issueID))
		}

		childIDs := make([]string, len(children))
		for i, child := range children {
			childIDs[i] = child.Key
		}
		result.children = processIssues(settings, cache, childIDs)
	case "Story":
		childIDs := make([]string, len(issue.Fields.Subtasks))
		for i, task := range issue.Fields.Subtasks {
			childIDs[i] = task.Key
		}
		result.children = processIssues(settings, cache, childIDs)
	default:
	}
	return result, nil
}

func showOneIssueResult(settings *appSettings, result *issueResult, indent string) {
	fmt.Printf("\n%s%s\n", indent, issueTitleLine(result.issue, settings.Jira.URL))
	if len(result.linkResults) == 0 {
		fmt.Printf("%s  no github links found\n", indent)
	} else {
		showLinkResults(settings, result.linkResults, indent+"  ")
	}
	for _, child := range result.children {
		showOneIssueResult(settings, child, indent+"  ")
	}
}

// fileExists checks if a file exists and is not a directory before we
// try using it to prevent further errors.
func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func processIssues(settings *appSettings, cache *cache, ids []string) []*issueResult {

	var wg sync.WaitGroup
	resultChan := make(chan *issueResult)

	for _, issueID := range uniqueStrings(ids) {
		wg.Add(1)
		go func(issueID string, ch chan<- *issueResult) {
			defer wg.Done()
			result, err := processOneIssue(settings, cache, issueID)
			if err != nil {
				fmt.Printf("ERROR: %s\n", err)
				return
			}
			ch <- result
		}(issueID, resultChan)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Hold the results in a map until we have them all, then return
	// them in the order the caller asked for them.
	resultsByID := make(map[string]*issueResult)
	for result := range resultChan {
		resultsByID[result.issue.Key] = result
	}
	results := []*issueResult{}
	for _, id := range uniqueStrings(ids) {
		if result, ok := resultsByID[id]; ok {
			results = append(results, result)
		}
	}
	return results
}

func main() {
	token := flag.String("github-token", "", "the API token")
	username := flag.String("jira-user", "", "the username")
	password := flag.String("jira-password", "", "the password")
	downstreamOrg := flag.String("downstream-org", "openshift",
		"the downstream github organization")
	jiraURL := flag.String("jira-url", "", "the jira server URL")
	verbose := flag.Bool("v", false, "verbose mode")

	flag.Parse()

	if *token == "" {
		fmt.Fprintf(os.Stderr, "Please provide an API token (-bugzilla-token)")
		os.Exit(1)
	}

	if *username == "" || *password == "" {
		fmt.Fprintf(os.Stderr, "Please specify both username (-jira-user) and password (-jira-password)")
		os.Exit(1)
	}

	if *jiraURL == "" {
		fmt.Fprintf(os.Stderr, "Please specify the -jira-url")
		os.Exit(1)
	}

	settings := &appSettings{
		Jira: jiraSettings{
			User:     *username,
			Password: *password,
			URL:      *jiraURL,
		},
		Github: githubSettings{
			Token: *token,
		},
		DownstreamOrg: *downstreamOrg,
		verbose:       *verbose,
	}

	_, err := settings.JiraClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create jira client: %v\n", err)
		os.Exit(1)
	}

	_, err = settings.GithubClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create githubclient: %v\n", err)
		os.Exit(1)
	}

	cache := &cache{
		pullRequestsByRepo: make(map[string]repoPRCache),
	}

	results := processIssues(settings, cache, flag.Args())
	for _, result := range results {
		showOneIssueResult(settings, result, "")
	}

}
