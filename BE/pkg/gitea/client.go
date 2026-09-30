package gitea

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client communicates with a Gitea instance API using Personal Access Token authentication.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a Gitea API client.
func NewClient(baseURL, token string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// VerifyConnection checks the PAT is valid by fetching the authenticated user.
func (c *Client) VerifyConnection() (*User, error) {
	var user User
	if err := c.doGet("/api/v1/user", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// ListUserRepos lists repositories accessible to the authenticated user.
func (c *Client) ListUserRepos() ([]Repository, error) {
	var allRepos []Repository
	page := 1
	for {
		url := fmt.Sprintf("/api/v1/repos/search?limit=50&page=%d", page)
		var result struct {
			OK     []Repository `json:"data"`
			Count  int          `json:"count"`
			OKBool bool         `json:"ok"`
		}
		if err := c.doGet(url, &result); err != nil {
			return nil, err
		}
		allRepos = append(allRepos, result.OK...)
		if len(result.OK) < 50 {
			break
		}
		page++
	}
	return allRepos, nil
}

// GetRepo fetches a single repository by owner and name.
func (c *Client) GetRepo(owner, name string) (*Repository, error) {
	var repo Repository
	if err := c.doGet(fmt.Sprintf("/api/v1/repos/%s/%s", owner, name), &repo); err != nil {
		return nil, err
	}
	return &repo, nil
}

// CreatePullRequest opens a pull request.
func (c *Client) CreatePullRequest(owner, repo, title, head, base, body string) (*PullRequest, error) {
	payload, err := json.Marshal(map[string]any{
		"title": title,
		"head":  head,
		"base":  base,
		"body":  body,
	})
	if err != nil {
		return nil, err
	}
	var pr PullRequest
	if err := c.doPost(fmt.Sprintf("/api/v1/repos/%s/%s/pulls", owner, repo), payload, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// GetPullRequest fetches a single pull request by number.
func (c *Client) GetPullRequest(owner, repo string, number int) (*PullRequest, error) {
	var pr PullRequest
	if err := c.doGet(fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d", owner, repo, number), &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// ListPRCommits lists commits on a pull request.
func (c *Client) ListPRCommits(owner, repo string, number int) ([]Commit, error) {
	var commits []Commit
	if err := c.doGet(fmt.Sprintf("/api/v1/repos/%s/%s/pulls/%d/commits?limit=100", owner, repo, number), &commits); err != nil {
		return nil, err
	}
	return commits, nil
}

// doGet is a helper for authenticated GET requests.
func (c *Client) doGet(urlPath string, result interface{}) error {
	fullURL := c.baseURL + urlPath
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gitea API error %d: %s", resp.StatusCode, body)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// doPost is a helper for authenticated POST requests.
func (c *Client) doPost(urlPath string, payload []byte, result interface{}) error {
	fullURL := c.baseURL + urlPath
	req, err := http.NewRequest("POST", fullURL, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gitea API error %d: %s", resp.StatusCode, body)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// CreateIssue opens a new issue in a repository.
func (c *Client) CreateIssue(owner, repo, title, body string) (*Issue, error) {
	payload, err := json.Marshal(map[string]any{
		"title": title,
		"body":  body,
	})
	if err != nil {
		return nil, err
	}
	var issue Issue
	if err := c.doPost(fmt.Sprintf("/api/v1/repos/%s/%s/issues", owner, repo), payload, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// EditIssue updates an existing issue's title, body, and/or state.
func (c *Client) EditIssue(owner, repo string, issueIndex int, title, body, state string) (*Issue, error) {
	fields := map[string]any{}
	if title != "" {
		fields["title"] = title
	}
	if body != "" {
		fields["body"] = body
	}
	if state != "" {
		fields["state"] = state
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	var issue Issue
	if err := c.doPatch(fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d", owner, repo, issueIndex), payload, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// CloseIssue closes an issue by setting its state to "closed".
func (c *Client) CloseIssue(owner, repo string, issueIndex int) (*Issue, error) {
	return c.EditIssue(owner, repo, issueIndex, "", "", "closed")
}

// ReopenIssue reopens an issue by setting its state to "open".
func (c *Client) ReopenIssue(owner, repo string, issueIndex int) (*Issue, error) {
	return c.EditIssue(owner, repo, issueIndex, "", "", "open")
}

// CreateIssueComment adds a comment to an issue.
func (c *Client) CreateIssueComment(owner, repo string, issueIndex int, body string) (*GiteaComment, error) {
	payload, err := json.Marshal(map[string]any{
		"body": body,
	})
	if err != nil {
		return nil, err
	}
	var comment GiteaComment
	if err := c.doPost(fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/comments", owner, repo, issueIndex), payload, &comment); err != nil {
		return nil, err
	}
	return &comment, nil
}

// EditIssueComment updates the body of an existing issue comment.
func (c *Client) EditIssueComment(owner, repo string, commentID int64, body string) error {
	payload, err := json.Marshal(map[string]any{
		"body": body,
	})
	if err != nil {
		return err
	}
	var comment GiteaComment
	return c.doPatch(fmt.Sprintf("/api/v1/repos/%s/%s/issues/comments/%d", owner, repo, commentID), payload, &comment)
}

// DeleteIssueComment removes an existing issue comment.
func (c *Client) DeleteIssueComment(owner, repo string, commentID int64) error {
	return c.doDelete(fmt.Sprintf("/api/v1/repos/%s/%s/issues/comments/%d", owner, repo, commentID))
}

// ListLabels returns every label defined on a repository.
func (c *Client) ListLabels(owner, repo string) ([]Label, error) {
	var all []Label
	for page := 1; ; page++ {
		var labels []Label
		url := fmt.Sprintf("/api/v1/repos/%s/%s/labels?limit=50&page=%d", owner, repo, page)
		if err := c.doGet(url, &labels); err != nil {
			return nil, err
		}
		all = append(all, labels...)
		if len(labels) < 50 {
			return all, nil
		}
	}
}

// CreateLabel defines a new label on a repository.
func (c *Client) CreateLabel(owner, repo, name, color, description string) (*Label, error) {
	payload, err := json.Marshal(map[string]string{
		"name": name, "color": color, "description": description,
	})
	if err != nil {
		return nil, err
	}
	var label Label
	if err := c.doPost(fmt.Sprintf("/api/v1/repos/%s/%s/labels", owner, repo), payload, &label); err != nil {
		return nil, err
	}
	return &label, nil
}

// GetIssueLabels returns the labels currently attached to an issue.
func (c *Client) GetIssueLabels(owner, repo string, issueIndex int) ([]Label, error) {
	var all []Label
	for page := 1; ; page++ {
		var labels []Label
		url := fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/labels?limit=50&page=%d", owner, repo, issueIndex, page)
		if err := c.doGet(url, &labels); err != nil {
			return nil, err
		}
		all = append(all, labels...)
		if len(labels) < 50 {
			return all, nil
		}
	}
}

// AddIssueLabels attaches existing labels to an issue.
func (c *Client) AddIssueLabels(owner, repo string, issueIndex int, labelIDs []int64) error {
	if len(labelIDs) == 0 {
		return nil
	}
	payload, err := json.Marshal(map[string][]int64{"labels": labelIDs})
	if err != nil {
		return err
	}
	var labels []Label
	return c.doPost(fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/labels", owner, repo, issueIndex), payload, &labels)
}

// RemoveIssueLabel detaches a label from an issue.
func (c *Client) RemoveIssueLabel(owner, repo string, issueIndex, labelID int64) error {
	return c.doDelete(fmt.Sprintf("/api/v1/repos/%s/%s/issues/%d/labels/%d", owner, repo, issueIndex, labelID))
}

// doDelete is a helper for authenticated DELETE requests.
func (c *Client) doDelete(urlPath string) error {
	fullURL := c.baseURL + urlPath
	req, err := http.NewRequest("DELETE", fullURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gitea API error %d: %s", resp.StatusCode, body)
	}
	return nil
}

// doPatch is a helper for authenticated PATCH requests.
func (c *Client) doPatch(urlPath string, payload []byte, result interface{}) error {
	fullURL := c.baseURL + urlPath
	req, err := http.NewRequest("PATCH", fullURL, strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gitea API error %d: %s", resp.StatusCode, body)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

// --- Types ---

// User represents a Gitea user.
type User struct {
	ID       int64  `json:"id"`
	Login    string `json:"login"`
	FullName string `json:"full_name"`
}

// Repository represents a Gitea repository.
type Repository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// PullRequest represents a Gitea pull request.
type PullRequest struct {
	ID        int64      `json:"id"`
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	Draft     bool       `json:"draft"`
	Merged    bool       `json:"merged"`
	HTMLURL   string     `json:"html_url"`
	Head      PRBranch   `json:"head"`
	Base      PRBranch   `json:"base"`
	User      GiteaUser  `json:"user"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
	MergedAt  *time.Time `json:"merged_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type PRBranch struct {
	Ref string `json:"ref"`
}

type GiteaUser struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

// Commit represents a Gitea commit.
type Commit struct {
	SHA     string     `json:"sha"`
	HTMLURL string     `json:"html_url"`
	Commit  CommitData `json:"commit"`
	Author  *GiteaUser `json:"author"`
}

type CommitData struct {
	Message string       `json:"message"`
	Author  CommitAuthor `json:"author"`
}

type CommitAuthor struct {
	Name string    `json:"name"`
	Date time.Time `json:"date"`
}

// Issue represents a Gitea issue.
type Issue struct {
	ID        int64      `json:"id"`
	Index     int64      `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at"`
}

// GiteaComment represents a comment on a Gitea issue.
type GiteaComment struct {
	ID        int64      `json:"id"`
	Body      string     `json:"body"`
	User      *GiteaUser `json:"user"`
	HTMLURL   string     `json:"html_url"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Label represents a repository or issue label on Gitea.
type Label struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	URL         string `json:"url"`
}
