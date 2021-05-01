package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Shared configuration ("global")
type TemposaurusEnv struct {
	JIRAEmail      string
	AtlassianToken string
	TempoToken     string
	Timeout        time.Duration
	DateFrom       string
	DateTo         string
}

// The (part) definition of the response document, as defined by the API end-point documentation
type Myself struct {
	AccountID    string `json:"accountId"`
	EmailAddress string `json:"emailAddress"`
	DisplayName  string `json:"displayName"`
}

// The (part) definition of the response document, as defined by the API end-point documentation
type Period struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type Periods struct {
	Periods []Period `json:"periods"`
}

// The (part) definition of the response document, as defined by the API end-point documentation
type TimesheetApproval struct {
	Self             string `json:"self"`
	Period           Period
	RequiredSeconds  int `json:"requiredSeconds"`
	TimeSpentSeconds int `json:"timeSpentSeconds"`
}

func main() {

	// Set default values for "global" configuration
	env := TemposaurusEnv{
		Timeout: time.Second * 30,
		DateTo:  time.Now().Format("2006-01-02"),
	}

	// TODO how to extract latest tagged version? Go generate somehow?
	log.Println("Temposaurus starting...")

	// Validate input...

	env.JIRAEmail = os.Getenv("JIRA_EMAIL")
	if env.JIRAEmail == "" {
		log.Fatalln("JIRA_EMAIL not set")
	}

	env.AtlassianToken = os.Getenv("ATLASSIAN_TOKEN")
	if env.AtlassianToken == "" {
		log.Fatalln("ATLASSIAN_TOKEN not set")
	}

	env.TempoToken = os.Getenv("TEMPO_TOKEN")
	if env.TempoToken == "" {
		log.Fatalln("TEMPO_TOKEN not set")
	}

	// TODO validate date format/value
	env.DateFrom = os.Getenv("DATE_FROM")
	if env.DateFrom == "" {
		log.Fatalln("DATE_FROM not set")
	}

	// TODO validate date format/value
	dateTo := os.Getenv("DATE_TO")
	if dateTo != "" {
		env.DateTo = dateTo
	}

	// HTTP REST API client timeout
	httpTimeoutStr := os.Getenv("HTTP_TIMEOUT")
	if httpTimeoutStr != "" {
		i, err := strconv.Atoi(httpTimeoutStr)
		if err != nil {
			log.Fatalln("expected a positive integer for HTTP_TIMEOUT value but read:", httpTimeoutStr)
		}
		env.Timeout = time.Second * time.Duration(i)
	}

	// TODO put Atlassian user ID acquisition into a separate function
	myself := Myself{}
	{
		// Acquire Atlassian Account User ID. It uniquely identifies the user across
		// all Atlassian products. Available via Jira Cloud API end-point `Myself`:
		// https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-myself/#api-rest-api-3-myself-get

		// Set-up an HTTP request to satisfy the API end-point requirements
		const urlAtlassianUserID = "https://verifa.atlassian.net/rest/api/3/myself"
		req, err := http.NewRequest(http.MethodGet, urlAtlassianUserID, nil)
		if err != nil {
			log.Fatalln("Failed to create a new HTTP request to the Atlassian API `Myself` end-point:", err)
		}
		req.Header.Set("Accept", "application/json")
		req.SetBasicAuth(env.JIRAEmail, env.AtlassianToken) // TODO check docs, something about OAuth2 url.QueryEscape there

		netClient := &http.Client{
			Timeout: env.Timeout,
		}

		// Fetch the document containing the Atlassian User ID
		log.Println("Acquiring Atlassian User ID...")
		resp, err := netClient.Do(req)
		if err != nil {
			log.Fatalln("HTTP request to Jira Cloud `Myself` API end-point failed:", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Fatalln("HTTP request to Jira Cloud `Myself` API end-point failed:", resp.Status)
		}

		// Parse the received document, extracting the Atlassian User ID
		err = json.NewDecoder(resp.Body).Decode(&myself)
		if err != nil {
			log.Fatalln("failed to parse the document received from the Jira Cloud `Myself` API end-point:", err)
		}

		// TODO remove this once the current code block is a separate function
		resp.Body.Close()
	}

	// With the Atlassian User ID in hand, time-tracking information from Tempo can be accessed.

	// TODO put start/end date acquisition into a separate function
	periods := Periods{}
	{
		// A time-sheet is the basic unit of time-tracking in Tempo. Time-sheets capture information
		// about work over a period of time - typically a pay period. After they are filled out,
		// time-sheets can be submitted, reviewed, and then approved or rejected. Each time-sheet
		// covers a single period of time.

		// Acquire the list of time-sheet start/end dates from Tempo...

		// Tempo API end-point for retrieving all periods for a given date range.
		const urlTempoPeriods = "https://api.tempo.io/core/3/periods"

		// Add query parameters to the URL
		u, err := url.Parse(urlTempoPeriods)
		if err != nil {
			log.Fatalln("failed to parse the URL for the Tempo API `periods` end-point:", err)
		}
		q := u.Query()
		q.Add("from", env.DateFrom)
		q.Add("to", env.DateTo)
		u.RawQuery = q.Encode()

		// Set-up an HTTP request to satisfy the API end-point requirements
		req, err := http.NewRequest(http.MethodGet, u.String(), nil)
		if err != nil {
			log.Fatalln("failed to create a new HTTP request to the Tempo API `periods` end-point:", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+env.TempoToken) // FIXME does this need to be url.QueryEscape?

		netClient := &http.Client{
			Timeout: env.Timeout,
		}

		// Fetch the document containing the list of time-sheet start/end dates
		log.Println("acquiring the list of time-sheet start/end dates...")
		resp, err := netClient.Do(req)
		if err != nil {
			log.Fatalln("HTTP request to Tempo `periods` API end-point failed:", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Fatalln("HTTP request to Tempo `periods` API end-point failed:", resp.Status)
		}

		err = json.NewDecoder(resp.Body).Decode(&periods)
		if err != nil {
			log.Fatalln("failed to parse the document received from the Tempo `periods` API end-point:", err)
		}

		// TODO remove this once the current code block is a separate function
		resp.Body.Close()
	}
	ps := periods.Periods

	// With the correct from/to dates in hand, the information about approved time-sheets can be pulled...

	// TODO on error retry with exponential backoff unless it's an auth error, in which case panic
	// TODO use goroutines for concurrent requests and print the whole table at the end (via a pool)
	// TODO env var to limit the active goroutine count

	tas := make([]TimesheetApproval, len(periods.Periods))

	// Collect the data for every period
	for i, p := range ps {
		ta, err := ReadTimesheetApprovalFor(env, myself, p)
		if err != nil {
			log.Printf("error, skipping period %s to %s: %v", p.From, p.To, err)
			continue
		}
		tas[i] = ta
	}

	// Calculate the grand total
	ts := struct {
		TotalRequiredSeconds  int
		TotalTimeSpentSeconds int
	}{}

	for _, ta := range tas {
		ts.TotalRequiredSeconds += ta.RequiredSeconds
		ts.TotalTimeSpentSeconds += ta.TimeSpentSeconds
	}

	// Print the result
	fmt.Printf("%-12s %-12s %-12s %-12s %s\n", "From", "To", "Required", "Approved", "Overtime")
	for _, ta := range tas {
		fmt.Printf("%-12s %-12s %-12s %-12s %-12s\n",
			ta.Period.From, ta.Period.To,
			SecondsToHumanReadableFormat(ta.RequiredSeconds),
			SecondsToHumanReadableFormat(ta.TimeSpentSeconds),
			SecondsToHumanReadableFormat(ta.TimeSpentSeconds-ta.RequiredSeconds),
		)
	}
	fmt.Printf("\n%s: %-12s\n", "Total", SecondsToHumanReadableFormat(ts.TotalTimeSpentSeconds-ts.TotalRequiredSeconds))
}

// ReadTimesheetApprovalFor ...
func ReadTimesheetApprovalFor(env TemposaurusEnv, user Myself, p Period) (TimesheetApproval, error) {

	// Each request is going to have its own HTTP client as each request
	// is going to be processed by a goroutine eventually.
	netClient := &http.Client{
		Timeout: env.Timeout,
	}

	const urlTempoTimesheetApprovals = "https://api.tempo.io/core/3/timesheet-approvals/user/"
	u, err := url.Parse(urlTempoTimesheetApprovals)
	if err != nil {
		log.Fatalln("failed to parse the URL for the Tempo API `timesheet-approvals/user` end-point:", err)
	}
	u.Path += user.AccountID // FIXME this assumes the trailing slash in existing path; not sure what's the best way
	q := u.Query()
	q.Add("from", p.From)
	q.Add("to", p.To)
	u.RawQuery = q.Encode()

	// Set-up an HTTP request to satisfy the API end-point requirements
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		log.Fatalln("Failed to create a new HTTP request to the Tempo API `timesheet-approvals/user` end-point:", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.TempoToken) // FIXME does this need to be url.QueryEscape?

	// Fetch the document containing the time-sheet data
	resp, err := netClient.Do(req)
	if err != nil {
		log.Fatalln("HTTP request to Tempo `timesheet-approvals/user` API end-point failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalln("HTTP request to Tempo `timesheet-approvals/user` API end-point failed:", resp.Status)
	}

	ta := TimesheetApproval{}

	err = json.NewDecoder(resp.Body).Decode(&ta)
	if err != nil {
		log.Fatalln("failed to parse the document received from the Tempo `timesheet-approvals/user` API end-point:", err)
	}

	return ta, nil
}

// SecondsToHumanReadableFormat returns the human-readable representation of the given time duration,
// as defined by Go standard library function (Duration) String.
func SecondsToHumanReadableFormat(s int) string {
	return (time.Duration(s) * time.Second).String()
}
