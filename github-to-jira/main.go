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

type syncArgs struct {
	githubClient      *github.Client
	githubOrg         string
	githubLabel       string
	jiraURL           string
	jiraUser          string
	jiraClient        *jira.Client
	jiraProject       string
	jiraComponent     string
	jiraIssueTypeName string
}

type callback func(syncArgs, *github.Repository) error

func processAllRepositories(args syncArgs, callback callback) error {
	ctx := context.Background()

	opts := github.RepositoryListByOrgOptions{
		Type: "all",
	}

	for {
		page, response, err := args.githubClient.Repositories.ListByOrg(ctx, args.githubOrg, &opts)
		if err != nil {
			return fmt.Errorf("Failed to list %s repositories: %v", args.githubOrg, err)
		}

		for _, repo := range page {
			if err = callback(args, repo); err != nil {
				return err
			}
		}

		if response.NextPage == 0 {
			break
		}
		opts.Page = response.NextPage
	}
	return nil
}

func processSomeRepositories(args syncArgs, callback callback, repoNames []string) error {
	ctx := context.Background()

	for _, repoName := range repoNames {
		repo, _, err := args.githubClient.Repositories.Get(ctx, args.githubOrg, repoName)
		if err != nil {
			return fmt.Errorf("Could not get repository %s/%s: %s", args.githubOrg, repoName, err)
		}
		if err = callback(args, repo); err != nil {
			return err
		}
	}

	return nil
}

func processOneRepository(args syncArgs, repo *github.Repository) error {

	opts := github.IssueListByRepoOptions{
		State: "open",
	}
	if args.githubLabel != "" {
		opts.Labels = append(opts.Labels, args.githubLabel)
	}

	fmt.Printf("\n%s\n", *repo.Name)

	for {
		issues, response, err := args.githubClient.Issues.ListByRepo(
			context.Background(), args.githubOrg, *repo.Name, &opts)
		if err != nil {
			return fmt.Errorf("Failed to list issues for %s: %s\n", repo, err)
		}

		if len(issues) == 0 {
			fmt.Printf("no issues\n")
			break
		}

		for _, ghIssue := range issues {
			if err = processOneIssue(args, repo, ghIssue); err != nil {
				return fmt.Errorf("Failed to process repo %s: %s", *repo.Name, err)
			}
		}

		if response.NextPage == 0 {
			break
		}
		opts.Page = response.NextPage
	}

	return nil
}

