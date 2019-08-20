package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/andygrunwald/go-jira"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	"io/ioutil"
	"os"
)

func main() {
	token := flag.String("github-token", "", "the API token")
	githubOrg := flag.String("github-org", "", "the organization to scan")
	username := flag.String("jira-user", "", "the username")
	password := flag.String("jira-password", "", "the password")
	jiraURL := flag.String("jira-url", "", "the jira server URL")
	jiraProject := flag.String("jira-project", "", "the jira project")
	jiraComponent := flag.String("jira-component", "", "the jira component for new tickets")

	flag.Parse()

	if *token == "" {
		fmt.Fprintf(os.Stderr, "Please provide an API token (-github-token)")
		os.Exit(1)
	}

	if *githubOrg == "" {
		fmt.Fprintf(os.Stderr, "Please specify the -github-org")
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

	if *jiraProject == "" || *jiraComponent == "" {
		fmt.Fprintf(os.Stderr, "Please specify the -jira-project and -jira-component")
		os.Exit(1)
	}

	tp := jira.BasicAuthTransport{
		Username: *username,
		Password: *password,
	}

	jiraClient, err := jira.NewClient(tp.Client(), *jiraURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create client: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *token},
	)
	tc := oauth2.NewClient(ctx, ts)

	ghClient := github.NewClient(tc)

	repos, _, err := ghClient.Repositories.ListByOrg(ctx, *githubOrg, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list %s repositories: %v", *githubOrg, err)
		os.Exit(1)
	}

	ghIssueQueryOpts := github.IssueListByRepoOptions{
		State: "open",
	}

	jiraCreateMeta, _, err := jiraClient.Issue.GetCreateMeta(*jiraProject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch metadata for %s: %s", *jiraProject, err)
		os.Exit(1)
	}
	knideployProject := jiraCreateMeta.GetProjectWithKey(*jiraProject)
	storyIssueType := knideployProject.GetIssueTypeWithName("story")

	for _, repo := range repos {
		issues, _, err := ghClient.Issues.ListByRepo(ctx, *githubOrg, *repo.Name, &ghIssueQueryOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to list issues for %s: %s\n", repo, err)
			os.Exit(1)
		}

		fmt.Printf("\n%s\n", *repo.Name)

		if len(issues) == 0 {
			fmt.Printf("no issues\n")
			continue
		}

		for _, ghIssue := range issues {

			if ghIssue.PullRequestLinks != nil {
				// skip pull requests
				continue
			}

			fmt.Printf("\n%d: [%6s] %s\n\t%s\n",
				*ghIssue.Number, *ghIssue.State, *ghIssue.Title, *ghIssue.HTMLURL)

			slug := fmt.Sprintf("github:%s:%s:%d", *githubOrg, *repo.Name, *ghIssue.Number)

			search := fmt.Sprintf("summary ~ \"%s\" and ( type = story or type = bug )", slug)

			jiraIssues, _, err := jiraClient.Issue.Search(search, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to search for issue: %v", err)
				continue
			}

			if len(jiraIssues) == 0 {
				body := ""
				if ghIssue.Body != nil {
					body = *ghIssue.Body
				}
				issueParams := &jira.Issue{
					Fields: &jira.IssueFields{
						Project: jira.Project{
							Key: *jiraProject,
						},
						Components: []*jira.Component{
							&jira.Component{
								Name: *jiraComponent,
							},
						},
						Type: jira.IssueType{
							Name: storyIssueType.Name,
						},
						Summary: fmt.Sprintf("%s [%s]", *ghIssue.Title, slug),
						Description: fmt.Sprintf("created automatically from %s\n\n%s",
							*ghIssue.HTMLURL, body),
					},
				}
				newJiraIssue, response, err := jiraClient.Issue.Create(issueParams)
				if err != nil {
					text, _ := ioutil.ReadAll(response.Body)
					fmt.Fprintf(os.Stderr, "Failed to create issue: %s\n%s\n", err, text)
					os.Exit(1)
				}
				fmt.Printf("CREATED %s %s/browse/%s\n", newJiraIssue.Key, *jiraURL, newJiraIssue.Key)
			} else {
				for _, jiraIssue := range jiraIssues {
					fmt.Printf("EXISTING [%s] (%s) %s: %+v\n",
						jiraIssue.Fields.Type.Name,
						jiraIssue.Fields.Priority.Name,
						jiraIssue.Key,
						jiraIssue.Fields.Summary,
					)
				}
			}
		}
	}
}
