package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/andygrunwald/go-jira"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type syncArgs struct {
	bugzillaURL string
	// bugzillaProduct   string
	// bugzillaToken     string
	jiraURL     string
	jiraClient  *jira.Client
	jiraProject string
}

type bug struct {
	ID          int    `json:"id"`
	Status      string `json:"status"`
	Summary     string `json:"summary"`
	Description string // not in the json from the original query
}

type bugSet struct {
	Bugs []bug `json:"bugs"`
}

func reportClosedIssues(args syncArgs) error {

	// Look for github:org:repo:issue or bugzilla:issue within the
	// HREF syntax for Jira ([title|url]). The URL is the UI version
	// so we never care about that. The type (github or bugzilla) is
	// extracted separately so we can switch handling based on it.
	linkSearch := regexp.MustCompile("\\[(github|bugzilla):(.+?)\\|.+\\]")

	search := fmt.Sprintf("status != CLOSED and status != DONE and ( labels = github or labels = bugzilla ) and project = %s", args.jiraProject)

	jiraIssues, _, err := args.jiraClient.Issue.Search(search, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to search for issue: %v", err)
		return err
	}

	bzClient := http.Client{
		Timeout: time.Second * 2, // Maximum of 2 secs
	}

	for _, jiraIssue := range jiraIssues {

		fmt.Printf("%s", jiraIssue.Key)
		match := linkSearch.FindStringSubmatch(jiraIssue.Fields.Description)
		if len(match) == 0 {
			continue
		}

		switch match[1] {

		case "github":
			fields := strings.Split(match[2], ":")
			fmt.Printf("\torg = %q repo = %q issue = %q\n",
				fields[0], fields[1], fields[2])

		case "bugzilla":
			fmt.Printf("\tbz = %s", match[2])
			bzURL := fmt.Sprintf("%s/rest/bug/%s?include_fields=id,summary,status",
				args.bugzillaURL, match[2])
			req, err := http.NewRequest(http.MethodGet, bzURL, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
				continue
			}
			req.Header.Set("User-Agent", "jira-sync")

			res, err := bzClient.Do(req)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
				continue
			}

			bzBody, err := ioutil.ReadAll(res.Body)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
				continue
			}

			bz := bugSet{}
			err = json.Unmarshal(bzBody, &bz)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
				continue
			}

			status := bz.Bugs[0].Status

			if status == "CLOSED" {
				fmt.Printf(" status = %s", status)
			}

			fmt.Printf("\n")

		default:
			fmt.Fprintf(os.Stderr, "ERROR:Could not parse %q\n", match[0])
		}
	}

	return nil
}

func main() {
	bugzillaURL := flag.String("bugzilla-url", "", "the base URL for the bugzilla server")
	// token := flag.String("bugzilla-token", "", "the API token")
	username := flag.String("jira-user", "", "the username")
	password := flag.String("jira-password", "", "the password")
	jiraURL := flag.String("jira-url", "", "the jira server URL")
	jiraProject := flag.String("jira-project", "", "the jira project")

	flag.Parse()

	if *bugzillaURL == "" {
		fmt.Fprintf(os.Stderr, "Please provide an API token (-bugzilla-url)")
		os.Exit(1)
	}

	// if *token == "" {
	// 	fmt.Fprintf(os.Stderr, "Please provide an API token (-bugzilla-token)")
	// 	os.Exit(1)
	// }

	if *username == "" || *password == "" {
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
		Username: *username,
		Password: *password,
	}

	jiraClient, err := jira.NewClient(tp.Client(), *jiraURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create client: %v", err)
		os.Exit(1)
	}

	args := syncArgs{
		bugzillaURL: *bugzillaURL,
		// bugzillaProduct:   *bugzillaProduct,
		// bugzillaToken:     *token,
		jiraURL:     *jiraURL,
		jiraClient:  jiraClient,
		jiraProject: *jiraProject,
	}

	err = reportClosedIssues(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
