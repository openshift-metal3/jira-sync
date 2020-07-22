package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/andygrunwald/go-jira"
)

type syncArgs struct {
	bugzillaURL       string
	bugzillaIDs       []string
	bugzillaToken     string
	jiraURL           string
	jiraUser          string
	jiraClient        *jira.Client
	jiraProject       string
	jiraComponent     string
	jiraIssueTypeName string
}

type bug struct {
	ID          int    `json:"id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}

type bugSet struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
	Bugs    []bug  `json:"bugs"`
}

func processAllIssues(args syncArgs) error {

	parsedURL, err := url.Parse(args.bugzillaURL)
	if err != nil {
		return fmt.Errorf("Unable to parse bugzillaURL %s: %s", args.bugzillaURL, err)
	}

	for _, bugID := range args.bugzillaIDs {
		q := url.Values{}
		q.Set("include_fields", "id,summary,description")
		parsedURL.RawQuery = q.Encode()
		parsedURL.Path = fmt.Sprintf("%s/rest/bug/%s", parsedURL.Path, bugID)

		client := http.Client{
			Timeout: time.Second * 20,
		}

		req, err := http.NewRequest(http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			return fmt.Errorf("Unable to build request: %s", err)
		}
		req.Header.Set("User-Agent", "jira-sync")

		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("Unable to query bugzilla: %s", err)
		}

		body, err := ioutil.ReadAll(res.Body)
		if err != nil {
			return err
		}

		theBugs := bugSet{}
		err = json.Unmarshal(body, &theBugs)
		if err != nil {
			return fmt.Errorf("Unable to parse bugzilla response for description: %s: %s", body, err)
		}

		if theBugs.Error {
			return errors.New(theBugs.Message)
		}

		for _, bug := range theBugs.Bugs {
			if err := processOneIssue(args, bug); err != nil {
				return err
			}
		}
	}

	return nil
}

func processOneIssue(args syncArgs, bug bug) error {

	bugDisplayURL := fmt.Sprintf("%s/show_bug.cgi?id=%d", args.bugzillaURL, bug.ID)
	fmt.Printf("%s \"%s\"", bugDisplayURL, bug.Summary)

	// Build a unique slug to use as a search term to find jira
	// tickets based on the bugzilla ticket.
	slug := fmt.Sprintf("bugzilla:%d", bug.ID)

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

	// The summary can only be 255 characters, so we have to truncate
	// what we're given if it will be too long with the slug we have
	// to add.
	title := bug.Summary
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
	description := fmt.Sprintf(
		"_created automatically from [%s|%s]_\n\n{noformat}\n%s\n{noformat}",
		slug, bugDisplayURL, bug.Description)

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
			Labels:      []string{"bugzilla"},
			Summary:     summary,
			Description: description,
		},
	}
	newJiraIssue, response, err := args.jiraClient.Issue.Create(issueParams)
	if err != nil {
		text, _ := ioutil.ReadAll(response.Body)
		return fmt.Errorf("Failed to create issue: %s\n%s\n", err, text)
	}
	fmt.Printf(" CREATED %s %s/browse/%s %s\n",
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
	bugzillaURL := flag.String("bugzilla-url", "", "the base URL for the bugzilla server")
	token := flag.String("bugzilla-token", "", "the API token")
	username := flag.String("jira-user", "", "the username")
	password := flag.String("jira-password", "", "the password")
	jiraURL := flag.String("jira-url", "", "the jira server URL")
	jiraProject := flag.String("jira-project", "", "the jira project")
	jiraComponent := flag.String("jira-component", "", "the jira component for new tickets")

	flag.Parse()

	if *bugzillaURL == "" {
		fmt.Fprintf(os.Stderr, "Please provide an API token (-bugzilla-url)")
		os.Exit(1)
	}

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

	if *jiraProject == "" || *jiraComponent == "" {
		fmt.Fprintf(os.Stderr, "Please specify the -jira-project and -jira-component")
		os.Exit(1)
	}

	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "Please specify the bugzilla IDs as arguments")
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
	jiraCreateMeta, _, err := jiraClient.Issue.GetCreateMeta(*jiraProject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch metadata for %s: %s", *jiraProject, err)
		os.Exit(1)
	}
	knideployProject := jiraCreateMeta.GetProjectWithKey(*jiraProject)
	bugIssueType := knideployProject.GetIssueTypeWithName("bug")

	args := syncArgs{
		bugzillaURL:       *bugzillaURL,
		bugzillaIDs:       flag.Args(),
		bugzillaToken:     *token,
		jiraURL:           *jiraURL,
		jiraUser:          *username,
		jiraClient:        jiraClient,
		jiraProject:       *jiraProject,
		jiraComponent:     *jiraComponent,
		jiraIssueTypeName: bugIssueType.Name,
	}

	err = processAllIssues(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
