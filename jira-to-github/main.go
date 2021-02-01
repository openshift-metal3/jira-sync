package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "strings"
    "time"

    "github.com/andygrunwald/go-jira"
    "github.com/google/go-github/github"
    "golang.org/x/oauth2"
)

type syncArgs struct {
	githubClient      *github.Client
	githubOrg         string
	githubRepo        []string
	githubLabel       string
	githubIgnore      []string
    jiraURL           string
    jiraUser          string
    jiraClient        *jira.Client
    jiraProject       string
    dryrun            bool
}

type issue struct {
	ID     int            `json:"id"`
    Title  string         `json:"title"`
    Body   string         `json:"body"`
	State  string         `json:"state"`
	Labels []github.Label `json:"labels,omitempty"`
}

func processAllIssues(args syncArgs) error {

    fmt.Printf("Fetching Jira issues ... ")

	var jiraIssues []jira.Issue

	// appendFunc will append jira issues to []jira.Issue
	appendFunc := func(i jira.Issue) (err error) {
		jiraIssues = append(jiraIssues, i)
		return err
	}

	// SearchPages will page through results and pass each issue to appendFunc
    jql := fmt.Sprintf(`project = %s AND issuetype = Epic AND labels in ("gh-sync")`, strings.TrimSpace(args.jiraProject))
	err := args.jiraClient.Issue.SearchPages(jql, nil, appendFunc)
	if err != nil {
		return fmt.Errorf("Could not fetch jira issues: %s\n", err)
	}

    fmt.Printf("%d Jira issues found.\n", len(jiraIssues))
    
    var githubIssues []issue
	for _, githubRepo := range args.githubRepo {

		opts := github.IssueListByRepoOptions{
			State: "open",
		}
		if args.githubLabel != "" {
			opts.Labels = append(opts.Labels, args.githubLabel)
		}

		for {
			fmt.Println("Fetching Github issues ...")

			issues, response, err := args.githubClient.Issues.ListByRepo(
				context.Background(), args.githubOrg, githubRepo, &opts)
			if err != nil {
				return fmt.Errorf("Failed to list issues for %s: %s", githubRepo, err)
			}

			if len(issues) == 0 {
				fmt.Println("no Github issues.")
				break
			}

			for _, ghIssue := range issues {
				githubIssues = append(githubIssues, issue{
					ID:     *ghIssue.Number,
                    Title:  *ghIssue.Title,
                    Body:   *ghIssue.Body,
					State:  *ghIssue.State,
					Labels: *&ghIssue.Labels,
				})
			}

			if response.NextPage == 0 {
				break
			}
			opts.Page = response.NextPage
		}

		for _, issue := range jiraIssues {
            fmt.Printf("\nProcessing Issue, Key: %s\nIssue Summary: %s\n", issue.Key, issue.Fields.Summary)
			if err := processOneIssue(args, &issue, githubIssues, githubRepo); err != nil {
				return fmt.Errorf("Failed to process issue %s: %s", issue.Key, err)
			}
        }
	}

    return nil
}

