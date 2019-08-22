package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/andygrunwald/go-jira"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const closedCommentMessage = "The upstream ticket has been closed."

type syncArgs struct {
	bugzillaURL  string
	githubClient *github.Client
	jiraURL      string
	jiraClient   *jira.Client
	jiraProject  string
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
	ctx := context.Background()

	for _, jiraIssue := range jiraIssues {

		isClosed := false

		fmt.Printf("%s", jiraIssue.Key)
		match := linkSearch.FindStringSubmatch(jiraIssue.Fields.Description)
		if len(match) == 0 {
			continue
		}

		switch match[1] {

		case "github":
			fields := strings.Split(match[2], ":")
			fmt.Printf("\torg = %q repo = %q issue = %q",
				fields[0], fields[1], fields[2])

			issueNum, err := strconv.Atoi(fields[2])
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
				continue
			}
			ghIssue, _, err := args.githubClient.Issues.Get(ctx, fields[0], fields[1], issueNum)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
				continue
			}

			isClosed = (*ghIssue.State == "closed")

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

			isClosed = bz.Bugs[0].Status == "CLOSED"

		default:
			fmt.Fprintf(os.Stderr, "ERROR:Could not parse %q\n", match[0])
		}

		if !isClosed {
			fmt.Printf("\n")
			continue
		}

		fmt.Printf(" CLOSED")

		needToAdd := true

		// The search results do not include comments, so we have to
		// fetch tickets when we need the comments.
		commentedIssue, _, err := args.jiraClient.Issue.Get(jiraIssue.Key, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR fetching issue %s: %s\n", jiraIssue.Key, err)
			continue
		}

		if commentedIssue.Fields.Comments != nil {
			for _, comment := range commentedIssue.Fields.Comments.Comments {
				if comment.Body == closedCommentMessage {
					needToAdd = false
					break
				}
			}
		} else {
			fmt.Printf(" nil comments")
		}

		if needToAdd {
			newComment := jira.Comment{
				Body: closedCommentMessage,
			}
			_, _, err := args.jiraClient.Issue.AddComment(jiraIssue.ID, &newComment)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR adding comment: %s\n", err)
				continue
			}
			fmt.Printf(" UPDATED")
		}

		fmt.Printf("\n")

	}

	return nil
}

func main() {
	bugzillaURL := flag.String("bugzilla-url", "", "the base URL for the bugzilla server")
	username := flag.String("jira-user", "", "the username")
	password := flag.String("jira-password", "", "the password")
	jiraURL := flag.String("jira-url", "", "the jira server URL")
	jiraProject := flag.String("jira-project", "", "the jira project")
	token := flag.String("github-token", "", "the API token")

	flag.Parse()

	if *bugzillaURL == "" {
		fmt.Fprintf(os.Stderr, "Please provide an API token (-bugzilla-url)")
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

	if *jiraProject == "" {
		fmt.Fprintf(os.Stderr, "Please specify the -jira-project")
		os.Exit(1)
	}

	if *token == "" {
		fmt.Fprintf(os.Stderr, "Please provide an API token (-github-token)")
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

	githubClient := github.NewClient(tc)

	args := syncArgs{
		bugzillaURL:  *bugzillaURL,
		githubClient: githubClient,
		jiraURL:      *jiraURL,
		jiraClient:   jiraClient,
		jiraProject:  *jiraProject,
	}

	err = reportClosedIssues(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}