func processOneIssue(args syncArgs, repo *github.Repository, ghIssue *github.Issue) error {
	if ghIssue.PullRequestLinks != nil {
		// skip pull requests
		return nil
	}

	fmt.Printf("%s \"%s\"", *ghIssue.HTMLURL, *ghIssue.Title)

	// Build a unique slug to use as a search term to find jira
	// tickets based on the github ticket.
	slug := fmt.Sprintf("github:%s:%s:%d", args.githubOrg, *repo.Name, *ghIssue.Number)

	search := fmt.Sprintf("text ~ \"%s\" and ( type = story or type = bug )", slug)
	jiraIssues, _, err := args.jiraClient.Issue.Search(search, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to search for issue: %v", err)
		return err
	}

	if len(jiraIssues) != 0 {
		for _, jiraIssue := range jiraIssues {
			fmt.Printf(" EXISTING %s %s/browse/%s\n",
				jiraIssue.Fields.Type.Name,
				args.jiraURL,
				jiraIssue.Key,
			)
		}
		return nil
	}

	body := ""
	if ghIssue.Body != nil {
		body = *ghIssue.Body
	}

	// The summary can only be 255 characters, so we have to truncate
	// what we're given if it will be too long with the slug we have
	// to add.
	title := *ghIssue.Title
	if len(title)+len(slug)+6 > 250 {
		// Remove space fo the slug, the space before it, the brackets
		// around it, and the elipsis we add on the following line.
		end := min(250, len(title)) - (len(slug) + 6)
		title = fmt.Sprintf("%s...", title[0:end])
	}
	summary := fmt.Sprintf("%s [%s]", title, slug)

	// Add a line indicating that this ticket was imported
	// automatically to the top of the description. Use italics (wrap
	// in _) and use the slug as the text for the link so that even if
	// someone modifies the summary text we can find this ticket
	// again.
	description := fmt.Sprintf("_created automatically from [%s|%s]_\n\n%s",
		slug, *ghIssue.HTMLURL, body)

	issueParams := &jira.Issue{
		Fields: &jira.IssueFields{
			Project: jira.Project{
				Key: args.jiraProject,
			},
			Components: []*jira.Component{
				&jira.Component{
					Name: args.jiraComponent,
				},
			},
			Type: jira.IssueType{
				Name: args.jiraIssueTypeName,
			},
			Labels:      []string{"github", fmt.Sprintf("%s/%s", args.githubOrg, *repo.Name)},
			Summary:     summary,
			Description: description,
		},
	}
	newJiraIssue, response, err := args.jiraClient.Issue.Create(issueParams)
	if err != nil {
		text, _ := ioutil.ReadAll(response.Body)
		return fmt.Errorf("Failed to create issue: %s\n%s\n", err, text)
	}
	fmt.Printf("CREATED %s %s/browse/%s %s\n",
		newJiraIssue.Key,
		args.jiraURL,
		newJiraIssue.Key,
		summary,
	)

	// Remove the watch from this issue for the user that created it,
	// assuming the user is either a bot or someone who does not
	// actually want to see all notifications for all of the items
	// they import.
	//
	// FIXME: Make this a command line option.
	//
	// FIXME: The client library doesn't construct the remove request
	// properly, so do it ourselves until we can fix that.
	// _, err = args.jiraClient.Issue.RemoveWatcher(newJiraIssue.ID, args.jiraUser)
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "Could not remove watch on %s for %s: %s",
	// 		newJiraIssue.ID, args.jiraUser, err)
	// }
	apiEndPoint := fmt.Sprintf("rest/api/2/issue/%s/watchers?username=%s",
		newJiraIssue.ID, args.jiraUser)
	req, err := args.jiraClient.NewRequest("DELETE", apiEndPoint, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not remove watch: %s\n", err)
		return nil
	}
	_, err = args.jiraClient.Do(req, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not remove watch: %s\n", err)
		return nil
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	token := flag.String("github-token", "", "the API token")
	githubOrg := flag.String("github-org", "", "the organization to scan")
	githubLabel := flag.String("github-label", "", "the issue label for filtering")
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
	jiraCreateMeta, response, err := jiraClient.Issue.GetCreateMeta(*jiraProject)
	defer response.Body.Close()
	if err != nil {
		text, _ := ioutil.ReadAll(response.Body)
		fmt.Fprintf(os.Stderr, "Failed to fetch metadata for %s: %s\n%s", *jiraProject, err, text)
		os.Exit(1)
	}
	knideployProject := jiraCreateMeta.GetProjectWithKey(*jiraProject)
	storyIssueType := knideployProject.GetIssueTypeWithName("story")

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *token},
	)
	tc := oauth2.NewClient(ctx, ts)

	githubClient := github.NewClient(tc)

	args := syncArgs{
		githubClient:      githubClient,
		githubOrg:         *githubOrg,
		githubLabel:       *githubLabel,
		jiraURL:           *jiraURL,
		jiraUser:          *username,
		jiraClient:        jiraClient,
		jiraProject:       *jiraProject,
		jiraComponent:     *jiraComponent,
		jiraIssueTypeName: storyIssueType.Name,
	}

	if len(flag.Args()) > 0 {
		err = processSomeRepositories(args, processOneRepository, flag.Args())
	} else {
		err = processAllRepositories(args, processOneRepository)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v", err)
		os.Exit(1)
	}
}