func processOneIssue(args syncArgs, jiraIssue *jira.Issue, issues []issue, githubRepo string) error {

    time.Sleep(1 * time.Second)
    
    fmt.Printf("Jira %s link: %s/browse/%s\n",
		jiraIssue.Fields.Type.Name,
		args.jiraURL,
		jiraIssue.Key,
	)

    jiraURL := fmt.Sprintf("%s/browse/%s", args.jiraURL, jiraIssue.Key)
    jiraStatus := jiraIssue.Fields.Status.Name

	foundOnGithub := false

	for _, issue := range issues {
		if !strings.Contains(issue.Body, jiraURL) {
			continue
        }
        
        foundOnGithub = true
        fmt.Printf("Github found %d\n", issue.ID)

		issueURL := fmt.Sprintf("https://github.com/%s/%s/issues/%d", args.githubOrg, githubRepo, issue.ID)

		// make sure there is a github link in jira
		if !strings.Contains(jiraIssue.Fields.Description, issueURL) {

			if args.dryrun {
				fmt.Printf("ACTIONREQUIRED Jira %s does not contain external link to Github %s\n", jiraURL, issueURL)
			} else {
				err := addGithubExternalLinkToJira(args, jiraIssue, jiraURL, issueURL)
				if err != nil {
					return err
				}
			}
		} else {
            fmt.Printf("Github link %s found in Jira issue\n", issueURL)
        }
	}

	if !foundOnGithub {
		if args.dryrun {
			fmt.Printf("ACTIONREQUIRED Failed to find a Github issue for Jira issue having %s status\n", jiraStatus)
		} else {
			err := createGithubIssue(args, jiraIssue, jiraURL, githubRepo)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func addGithubExternalLinkToJira(args syncArgs, jiraIssue *jira.Issue, jiraURL string, issueURL string) error {
	if args.dryrun {
		return nil
    }

    desc := fmt.Sprintf("GH Epic - %s \n\n %s", issueURL, jiraIssue.Fields.Description)
    jsonMap := map[string]interface{}{
		"fields": map[string]string{
			"description": desc,
		},
	}
    _, err := args.jiraClient.Issue.UpdateIssue(jiraIssue.ID, jsonMap)
    if err != nil {
        return fmt.Errorf("ERROR updating jira issue %s: %s\n", jiraIssue.Key, err)
    }

	fmt.Printf("MODIFIED Jira %s adding external link to Github %s\n", jiraURL, issueURL)

	return nil
}

func updateJiraStatus(status string, args syncArgs, jiraIssue *jira.Issue) error {
	if args.dryrun {
		return nil
    }

	currentStatus := jiraIssue.Fields.Status.Name
	fmt.Printf("Current status: %s\n", currentStatus)

	var transitionID string
    possibleTransitions, _, err := args.jiraClient.Issue.GetTransitions(jiraIssue.ID)
    if err != nil {
		return err
	}
	for _, v := range possibleTransitions {
		if v.Name == status {
			transitionID = v.ID
			break
		}
	}

    _, err = args.jiraClient.Issue.DoTransition(jiraIssue.ID, transitionID)
    if err != nil {
		return err
	}
    issue, _, _ := args.jiraClient.Issue.Get(jiraIssue.ID, nil)
    if err != nil {
		return err
	}
	fmt.Printf("Status after transition: %+v\n", issue.Fields.Status.Name)

	return nil
}

func createGithubIssue(args syncArgs, jiraIssue *jira.Issue, jiraURL string, githubRepo string) error {
	if args.dryrun {
		return nil
	}

	newIssueTitle := jiraIssue.Fields.Summary
	newIssueBody := "Jira entry " + jiraURL + " \n\n " + jiraIssue.Fields.Description
	var newLabels []string
    var newAssignees []string
    var milestone string

    // set the Github milestone based on Jira milestone
	for _, version := range jiraIssue.Fields.FixVersions {
		milestone = version.Name
    }
    var milestoneID *int
    if milestone != "" { 
        opts := github.MilestoneListOptions{
			State: "open",
        }
        for {
            // fmt.Println("Fetching Github milestones ...")
            possibleMilestones, response, err := args.githubClient.Issues.ListMilestones(context.Background(), args.githubOrg, githubRepo, &opts)
            if err != nil {
                return err
            }

            for _ ,v := range possibleMilestones {
                if *(v.Title) == milestone {
                    milestoneID = new(int)
                    milestoneID = v.Number
                    break
                }
            }

            if response.NextPage == 0 || milestoneID != nil {
                break
            }
            opts.Page = response.NextPage
        }
    }

	switch jiraIssue.Fields.Priority.Name {
	case "Blocker":
		newLabels = append(newLabels, "blocker (P0)", "jira-blocker")
	case "Urgent":
        newLabels = append(newLabels, "Priority/P1")
    case "High":
		newLabels = append(newLabels, "Priority/P1")
	case "Medium":
		newLabels = append(newLabels, "Priority/P2")
	case "Low":
		newLabels = append(newLabels, "Priority/P3")
	}

	newLabels = append(newLabels, "Epic", "jira-epic")

	for _, component := range jiraIssue.Fields.Components {
		switch component.Name {
		case "app lifecycle":
			newLabels = append(newLabels, "squad:app-lifecycle", "Pillar: Application Lifecycle", "Theme: App & Policy Mgmt")
            newAssignees = append(newAssignees, "jnpacker")
		case "devops":
			newLabels = append(newLabels, "squad:cicd", "Pillar: DevOps", "Theme: Raising the Bar")
            newAssignees = append(newAssignees, "tpouyer")
		case "cluster lifecycle":
			newLabels = append(newLabels, "squad:cluster-lifecycle", "Pillar: Cluster Lifecycle", "Theme: OpenShift Everywhere")
            newAssignees = append(newAssignees, "jnpacker")
		case "observability":
			newLabels = append(newLabels, "squad:core-services", "squad:observability", "Pillar: Observability", "Theme: Observability")
            newAssignees = append(newAssignees, "randymgeorge")
		case "grc":
			newLabels = append(newLabels, "squad:policy-grc", "Pillar: GRC", "Theme: App & Policy Mgmt")
            newAssignees = append(newAssignees, "jrnp7")
		}
    }
    
    for _, label := range jiraIssue.Fields.Labels {
		newLabels = append(newLabels, label)
	}

	if newAssignees == nil {
        newAssignees = append(newAssignees, "berenss", "jeff-brent")
	}

	newIssueRequest := github.IssueRequest{
		Title:     &newIssueTitle,
		Body:      &newIssueBody,
		Assignees: &newAssignees,
        Labels:    &newLabels,
    }
    if milestoneID != nil {
        newIssueRequest.Milestone = milestoneID
    }

	newIssue, _, err := args.githubClient.Issues.Create(context.Background(), args.githubOrg, githubRepo, &newIssueRequest)
	if err != nil {
		return err
    }

    issueURL := fmt.Sprintf("https://github.com/%s/%s/issues/%d", args.githubOrg, githubRepo, *newIssue.Number)
	fmt.Printf("CREATED Github issue %s for Jira %s %s\n", issueURL, jiraIssue.Fields.Status.Name, jiraURL)

    // add the github link in jira description
    err = addGithubExternalLinkToJira(args, jiraIssue, jiraURL, issueURL)
    if err != nil {
        return err
    }

    // The search results do not include comments, so we have to
	// fetch tickets when we need the comments.
	commentedIssue, _, err := args.jiraClient.Issue.Get(jiraIssue.Key, nil)
	if err != nil {
        return err
	}

    if commentedIssue.Fields.Comments != nil {
        for _, comment := range commentedIssue.Fields.Comments.Comments {
            newIssueComment := github.IssueComment{
                Body: &(comment.Body),
            }
            _, _, err = args.githubClient.Issues.CreateComment(context.Background(), args.githubOrg, githubRepo, *newIssue.Number, &newIssueComment)
            if err != nil {
                return err
            }
        }
    }

	return nil
}

func main() {
	githubToken := flag.String("github-token", "", "the github API token")
	githubOrg := flag.String("github-org", "", "the organization to scan")
	githubRepo := flag.String("github-repo", "", "comma separated names of repos to include")
	githubLabel := flag.String("github-label", "", "the issue label for filtering")
	githubIgnore := flag.String("github-ignore", "", "comma separated names of repos to ignore")
    jiraUsername := flag.String("jira-user", "", "the jira username")
    jiraPassword := flag.String("jira-password", "", "the jira password")
    jiraURL := flag.String("jira-url", "", "the jira server URL")
    jiraProject := flag.String("jira-project", "", "the jira project key")
    dryrun := flag.Bool("dryrun", true, "perform a dryrun which doesn't make any changes")

    flag.Parse()

	if *githubToken == "" {
		fmt.Fprintf(os.Stderr, "Please provide a Github API token (-github-token)\n")
		os.Exit(1)
	}

	if *githubOrg == "" {
		fmt.Fprintf(os.Stderr, "Please provide a Github organization (-github-org)\n")
		os.Exit(1)
	}

	if *githubRepo == "" {
		fmt.Fprintf(os.Stderr, "Please provide a comma separated names of Github repos to include  (-github-repo)\n")
		os.Exit(1)
	}

    if *jiraUsername == "" || *jiraPassword == "" {
        fmt.Fprintf(os.Stderr, "Please specify both username (-jira-user) and password (-jira-password)")
        os.Exit(1)
    }

    if *jiraURL == "" {
        fmt.Fprintf(os.Stderr, "Please specify the -jira-url")
        os.Exit(1)
    }

    if *jiraProject == "" {
        fmt.Fprintf(os.Stderr, "Please specify the -jira-project")
        os.Exit(1)
    }

    tp := jira.BasicAuthTransport{
        Username: *jiraUsername,
        Password: *jiraPassword,
    }

    jiraClient, err := jira.NewClient(tp.Client(), *jiraURL)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Could not create client: %v", err)
        os.Exit(1)
    }

    ctx := context.Background()
    ts := oauth2.StaticTokenSource(
        &oauth2.Token{AccessToken: *githubToken},
    )
    tc := oauth2.NewClient(ctx, ts)

    githubClient := github.NewClient(tc)

    args := syncArgs{
		githubClient:      githubClient,
		githubOrg:         *githubOrg,
		githubLabel:       *githubLabel,
		githubRepo:        strings.Split(*githubRepo, ","),
		githubIgnore:      strings.Split(*githubIgnore, ","),
        jiraURL:           *jiraURL,
        jiraUser:          *jiraUsername,
        jiraClient:        jiraClient,
        jiraProject:       *jiraProject,
        dryrun:            *dryrun,
    }

	err = processAllIssues(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
