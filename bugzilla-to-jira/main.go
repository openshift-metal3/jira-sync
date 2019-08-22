package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/andygrunwald/go-jira"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"
)

type syncArgs struct {
	bugzillaURL       string
	bugzillaProduct   string
	bugzillaToken     string
	jiraURL           string
	jiraClient        *jira.Client
	jiraProject       string
	jiraComponent     string
	jiraIssueTypeName string
}

type bug struct {
	ID          int    `json:"id"`
	Summary     string `json:"summary"`
	Description string // not in the json from the original query
}

type bugSet struct {
	Bugs []bug `json:"bugs"`
}

func processAllIssues(args syncArgs) error {

	parsedURL, err := url.Parse(args.bugzillaURL)
	if err != nil {
		return fmt.Errorf("Unable to parse bugzillaURL %s: %s", args.bugzillaURL, err)
	}

	q := url.Values{}
	q.Set("product", args.bugzillaProduct)
	q.Add("status", "NEW")
	q.Add("status", "ASSIGNED")
	q.Add("status", "POST")
	q.Add("status", "MODIFIED")
	q.Add("status", "ON_DEV")
	q.Add("status", "ON_QA")
	q.Add("status", "VERIFIED")
	q.Add("status", "RELEASE_PENDING")
	q.Set("include_fields", "id,summary")
	parsedURL.RawQuery = q.Encode()
	parsedURL.Path = fmt.Sprintf("%s/rest/bug", parsedURL.Path)

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

	for _, bug := range theBugs.Bugs {
		bug.Description, err = fetchDescriptionForBug(args, bug)
		if err != nil {
			return fmt.Errorf("Failed to fetch description of bug %d: %s", bug.ID, err)
		}
		if err := processOneIssue(args, bug); err != nil {
			return err
		}
	}

	return nil
}

func fetchDescriptionForBug(args syncArgs, bug bug) (string, error) {
	commentURL := fmt.Sprintf("%s/rest/bug/%d/comment", args.bugzillaURL, bug.ID)

	client := http.Client{
		Timeout: time.Second * 5,
	}

	req, err := http.NewRequest(http.MethodGet, commentURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "jira-sync")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}

	commentsBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	// A response looks like:
	// {
	// 	"bugs": {
	// 		"35": {
	// 			"comments": [
	// 				{
	// 					"time": "2000-07-25T13:50:04Z",
	// 					"text": "test bug to fix problem in removing from cc list.",
	// 					"bug_id": 35,
	// 					"count": 0,
	// 					"attachment_id": null,
	// 					"is_private": false,
	// 					"tags": [],
	// 					"creator": "user@bugzilla.org",
	// 					"creation_time": "2000-07-25T13:50:04Z",
	// 					"id": 75
	// 				}
	// 			]
	// 		}
	// 	},
	// 	"comments": {}
	// }
	//
	// This poses a challenge, since the keys for the second level
	// are bug IDs, and not static.

	var c interface{}
	err = json.Unmarshal(commentsBody, &c)

	key := fmt.Sprintf("%d", bug.ID)

	bugMap := c.(map[string]interface{})
	commentsByBugID := bugMap["bugs"].(map[string]interface{})
	commentsForBug := commentsByBugID[key]
	commentWrapper := commentsForBug.(map[string]interface{})
	commentList := commentWrapper["comments"].([]interface{})
	descriptionComment := commentList[0].(map[string]interface{})
	descriptionText := descriptionComment["text"].(string)

	return descriptionText, nil
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
	bugzillaProduct := flag.String("bugzilla-product", "", "the product name for the bugzilla query")
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

	if *bugzillaProduct == "" {
		fmt.Fprintf(os.Stderr, "Please provide a product to filter the bugzilla query (-bugzilla-product)")
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
	jiraCreateMeta, _, err := jiraClient.Issue.GetCreateMeta(*jiraProject)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to fetch metadata for %s: %s", *jiraProject, err)
		os.Exit(1)
	}
	knideployProject := jiraCreateMeta.GetProjectWithKey(*jiraProject)
	bugIssueType := knideployProject.GetIssueTypeWithName("bug")

	args := syncArgs{
		bugzillaURL:       *bugzillaURL,
		bugzillaProduct:   *bugzillaProduct,
		bugzillaToken:     *token,
		jiraURL:           *jiraURL,
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
