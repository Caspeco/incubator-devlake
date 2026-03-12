/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
the "License"; you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/helpers/dbhelper"
	helper "github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	doramodels "github.com/apache/incubator-devlake/plugins/dora/models"
	"github.com/apache/incubator-devlake/plugins/github/models"
	webhookapi "github.com/apache/incubator-devlake/plugins/webhook/api"
	webhookmodels "github.com/apache/incubator-devlake/plugins/webhook/models"
)

const githubWebhookExportRunsPageSize = 30
const githubWebhookExportPageSize = 100
const githubWebhookExportMaxRetry = 3
const githubWebhookExportRetryDelay = time.Second

type GithubWebhookExportReq struct {
	RepoFullName            string   `mapstructure:"repoFullName" validate:"required"`
	TeamPrefix              string   `mapstructure:"teamPrefix"`
	TeamPrefixes            []string `mapstructure:"teamPrefixes"`
	DeploymentWorkflowNames []string `mapstructure:"deploymentWorkflowNames" validate:"dive,required"`
	WebhookConnectionId     uint64   `mapstructure:"webhookConnectionId" validate:"required"`
	LookbackDays            int      `mapstructure:"lookbackDays" validate:"required,min=1"`
	Submit                  bool     `mapstructure:"submit"`
	GithubConnectionId      uint64   `json:"-" mapstructure:"-"`
}

type GithubWebhookExportResponse struct {
	RepoFullName string                      `json:"repoFullName"`
	TeamPrefix   string                      `json:"teamPrefix"`
	TeamPrefixes []string                    `json:"teamPrefixes,omitempty"`
	LookbackDays int                         `json:"lookbackDays"`
	Submit       bool                        `json:"submit"`
	Counts       GithubWebhookExportCounts   `json:"counts"`
	Calls        []GithubWebhookPreparedCall `json:"calls"`
	Warnings     []string                    `json:"warnings,omitempty"`
}

type GithubWebhookExportCounts struct {
	PullRequests        int `json:"pullRequests"`
	PullRequestCommits  int `json:"pullRequestCommits"`
	PullRequestComments int `json:"pullRequestComments"`
	Deployments         int `json:"deployments"`
}

type GithubWebhookPreparedCall struct {
	Entity    string                 `json:"entity"`
	Method    string                 `json:"method"`
	Endpoint  string                 `json:"endpoint"`
	Payload   map[string]interface{} `json:"payload"`
	Submitted bool                   `json:"submitted"`
}

type githubWebhookPreparedExport struct {
	pullRequests        []webhookapi.WebhookPullRequestReq
	pullRequestCommits  []webhookapi.WebhookPullRequestCommitReq
	pullRequestComments []webhookapi.WebhookPullRequestCommentReq
	deployments         []webhookapi.WebhookDeploymentReq
	deploymentPRLinks   []githubDeploymentPRLink
	calls               []GithubWebhookPreparedCall
	warnings            []string
}

type githubDeploymentPRLink struct {
	DeploymentId     string
	PullRequestKey   int
	MatchedCommitSha string
}

type githubRepoResponse struct {
	ID            int    `json:"id"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
}

type githubUserRef struct {
	Login string `json:"login"`
	ID    int    `json:"id"`
}

type githubPRBranchRef struct {
	Ref  string `json:"ref"`
	Sha  string `json:"sha"`
	Repo *struct {
		ID int `json:"id"`
	} `json:"repo"`
}

type githubPullRequestResponse struct {
	ID             int               `json:"id"`
	Number         int               `json:"number"`
	State          string            `json:"state"`
	Title          string            `json:"title"`
	Body           string            `json:"body"`
	HTMLURL        string            `json:"html_url"`
	CreatedAt      time.Time         `json:"created_at"`
	ClosedAt       *time.Time        `json:"closed_at"`
	MergedAt       *time.Time        `json:"merged_at"`
	MergeCommitSHA string            `json:"merge_commit_sha"`
	Head           githubPRBranchRef `json:"head"`
	Base           githubPRBranchRef `json:"base"`
	User           *githubUserRef    `json:"user"`
	MergedBy       *githubUserRef    `json:"merged_by"`
	Additions      int               `json:"additions"`
	Deletions      int               `json:"deletions"`
	Draft          bool              `json:"draft"`
}

type githubPRCommitResponse struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name  string    `json:"name"`
			Email string    `json:"email"`
			Date  time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type githubIssueCommentResponse struct {
	ID        int            `json:"id"`
	Body      string         `json:"body"`
	User      *githubUserRef `json:"user"`
	CreatedAt time.Time      `json:"created_at"`
}

type githubReviewCommentResponse struct {
	ID                  int            `json:"id"`
	Body                string         `json:"body"`
	User                *githubUserRef `json:"user"`
	CreatedAt           time.Time      `json:"created_at"`
	CommitID            string         `json:"commit_id"`
	PullRequestReviewID int            `json:"pull_request_review_id"`
}

type githubReviewResponse struct {
	ID          int            `json:"id"`
	Body        string         `json:"body"`
	User        *githubUserRef `json:"user"`
	SubmittedAt *time.Time     `json:"submitted_at"`
	State       string         `json:"state"`
	CommitID    string         `json:"commit_id"`
}

type githubRunListResponse struct {
	WorkflowRuns []githubRunResponse `json:"workflow_runs"`
}

type githubWorkflowListResponse struct {
	Workflows []githubWorkflowResponse `json:"workflows"`
}

type githubWorkflowResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type githubRunPRRef struct {
	Number int `json:"number"`
}

type githubRunResponse struct {
	ID           int              `json:"id"`
	Name         string           `json:"name"`
	DisplayTitle string           `json:"display_title"`
	HeadBranch   string           `json:"head_branch"`
	HeadSHA      string           `json:"head_sha"`
	Status       string           `json:"status"`
	Conclusion   string           `json:"conclusion"`
	HTMLURL      string           `json:"html_url"`
	CreatedAt    *time.Time       `json:"created_at"`
	UpdatedAt    *time.Time       `json:"updated_at"`
	RunStartedAt *time.Time       `json:"run_started_at"`
	PullRequests []githubRunPRRef `json:"pull_requests"`
}

type githubRepoDeploymentResponse struct {
	ID          int        `json:"id"`
	SHA         string     `json:"sha"`
	Ref         string     `json:"ref"`
	Task        string     `json:"task"`
	Environment string     `json:"environment"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

type githubRepoDeploymentStatusResponse struct {
	ID             int        `json:"id"`
	State          string     `json:"state"`
	Environment    string     `json:"environment"`
	EnvironmentURL string     `json:"environment_url"`
	LogURL         string     `json:"log_url"`
	CreatedAt      *time.Time `json:"created_at"`
	UpdatedAt      *time.Time `json:"updated_at"`
}

type githubCompareResponse struct {
	Commits []githubCompareCommitResponse `json:"commits"`
}

type githubCompareCommitResponse struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
	} `json:"commit"`
}

type githubSelectedPullRequest struct {
	pr      githubPullRequestResponse
	commits []githubPRCommitResponse
}

type githubDeploymentCandidate struct {
	workflow   githubWorkflowResponse
	run        githubRunResponse
	deploymentData *githubRepoDeploymentResponse
	statusData     *githubRepoDeploymentStatusResponse
	deployment webhookapi.WebhookDeploymentReq
}

var jiraProjectPrefixSanitizer = regexp.MustCompile(`[^A-Z0-9]+`)
var githubMergeCommitPRNumberMatcher = regexp.MustCompile(`\(#(\d+)\)`)

func ExportWebhookData(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	req := &GithubWebhookExportReq{}
	err := helper.DecodeMapStruct(input.Body, req, true)
	if err != nil {
		return &plugin.ApiResourceOutput{Body: err.Error(), Status: http.StatusBadRequest}, nil
	}
	if err := errors.Convert(vld.Struct(req)); err != nil {
		return nil, errors.BadInput.Wrap(err, "input json error")
	}
	normalizedTeamPrefixes := normalizeJiraProjectPrefixes(req.TeamPrefix, req.TeamPrefixes)
	if len(normalizedTeamPrefixes) == 0 {
		return nil, errors.BadInput.New("either teamPrefix or teamPrefixes must contain at least one valid prefix")
	}

	connectionHelper := helper.NewConnectionHelper(basicRes, vld, "github")
	githubConnection := &models.GithubConnection{}
	if err := connectionHelper.First(githubConnection, input.Params); err != nil {
		return nil, err
	}

	webhookConnectionHelper := helper.NewConnectionHelper(basicRes, vld, "webhook")
	webhookConnection := &webhookmodels.WebhookConnection{}
	if err := webhookConnectionHelper.FirstById(webhookConnection, req.WebhookConnectionId); err != nil {
		return nil, errors.BadInput.Wrap(err, "unable to get webhook connection by the given connection ID")
	}
	basicRes.GetLogger().Info(
		"starting github webhook export: githubConnectionId=%s repo=%s teamPrefixes=%s webhookConnectionId=%d lookbackDays=%d submit=%t deploymentWorkflowNames=%s",
		input.Params["connectionId"],
		req.RepoFullName,
		strings.Join(normalizedTeamPrefixes, ", "),
		req.WebhookConnectionId,
		req.LookbackDays,
		req.Submit,
		strings.Join(req.DeploymentWorkflowNames, ", "),
	)

	ctx := context.TODO()
	if input.Request != nil {
		ctx = input.Request.Context()
	}
	apiClient, err := helper.NewApiClientFromConnection(ctx, basicRes, githubConnection)
	if err != nil {
		return nil, errors.Default.Wrap(err, "unable to create github api client")
	}
	req.GithubConnectionId = githubConnection.ID

	export, err := prepareGithubWebhookExport(apiClient, req)
	if err != nil {
		return nil, err
	}

	if req.Submit {
		if err := submitGithubWebhookExport(webhookConnection, export); err != nil {
			return nil, err
		}
		for i := range export.calls {
			export.calls[i].Submitted = true
		}
	} else {
		logGithubWebhookPreparedCalls(export.calls)
	}

	return &plugin.ApiResourceOutput{
		Body: &GithubWebhookExportResponse{
			RepoFullName: req.RepoFullName,
			TeamPrefix:   firstNormalizedJiraProjectPrefix(normalizedTeamPrefixes),
			TeamPrefixes: normalizedTeamPrefixes,
			LookbackDays: req.LookbackDays,
			Submit:       req.Submit,
			Counts: GithubWebhookExportCounts{
				PullRequests:        len(export.pullRequests),
				PullRequestCommits:  len(export.pullRequestCommits),
				PullRequestComments: len(export.pullRequestComments),
				Deployments:         len(export.deployments),
			},
			Calls:    export.calls,
			Warnings: export.warnings,
		},
		Status: http.StatusOK,
	}, nil
}

func prepareGithubWebhookExport(apiClient plugin.ApiClient, req *GithubWebhookExportReq) (*githubWebhookPreparedExport, errors.Error) {
	logger := basicRes.GetLogger()
	lookbackSince := time.Now().AddDate(0, 0, -req.LookbackDays)
	normalizedTeamPrefixes := normalizeJiraProjectPrefixes(req.TeamPrefix, req.TeamPrefixes)
	excludedGithubAccountIDs, err := fetchExcludedGithubAccountIDs(req.GithubConnectionId)
	if err != nil {
		return nil, err
	}
	repo, err := fetchGithubRepo(apiClient, req.RepoFullName)
	if err != nil {
		return nil, err
	}
	logger.Info("fetched github repo metadata: repo=%s repoId=%d defaultBranch=%s", repo.FullName, repo.ID, repo.DefaultBranch)

	export := &githubWebhookPreparedExport{}
	deployments, includedPRNumbers, deploymentPRLinks, warnings, err := fetchGithubDeployments(apiClient, req, repo, normalizedTeamPrefixes, lookbackSince)
	if err != nil {
		return nil, err
	}
	export.deploymentPRLinks = deploymentPRLinks
	export.warnings = append(export.warnings, warnings...)
	for _, deployment := range deployments {
		export.deployments = append(export.deployments, deployment)
	}

	selectedPRs, err := fetchGithubPullRequestsByNumbers(apiClient, req.RepoFullName, includedPRNumbers)
	if err != nil {
		return nil, err
	}
	applyNewestPullRequestTitleToDeployments(export.deployments, export.deploymentPRLinks, selectedPRs)
	for _, deployment := range export.deployments {
		if err := export.addCall("deployment", req.WebhookConnectionId, "deployments", deployment); err != nil {
			return nil, err
		}
	}
	logger.Info("selected %d merged pull requests for export from repo=%s using deployment comparison", len(selectedPRs), req.RepoFullName)

	for _, selected := range selectedPRs {
		prPayload := buildWebhookPullRequest(req.WebhookConnectionId, repo, selected.pr)
		export.pullRequests = append(export.pullRequests, prPayload)
		if err := export.addCall("pull_request", req.WebhookConnectionId, "pull_requests", prPayload); err != nil {
			return nil, err
		}

		for _, commit := range selected.commits {
			commitPayload := buildWebhookPullRequestCommit(selected.pr.Number, commit)
			export.pullRequestCommits = append(export.pullRequestCommits, commitPayload)
			if err := export.addCall("pull_request_commit", req.WebhookConnectionId, "pull_request_commits", commitPayload); err != nil {
				return nil, err
			}
		}

		issueComments, err := fetchGithubIssueComments(apiClient, req.RepoFullName, selected.pr.Number)
		if err != nil {
			return nil, err
		}
		for _, comment := range issueComments {
			if isExcludedGithubCommentAuthor(comment.User, excludedGithubAccountIDs) {
				continue
			}
			commentPayload := buildWebhookIssueComment(selected.pr.Number, comment)
			export.pullRequestComments = append(export.pullRequestComments, commentPayload)
			if err := export.addCall("pull_request_comment", req.WebhookConnectionId, "pull_request_comments", commentPayload); err != nil {
				return nil, err
			}
		}

		reviewComments, err := fetchGithubReviewComments(apiClient, req.RepoFullName, selected.pr.Number)
		if err != nil {
			return nil, err
		}
		for _, comment := range reviewComments {
			if isExcludedGithubCommentAuthor(comment.User, excludedGithubAccountIDs) {
				continue
			}
			commentPayload := buildWebhookReviewComment(selected.pr.Number, comment)
			export.pullRequestComments = append(export.pullRequestComments, commentPayload)
			if err := export.addCall("pull_request_comment", req.WebhookConnectionId, "pull_request_comments", commentPayload); err != nil {
				return nil, err
			}
		}

		reviews, err := fetchGithubReviews(apiClient, req.RepoFullName, selected.pr.Number)
		if err != nil {
			return nil, err
		}
		for _, review := range reviews {
			if review.SubmittedAt == nil {
				continue
			}
			if isExcludedGithubCommentAuthor(review.User, excludedGithubAccountIDs) {
				continue
			}
			commentPayload := buildWebhookReviewSummary(selected.pr.Number, review)
			export.pullRequestComments = append(export.pullRequestComments, commentPayload)
			if err := export.addCall("pull_request_comment", req.WebhookConnectionId, "pull_request_comments", commentPayload); err != nil {
				return nil, err
			}
		}
		logger.Info(
			"prepared PR export payloads: pr=%d commits=%d issueComments=%d reviewComments=%d reviews=%d",
			selected.pr.Number,
			len(selected.commits),
			len(issueComments),
			len(reviewComments),
			len(reviews),
		)
	}
	logger.Info(
		"prepared github webhook export totals: pullRequests=%d pullRequestCommits=%d pullRequestComments=%d deployments=%d warnings=%d",
		len(export.pullRequests),
		len(export.pullRequestCommits),
		len(export.pullRequestComments),
		len(export.deployments),
		len(export.warnings),
	)
	return export, nil
}

func submitGithubWebhookExport(connection *webhookmodels.WebhookConnection, export *githubWebhookPreparedExport) errors.Error {
	var err errors.Error
	txHelper := dbhelper.NewTxHelper(basicRes, &err)
	defer txHelper.End()
	tx := txHelper.Begin()
	logger := basicRes.GetLogger()

	for i := range export.pullRequests {
		if err := webhookapi.CreatePullRequest(connection, &export.pullRequests[i], tx, logger); err != nil {
			return err
		}
	}
	for i := range export.pullRequestCommits {
		if err := webhookapi.CreatePullRequestCommit(connection, &export.pullRequestCommits[i], tx); err != nil {
			return err
		}
	}
	for i := range export.pullRequestComments {
		if err := webhookapi.CreatePullRequestComment(connection, &export.pullRequestComments[i], tx); err != nil {
			return err
		}
	}
	for i := range export.deployments {
		if err := webhookapi.CreateDeploymentAndDeploymentCommits(connection, &export.deployments[i], tx, logger); err != nil {
			return err
		}
	}
	for i := range export.deploymentPRLinks {
		link := export.deploymentPRLinks[i]
		deploymentCommitId, ok := export.findDeploymentCommitId(connection.ID, link.DeploymentId)
		if !ok {
			return errors.Default.New(fmt.Sprintf("deployment commit not found for deployment %s", link.DeploymentId))
		}
		record := &doramodels.DeploymentCommitPullRequest{
			ProjectName:        connection.Name,
			DeploymentCommitId: deploymentCommitId,
			PullRequestId:      fmt.Sprintf("webhook:%d:%d", connection.ID, link.PullRequestKey),
			MatchedCommitSha:   link.MatchedCommitSha,
			PullRequestKey:     link.PullRequestKey,
			DetectionMethod:    "github_webhook_export_compare",
		}
		if err := tx.CreateOrUpdate(record); err != nil {
			return err
		}
	}
	return nil
}

func fetchGithubRepo(apiClient plugin.ApiClient, repoFullName string) (*githubRepoResponse, errors.Error) {
	repo := &githubRepoResponse{}
	if err := githubApiGetAndUnmarshalWithRetry(apiClient, fmt.Sprintf("repos/%s", repoFullName), nil, repo); err != nil {
		return nil, err
	}
	return repo, nil
}

func fetchGithubPullRequestsByNumbers(apiClient plugin.ApiClient, repoFullName string, prNumbers []int) ([]githubSelectedPullRequest, errors.Error) {
	var selected []githubSelectedPullRequest
	logger := basicRes.GetLogger()
	for _, prNumber := range prNumbers {
		pr := githubPullRequestResponse{}
		if err := githubApiGetAndUnmarshalWithRetry(apiClient, fmt.Sprintf("repos/%s/pulls/%d", repoFullName, prNumber), nil, &pr); err != nil {
			return nil, err
		}
		if pr.MergedAt == nil {
			logger.Info("skipping unmatched pull request because it is not merged: repo=%s pr=%d", repoFullName, pr.Number)
			continue
		}
		commits, err := fetchGithubPRCommits(apiClient, repoFullName, pr.Number)
		if err != nil {
			return nil, err
		}
		selected = append(selected, githubSelectedPullRequest{
			pr:      pr,
			commits: commits,
		})
		logger.Info("selected github pull request from deployment comparison: repo=%s pr=%d commits=%d", repoFullName, pr.Number, len(commits))
	}
	return selected, nil
}

func pullRequestMatchesTeamPrefix(pr githubPullRequestResponse, teamPrefixes ...string) bool {
	if len(teamPrefixes) == 0 {
		return false
	}
	for _, teamPrefix := range teamPrefixes {
		if teamPrefix == "" {
			continue
		}
		jiraPrefix := teamPrefix + "-"
		for _, candidate := range []string{pr.Head.Ref, pr.Title, pr.Body} {
			if strings.Contains(strings.ToUpper(candidate), jiraPrefix) {
				return true
			}
		}
	}
	return false
}

func normalizeJiraProjectPrefix(teamPrefix string) string {
	normalized := strings.ToUpper(strings.TrimSpace(teamPrefix))
	normalized = strings.TrimSuffix(normalized, "-")
	return jiraProjectPrefixSanitizer.ReplaceAllString(normalized, "")
}

func normalizeJiraProjectPrefixes(teamPrefix string, teamPrefixes []string) []string {
	seen := make(map[string]struct{})
	var normalizedPrefixes []string
	for _, candidate := range append([]string{teamPrefix}, teamPrefixes...) {
		normalized := normalizeJiraProjectPrefix(candidate)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		normalizedPrefixes = append(normalizedPrefixes, normalized)
	}
	return normalizedPrefixes
}

func firstNormalizedJiraProjectPrefix(teamPrefixes []string) string {
	if len(teamPrefixes) == 0 {
		return ""
	}
	return teamPrefixes[0]
}

func fetchGithubPRCommits(apiClient plugin.ApiClient, repoFullName string, prNumber int) ([]githubPRCommitResponse, errors.Error) {
	var commits []githubPRCommitResponse
	page := 1
	for {
		query := url.Values{
			"page":     []string{fmt.Sprintf("%d", page)},
			"per_page": []string{fmt.Sprintf("%d", githubWebhookExportPageSize)},
		}
		var batch []githubPRCommitResponse
		if err := githubApiGetAndUnmarshalWithRetry(apiClient, fmt.Sprintf("repos/%s/pulls/%d/commits", repoFullName, prNumber), query, &batch); err != nil {
			return nil, err
		}
		commits = append(commits, batch...)
		if len(batch) < githubWebhookExportPageSize {
			break
		}
		page++
	}
	return commits, nil
}

func fetchGithubIssueComments(apiClient plugin.ApiClient, repoFullName string, prNumber int) ([]githubIssueCommentResponse, errors.Error) {
	return fetchGithubPagedArray[githubIssueCommentResponse](apiClient, fmt.Sprintf("repos/%s/issues/%d/comments", repoFullName, prNumber))
}

func fetchGithubReviewComments(apiClient plugin.ApiClient, repoFullName string, prNumber int) ([]githubReviewCommentResponse, errors.Error) {
	return fetchGithubPagedArray[githubReviewCommentResponse](apiClient, fmt.Sprintf("repos/%s/pulls/%d/comments", repoFullName, prNumber))
}

func fetchGithubReviews(apiClient plugin.ApiClient, repoFullName string, prNumber int) ([]githubReviewResponse, errors.Error) {
	return fetchGithubPagedArray[githubReviewResponse](apiClient, fmt.Sprintf("repos/%s/pulls/%d/reviews", repoFullName, prNumber))
}

func fetchGithubPagedArray[T any](apiClient plugin.ApiClient, path string) ([]T, errors.Error) {
	var items []T
	page := 1
	for {
		query := url.Values{
			"page":     []string{fmt.Sprintf("%d", page)},
			"per_page": []string{fmt.Sprintf("%d", githubWebhookExportPageSize)},
		}
		var batch []T
		if err := githubApiGetAndUnmarshalWithRetry(apiClient, path, query, &batch); err != nil {
			return nil, err
		}
		items = append(items, batch...)
		if len(batch) < githubWebhookExportPageSize {
			break
		}
		page++
	}
	return items, nil
}

func fetchGithubDeployments(
	apiClient plugin.ApiClient,
	req *GithubWebhookExportReq,
	repo *githubRepoResponse,
	teamPrefixes []string,
	lookbackSince time.Time,
) ([]webhookapi.WebhookDeploymentReq, []int, []githubDeploymentPRLink, []string, errors.Error) {
	var deployments []webhookapi.WebhookDeploymentReq
	var candidates []githubDeploymentCandidate
	var deploymentPRLinks []githubDeploymentPRLink
	var warnings []string
	logger := basicRes.GetLogger()

	githubDeployments, err := fetchGithubRepoDeployments(apiClient, req.RepoFullName)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	logger.Info("scanned github deployments: repo=%s deployments=%d", req.RepoFullName, len(githubDeployments))
	for _, deploymentData := range githubDeployments {
		if deploymentData.CreatedAt != nil && deploymentData.CreatedAt.Before(lookbackSince) {
			continue
		}
		if !deploymentTargetsProd(deploymentData.Environment) {
			continue
		}
		statuses, err := fetchGithubRepoDeploymentStatuses(apiClient, req.RepoFullName, deploymentData.ID)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		status, ok := latestSuccessfulGithubDeploymentStatus(statuses)
		if !ok {
			logger.Info(
				"skipping github deployment because no successful status was found: deploymentId=%d environment=%s statuses=%d",
				deploymentData.ID,
				deploymentData.Environment,
				len(statuses),
			)
			continue
		}
		deployment := buildWebhookDeploymentFromGithubDeployment(repo, deploymentData, status)
		if len(deployment.DeploymentCommits) == 0 {
			warnings = append(warnings, fmt.Sprintf("skipped github deployment %d because no deployment commit could be constructed", deploymentData.ID))
			continue
		}
		candidates = append(candidates, githubDeploymentCandidate{
			deploymentData: &deploymentData,
			statusData:     &status,
			deployment:     deployment,
		})
		logger.Info(
			"matched github deployment for export: deploymentId=%d environment=%s deploymentCommits=%d",
			deploymentData.ID,
			deploymentData.Environment,
			len(deployment.DeploymentCommits),
		)
	}

	workflows, err := fetchGithubWorkflows(apiClient, req.RepoFullName)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	targetWorkflows := filterGithubWorkflowsByName(workflows, req.DeploymentWorkflowNames)
	logger.Info(
		"resolved deployment workflows for export: repo=%s requested=%s matched=%s",
		req.RepoFullName,
		strings.Join(req.DeploymentWorkflowNames, ", "),
		joinGithubWorkflowNames(targetWorkflows),
	)
	if len(targetWorkflows) == 0 && len(req.DeploymentWorkflowNames) > 0 {
		warnings = append(warnings, fmt.Sprintf("no workflows matched requested deployment workflow names: %s", strings.Join(req.DeploymentWorkflowNames, ", ")))
	}

	for _, workflow := range targetWorkflows {
		workflowRuns, err := fetchGithubWorkflowRuns(apiClient, req.RepoFullName, workflow.ID, lookbackSince)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		logger.Info("scanned github workflow runs: repo=%s workflow=%s runs=%d", req.RepoFullName, workflow.Name, len(workflowRuns))
		for _, run := range workflowRuns {
			if strings.ToLower(run.Status) != "completed" {
				logger.Info("skipping github workflow run because status is not completed: workflow=%s runId=%d status=%s", workflow.Name, run.ID, run.Status)
				continue
			}
			if strings.ToLower(run.Conclusion) != "success" {
				logger.Info("skipping github workflow run because conclusion is not success: workflow=%s runId=%d conclusion=%s", workflow.Name, run.ID, run.Conclusion)
				continue
			}
			logger.Info(
				"continuing github workflow run for deployment comparison without direct pull request association: workflow=%s runId=%d headSha=%s pullRequests=%d",
				workflow.Name,
				run.ID,
				run.HeadSHA,
				len(run.PullRequests),
			)
			deployment := buildWebhookDeployment(repo, workflow, run, nil)
			if len(deployment.DeploymentCommits) == 0 {
				warnings = append(warnings, fmt.Sprintf("skipped workflow run %d for workflow %s because no deployment commit could be constructed", run.ID, workflow.Name))
				logger.Info(
					"matched github workflow but produced no deployment commits: workflow=%s runId=%d headSha=%s",
					workflow.Name,
					run.ID,
					run.HeadSHA,
				)
				continue
			}
			candidates = append(candidates, githubDeploymentCandidate{
				workflow:   workflow,
				run:        run,
				deployment: deployment,
			})
			logger.Info(
				"matched deployment workflow for export: workflow=%s runId=%d deploymentCommits=%d",
				workflow.Name,
				run.ID,
				len(deployment.DeploymentCommits),
			)
		}
	}
	sortGithubDeploymentCandidates(candidates)
	includedPRNumbers, includedDeploymentIds, deploymentLinksById, err := findGithubDeploymentIncludedPRNumbers(apiClient, req.RepoFullName, teamPrefixes, candidates)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	for _, candidate := range candidates {
		if _, ok := includedDeploymentIds[candidate.deployment.Id]; !ok {
			continue
		}
		deployments = append(deployments, candidate.deployment)
		deploymentPRLinks = append(deploymentPRLinks, deploymentLinksById[candidate.deployment.Id]...)
	}
	return deployments, includedPRNumbers, deploymentPRLinks, warnings, nil
}

func fetchGithubWorkflows(apiClient plugin.ApiClient, repoFullName string) ([]githubWorkflowResponse, errors.Error) {
	var workflows []githubWorkflowResponse
	page := 1
	for {
		query := url.Values{
			"page":     []string{fmt.Sprintf("%d", page)},
			"per_page": []string{fmt.Sprintf("%d", githubWebhookExportPageSize)},
		}
		workflowList := &githubWorkflowListResponse{}
		if err := githubApiGetAndUnmarshalWithRetry(apiClient, fmt.Sprintf("repos/%s/actions/workflows", repoFullName), query, workflowList); err != nil {
			return nil, err
		}
		workflows = append(workflows, workflowList.Workflows...)
		if len(workflowList.Workflows) < githubWebhookExportPageSize {
			break
		}
		page++
	}
	return workflows, nil
}

func fetchGithubRepoDeployments(apiClient plugin.ApiClient, repoFullName string) ([]githubRepoDeploymentResponse, errors.Error) {
	var deployments []githubRepoDeploymentResponse
	page := 1
	for {
		query := url.Values{
			"page":     []string{fmt.Sprintf("%d", page)},
			"per_page": []string{fmt.Sprintf("%d", githubWebhookExportPageSize)},
		}
		var batch []githubRepoDeploymentResponse
		if err := githubApiGetAndUnmarshalWithRetry(apiClient, fmt.Sprintf("repos/%s/deployments", repoFullName), query, &batch); err != nil {
			return nil, err
		}
		deployments = append(deployments, batch...)
		if len(batch) < githubWebhookExportPageSize {
			break
		}
		page++
	}
	return deployments, nil
}

func fetchGithubRepoDeploymentStatuses(apiClient plugin.ApiClient, repoFullName string, deploymentID int) ([]githubRepoDeploymentStatusResponse, errors.Error) {
	var statuses []githubRepoDeploymentStatusResponse
	page := 1
	for {
		query := url.Values{
			"page":     []string{fmt.Sprintf("%d", page)},
			"per_page": []string{fmt.Sprintf("%d", githubWebhookExportPageSize)},
		}
		var batch []githubRepoDeploymentStatusResponse
		if err := githubApiGetAndUnmarshalWithRetry(apiClient, fmt.Sprintf("repos/%s/deployments/%d/statuses", repoFullName, deploymentID), query, &batch); err != nil {
			return nil, err
		}
		statuses = append(statuses, batch...)
		if len(batch) < githubWebhookExportPageSize {
			break
		}
		page++
	}
	return statuses, nil
}

func fetchExcludedGithubAccountIDs(connectionID uint64) (map[int]struct{}, errors.Error) {
	excludedAccounts := make([]models.GithubAccount, 0)
	err := basicRes.GetDal().All(
		&excludedAccounts,
		dal.Where("connection_id = ?", connectionID),
		dal.Where("exclude_from_computation = ?", true),
	)
	if err != nil {
		return nil, err
	}
	excluded := make(map[int]struct{}, len(excludedAccounts))
	for _, account := range excludedAccounts {
		excluded[account.Id] = struct{}{}
	}
	return excluded, nil
}

func filterGithubWorkflowsByName(workflows []githubWorkflowResponse, requested []string) []githubWorkflowResponse {
	requestedNames := make(map[string]struct{}, len(requested))
	for _, name := range requested {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		requestedNames[trimmed] = struct{}{}
	}
	var matches []githubWorkflowResponse
	for _, workflow := range workflows {
		if _, ok := requestedNames[workflow.Name]; ok {
			matches = append(matches, workflow)
		}
	}
	return matches
}

func joinGithubWorkflowNames(workflows []githubWorkflowResponse) string {
	names := make([]string, 0, len(workflows))
	for _, workflow := range workflows {
		names = append(names, workflow.Name)
	}
	return strings.Join(names, ", ")
}

func fetchGithubWorkflowRuns(apiClient plugin.ApiClient, repoFullName string, workflowID int, lookbackSince time.Time) ([]githubRunResponse, errors.Error) {
	var runs []githubRunResponse
	page := 1
	createdFilter := fmt.Sprintf(">=%s", lookbackSince.UTC().Format("2006-01-02T15:04:05Z"))
	for {
		query := url.Values{
			"page":     []string{fmt.Sprintf("%d", page)},
			"per_page": []string{fmt.Sprintf("%d", githubWebhookExportPageSize)},
			"created":  []string{createdFilter},
		}
		runList := &githubRunListResponse{}
		if err := githubApiGetAndUnmarshalWithRetry(apiClient, fmt.Sprintf("repos/%s/actions/workflows/%d/runs", repoFullName, workflowID), query, runList); err != nil {
			return nil, err
		}
		runs = append(runs, runList.WorkflowRuns...)
		if len(runList.WorkflowRuns) < githubWebhookExportPageSize {
			break
		}
		page++
	}
	return runs, nil
}

func fetchGithubComparedCommits(apiClient plugin.ApiClient, repoFullName string, baseSHA string, headSHA string) ([]githubCompareCommitResponse, errors.Error) {
	if strings.TrimSpace(baseSHA) == "" || strings.TrimSpace(headSHA) == "" || baseSHA == headSHA {
		return nil, nil
	}
	compareResponse := &githubCompareResponse{}
	if err := githubApiGetAndUnmarshalWithRetry(apiClient, fmt.Sprintf("repos/%s/compare/%s...%s", repoFullName, baseSHA, headSHA), nil, compareResponse); err != nil {
		return nil, err
	}
	return compareResponse.Commits, nil
}

func buildWebhookPullRequest(webhookConnectionId uint64, repo *githubRepoResponse, pr githubPullRequestResponse) webhookapi.WebhookPullRequestReq {
	baseRepoId := fmt.Sprintf("webhook:%d", webhookConnectionId)
	headRepoId := fmt.Sprintf("github:%d:%d", webhookConnectionId, repo.ID)
	req := webhookapi.WebhookPullRequestReq{
		Id:             fmt.Sprintf("github:%d", pr.ID),
		BaseRepoId:     baseRepoId,
		HeadRepoId:     headRepoId,
		Status:         "MERGED",
		OriginalStatus: strings.ToUpper(pr.State),
		Title:          pr.Title,
		Description:    pr.Body,
		Url:            pr.HTMLURL,
		PullRequestKey: pr.Number,
		CreatedDate:    pr.CreatedAt,
		MergedDate:     pr.MergedAt,
		ClosedDate:     pr.ClosedAt,
		MergeCommitSha: pr.MergeCommitSHA,
		HeadRef:        pr.Head.Ref,
		BaseRef:        pr.Base.Ref,
		BaseCommitSha:  pr.Base.Sha,
		HeadCommitSha:  pr.Head.Sha,
		Additions:      pr.Additions,
		Deletions:      pr.Deletions,
		IsDraft:        pr.Draft,
	}
	if pr.User != nil {
		req.AuthorName = pr.User.Login
		req.AuthorId = fmt.Sprintf("%d", pr.User.ID)
	}
	if pr.MergedBy != nil {
		req.MergedByName = pr.MergedBy.Login
		req.MergedById = fmt.Sprintf("%d", pr.MergedBy.ID)
	}
	return req
}

func buildWebhookPullRequestCommit(prNumber int, commit githubPRCommitResponse) webhookapi.WebhookPullRequestCommitReq {
	authoredAt := commit.Commit.Author.Date
	return webhookapi.WebhookPullRequestCommitReq{
		PullRequestKey:     intPtr(prNumber),
		CommitSha:          commit.SHA,
		CommitAuthorName:   commit.Commit.Author.Name,
		CommitAuthorEmail:  commit.Commit.Author.Email,
		CommitAuthoredDate: &authoredAt,
	}
}

func buildWebhookIssueComment(prNumber int, comment githubIssueCommentResponse) webhookapi.WebhookPullRequestCommentReq {
	createdAt := comment.CreatedAt
	req := webhookapi.WebhookPullRequestCommentReq{
		Id:             fmt.Sprintf("issue-comment-%d", comment.ID),
		PullRequestKey: intPtr(prNumber),
		Body:           comment.Body,
		CreatedDate:    &createdAt,
		Type:           "NORMAL",
	}
	if comment.User != nil {
		req.AccountId = fmt.Sprintf("%d", comment.User.ID)
	}
	return req
}

func buildWebhookReviewComment(prNumber int, comment githubReviewCommentResponse) webhookapi.WebhookPullRequestCommentReq {
	createdAt := comment.CreatedAt
	req := webhookapi.WebhookPullRequestCommentReq{
		Id:             fmt.Sprintf("review-comment-%d", comment.ID),
		PullRequestKey: intPtr(prNumber),
		Body:           comment.Body,
		CreatedDate:    &createdAt,
		CommitSha:      comment.CommitID,
		Type:           "DIFF",
		ReviewId:       fmt.Sprintf("%d", comment.PullRequestReviewID),
	}
	if comment.User != nil {
		req.AccountId = fmt.Sprintf("%d", comment.User.ID)
	}
	return req
}

func buildWebhookReviewSummary(prNumber int, review githubReviewResponse) webhookapi.WebhookPullRequestCommentReq {
	req := webhookapi.WebhookPullRequestCommentReq{
		Id:             fmt.Sprintf("review-%d", review.ID),
		PullRequestKey: intPtr(prNumber),
		Body:           review.Body,
		CreatedDate:    review.SubmittedAt,
		CommitSha:      review.CommitID,
		Type:           "REVIEW",
		Status:         strings.ToUpper(review.State),
		ReviewId:       fmt.Sprintf("%d", review.ID),
	}
	if review.User != nil {
		req.AccountId = fmt.Sprintf("%d", review.User.ID)
	}
	return req
}

func isExcludedGithubCommentAuthor(user *githubUserRef, excludedGithubAccountIDs map[int]struct{}) bool {
	if user == nil || len(excludedGithubAccountIDs) == 0 {
		return false
	}
	_, excluded := excludedGithubAccountIDs[user.ID]
	return excluded
}

func buildWebhookDeployment(
	repo *githubRepoResponse,
	workflow githubWorkflowResponse,
	run githubRunResponse,
	associatedPRs []githubSelectedPullRequest,
) webhookapi.WebhookDeploymentReq {
	startedAt := firstNonNilTime(run.RunStartedAt, run.CreatedAt)
	finishedAt := firstNonNilTime(run.UpdatedAt, startedAt)
	deployment := webhookapi.WebhookDeploymentReq{
		Id:           fmt.Sprintf("github-workflow-%d-run-%d", workflow.ID, run.ID),
		DisplayTitle: firstNonEmpty(run.DisplayTitle, workflow.Name, run.Name),
		Url:          run.HTMLURL,
		Result:       mapGithubConclusionToWebhookResult(run.Conclusion),
		Environment:  "PRODUCTION",
		Name:         firstNonEmpty(workflow.Name, run.DisplayTitle, run.Name),
		CreatedDate:  firstNonNilTime(run.CreatedAt, startedAt, finishedAt),
		StartedDate:  startedAt,
		FinishedDate: finishedAt,
	}
	if len(associatedPRs) == 0 {
		commitSha := firstNonEmpty(run.HeadSHA)
		if commitSha != "" {
			deployment.DeploymentCommits = append(deployment.DeploymentCommits, webhookapi.WebhookDeploymentCommitReq{
				DisplayTitle: firstNonEmpty(run.DisplayTitle, workflow.Name, run.Name),
				Url:          run.HTMLURL,
				RepoUrl:      firstNonEmpty(repo.CloneURL, repo.HTMLURL),
				Name:         fmt.Sprintf("%s run-%d", workflow.Name, run.ID),
				RefName:      firstNonEmpty(run.HeadBranch, repo.DefaultBranch),
				CommitSha:    commitSha,
				CommitMsg:    firstNonEmpty(run.DisplayTitle, run.Name, workflow.Name),
				Result:       mapGithubConclusionToWebhookResult(run.Conclusion),
				Status:       "DONE",
				CreatedDate:  firstNonNilTime(run.CreatedAt, startedAt, finishedAt),
				StartedDate:  startedAt,
				FinishedDate: finishedAt,
			})
		}
		return deployment
	}
	for _, pr := range associatedPRs {
		commitSha := firstNonEmpty(pr.pr.MergeCommitSHA, run.HeadSHA, pr.pr.Head.Sha)
		if commitSha == "" {
			continue
		}
		deployment.DeploymentCommits = append(deployment.DeploymentCommits, webhookapi.WebhookDeploymentCommitReq{
			DisplayTitle: fmt.Sprintf("%s for PR #%d %s", workflow.Name, pr.pr.Number, pr.pr.Title),
			Url:          run.HTMLURL,
			RepoUrl:      firstNonEmpty(repo.CloneURL, repo.HTMLURL),
			Name:         fmt.Sprintf("%s for pr-%d", workflow.Name, pr.pr.Number),
			RefName:      firstNonEmpty(pr.pr.Base.Ref, run.HeadBranch, repo.DefaultBranch),
			CommitSha:    commitSha,
			CommitMsg:    fmt.Sprintf("PR #%d %s", pr.pr.Number, pr.pr.Title),
			Result:       mapGithubConclusionToWebhookResult(run.Conclusion),
			Status:       "DONE",
			CreatedDate:  firstNonNilTime(run.CreatedAt, startedAt, finishedAt),
			StartedDate:  startedAt,
			FinishedDate: finishedAt,
		})
	}
	return deployment
}

func buildWebhookDeploymentFromGithubDeployment(
	repo *githubRepoResponse,
	deploymentData githubRepoDeploymentResponse,
	status githubRepoDeploymentStatusResponse,
) webhookapi.WebhookDeploymentReq {
	startedAt := firstNonNilTime(status.CreatedAt, deploymentData.CreatedAt, deploymentData.UpdatedAt)
	finishedAt := firstNonNilTime(status.UpdatedAt, status.CreatedAt, deploymentData.UpdatedAt, deploymentData.CreatedAt)
	displayTitle := firstNonEmpty(deploymentData.Task, deploymentData.Environment, deploymentData.Ref, deploymentData.SHA)
	deploymentURL := firstNonEmpty(status.LogURL, status.EnvironmentURL)
	deployment := webhookapi.WebhookDeploymentReq{
		Id:           fmt.Sprintf("github-deployment-%d", deploymentData.ID),
		DisplayTitle: displayTitle,
		Url:          deploymentURL,
		Result:       mapGithubDeploymentStatusToWebhookResult(status.State),
		Environment:  "PRODUCTION",
		Name:         firstNonEmpty(deploymentData.Task, deploymentData.Environment, deploymentData.Ref, fmt.Sprintf("deployment-%d", deploymentData.ID)),
		CreatedDate:  firstNonNilTime(deploymentData.CreatedAt, startedAt, finishedAt),
		StartedDate:  startedAt,
		FinishedDate: finishedAt,
	}
	if strings.TrimSpace(deploymentData.SHA) != "" {
		deployment.DeploymentCommits = append(deployment.DeploymentCommits, webhookapi.WebhookDeploymentCommitReq{
			DisplayTitle: displayTitle,
			Url:          deploymentURL,
			RepoUrl:      firstNonEmpty(repo.CloneURL, repo.HTMLURL),
			Name:         firstNonEmpty(deploymentData.Task, fmt.Sprintf("deployment-%d", deploymentData.ID)),
			RefName:      firstNonEmpty(deploymentData.Ref, repo.DefaultBranch),
			CommitSha:    deploymentData.SHA,
			CommitMsg:    firstNonEmpty(deploymentData.Task, deploymentData.Environment, deploymentData.Ref),
			Result:       mapGithubDeploymentStatusToWebhookResult(status.State),
			Status:       "DONE",
			CreatedDate:  firstNonNilTime(deploymentData.CreatedAt, startedAt, finishedAt),
			StartedDate:  startedAt,
			FinishedDate: finishedAt,
		})
	}
	return deployment
}

func mapGithubConclusionToWebhookResult(conclusion string) string {
	switch strings.ToLower(conclusion) {
	case "success", "neutral", "skipped":
		return "SUCCESS"
	case "cancelled":
		return "CANCELLED"
	default:
		return "FAILURE"
	}
}

func mapGithubDeploymentStatusToWebhookResult(state string) string {
	switch strings.ToLower(state) {
	case "success":
		return "SUCCESS"
	case "inactive":
		return "CANCELLED"
	default:
		return "FAILURE"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonNilTime(values ...*time.Time) *time.Time {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func workflowTargetsProd(value string) bool {
	normalized := strings.ToLower(value)
	return strings.Contains(normalized, " prod") ||
		strings.Contains(normalized, "-prod") ||
		strings.Contains(normalized, "_prod") ||
		strings.Contains(normalized, " prd") ||
		strings.Contains(normalized, "-prd") ||
		strings.Contains(normalized, "_prd") ||
		strings.HasSuffix(normalized, "prod") ||
		strings.HasSuffix(normalized, "prd") ||
		strings.Contains(normalized, "production") ||
		strings.Contains(normalized, "produciton")
}

func deploymentTargetsProd(value string) bool {
	return workflowTargetsProd(value)
}

func latestSuccessfulGithubDeploymentStatus(statuses []githubRepoDeploymentStatusResponse) (githubRepoDeploymentStatusResponse, bool) {
	var latest githubRepoDeploymentStatusResponse
	found := false
	for _, status := range statuses {
		if strings.ToLower(status.State) != "success" {
			continue
		}
		if !found {
			latest = status
			found = true
			continue
		}
		currentTime := firstNonNilTime(status.UpdatedAt, status.CreatedAt)
		latestTime := firstNonNilTime(latest.UpdatedAt, latest.CreatedAt)
		if currentTime != nil && latestTime != nil && currentTime.After(*latestTime) {
			latest = status
		}
	}
	return latest, found
}

func sortGithubDeploymentCandidates(candidates []githubDeploymentCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		left := githubDeploymentExecutionTime(candidates[i].deployment)
		right := githubDeploymentExecutionTime(candidates[j].deployment)
		if left.Equal(right) {
			return candidates[i].deployment.Id < candidates[j].deployment.Id
		}
		return left.Before(right)
	})
}

func githubDeploymentExecutionTime(deployment webhookapi.WebhookDeploymentReq) time.Time {
	for _, candidate := range []*time.Time{deployment.StartedDate, deployment.FinishedDate, deployment.CreatedDate} {
		if candidate != nil {
			return *candidate
		}
	}
	return time.Time{}
}

func applyNewestPullRequestTitleToDeployments(
	deployments []webhookapi.WebhookDeploymentReq,
	deploymentPRLinks []githubDeploymentPRLink,
	selectedPRs []githubSelectedPullRequest,
) {
	if len(deployments) == 0 || len(deploymentPRLinks) == 0 || len(selectedPRs) == 0 {
		return
	}
	prByNumber := make(map[int]githubSelectedPullRequest, len(selectedPRs))
	for _, selected := range selectedPRs {
		prByNumber[selected.pr.Number] = selected
	}
	newestPRByDeploymentID := make(map[string]githubSelectedPullRequest)
	for _, link := range deploymentPRLinks {
		selected, ok := prByNumber[link.PullRequestKey]
		if !ok {
			continue
		}
		// deploymentPRLinks are appended in compare order, so the last seen PR for a
		// deployment is the newest matched merge commit in that deployment range.
		newestPRByDeploymentID[link.DeploymentId] = selected
	}
	for index := range deployments {
		selected, ok := newestPRByDeploymentID[deployments[index].Id]
		if !ok {
			continue
		}
		title := selected.pr.Title
		deployments[index].DisplayTitle = title
		if len(deployments[index].DeploymentCommits) > 0 {
			deployments[index].DeploymentCommits[0].DisplayTitle = title
			deployments[index].DeploymentCommits[0].CommitMsg = fmt.Sprintf("PR #%d %s", selected.pr.Number, selected.pr.Title)
		}
	}
}

func findGithubDeploymentIncludedPRNumbers(
	apiClient plugin.ApiClient,
	repoFullName string,
	teamPrefixes []string,
	candidates []githubDeploymentCandidate,
) ([]int, map[string]struct{}, map[string][]githubDeploymentPRLink, errors.Error) {
	logger := basicRes.GetLogger()
	includedPRNumbers := make(map[int]struct{})
	includedDeploymentIds := make(map[string]struct{})
	deploymentLinksById := make(map[string][]githubDeploymentPRLink)
	if len(candidates) < 2 || len(teamPrefixes) == 0 {
		return nil, includedDeploymentIds, deploymentLinksById, nil
	}
	for index := 1; index < len(candidates); index++ {
		previous := candidates[index-1]
		current := candidates[index]
		baseSHA := githubPrimaryDeploymentCommitSHA(previous.deployment)
		headSHA := githubPrimaryDeploymentCommitSHA(current.deployment)
		if baseSHA == "" || headSHA == "" {
			logger.Info(
				"skipping deployment comparison because one of the deployment commit SHAs is missing: previous=%s current=%s baseSha=%s headSha=%s",
				previous.deployment.Name,
				current.deployment.Name,
				baseSHA,
				headSHA,
			)
			continue
		}
		if baseSHA == headSHA {
			previousLinks, ok := deploymentLinksById[previous.deployment.Id]
			if !ok || len(previousLinks) == 0 {
				logger.Info(
					"skipping deployment comparison because consecutive deployments point to the same commit without prior matched PRs: previous=%s current=%s sha=%s",
					previous.deployment.Name,
					current.deployment.Name,
					headSHA,
				)
				continue
			}
			for _, link := range previousLinks {
				includedPRNumbers[link.PullRequestKey] = struct{}{}
				deploymentLinksById[current.deployment.Id] = append(deploymentLinksById[current.deployment.Id], githubDeploymentPRLink{
					DeploymentId:     current.deployment.Id,
					PullRequestKey:   link.PullRequestKey,
					MatchedCommitSha: link.MatchedCommitSha,
				})
			}
			includedDeploymentIds[current.deployment.Id] = struct{}{}
			logger.Info(
				"carried deployment PR matches forward because consecutive deployments point to the same commit: previous=%s current=%s sha=%s prs=%d",
				previous.deployment.Name,
				current.deployment.Name,
				headSHA,
				len(previousLinks),
			)
			continue
		}
		commits, err := fetchGithubComparedCommits(apiClient, repoFullName, baseSHA, headSHA)
		if err != nil {
			return nil, nil, nil, err
		}
		matchedCurrentDeployment := false
		seenPRsForDeployment := make(map[int]struct{})
		for _, commit := range commits {
			title := githubCommitTitle(commit.Commit.Message)
			prNumber, ok := githubMergeCommitPRNumber(title)
			if !ok || !githubCommitTitleMatchesAnyTeamPrefix(title, teamPrefixes) {
				continue
			}
			logger.Info("==== found deployments %s included %s ===", current.deployment.Name, prNumber)
			matchedCurrentDeployment = true
			if parsedPRNumber, err := strconv.Atoi(prNumber); err == nil {
				includedPRNumbers[parsedPRNumber] = struct{}{}
				if _, exists := seenPRsForDeployment[parsedPRNumber]; !exists {
					seenPRsForDeployment[parsedPRNumber] = struct{}{}
					deploymentLinksById[current.deployment.Id] = append(deploymentLinksById[current.deployment.Id], githubDeploymentPRLink{
						DeploymentId:     current.deployment.Id,
						PullRequestKey:   parsedPRNumber,
						MatchedCommitSha: commit.SHA,
					})
				}
			}
		}
		if matchedCurrentDeployment {
			includedDeploymentIds[current.deployment.Id] = struct{}{}
		}
	}
	return githubSortedPRNumbers(includedPRNumbers), includedDeploymentIds, deploymentLinksById, nil
}

func githubCommitTitleMatchesAnyTeamPrefix(title string, teamPrefixes []string) bool {
	normalizedTitle := strings.ToUpper(strings.TrimSpace(title))
	for _, teamPrefix := range teamPrefixes {
		if strings.HasPrefix(normalizedTitle, teamPrefix+"-") {
			return true
		}
	}
	return false
}

func githubPrimaryDeploymentCommitSHA(deployment webhookapi.WebhookDeploymentReq) string {
	for _, commit := range deployment.DeploymentCommits {
		if strings.TrimSpace(commit.CommitSha) != "" {
			return commit.CommitSha
		}
	}
	return ""
}

func githubCommitTitle(message string) string {
	title, _, _ := strings.Cut(message, "\n")
	return strings.TrimSpace(title)
}

func githubMergeCommitPRNumber(title string) (string, bool) {
	match := githubMergeCommitPRNumberMatcher.FindStringSubmatch(title)
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func githubSortedPRNumbers(values map[int]struct{}) []int {
	if len(values) == 0 {
		return nil
	}
	numbers := make([]int, 0, len(values))
	for value := range values {
		numbers = append(numbers, value)
	}
	sort.Ints(numbers)
	return numbers
}

func githubApiGetAndUnmarshalWithRetry(apiClient plugin.ApiClient, path string, query url.Values, target interface{}) errors.Error {
	logger := basicRes.GetLogger()
	var lastErr errors.Error
	for attempt := 0; attempt <= githubWebhookExportMaxRetry; attempt++ {
		res, err := apiClient.Get(path, query, nil)
		if err == nil {
			err = helper.UnmarshalResponse(res, target)
		}
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == githubWebhookExportMaxRetry {
			break
		}
		logger.Warn(err, "retry #%d calling %s", attempt+1, path)
		time.Sleep(githubWebhookExportRetryDelay)
	}
	return errors.Default.Wrap(lastErr, fmt.Sprintf("Retry exceeded %d times calling %s", githubWebhookExportMaxRetry, path))
}

func intPtr(value int) *int {
	return &value
}

func (e *githubWebhookPreparedExport) addCall(entity string, webhookConnectionId uint64, route string, payload interface{}) errors.Error {
	body, err := toBodyMap(payload)
	if err != nil {
		return err
	}
	e.calls = append(e.calls, GithubWebhookPreparedCall{
		Entity:   entity,
		Method:   http.MethodPost,
		Endpoint: fmt.Sprintf("/plugins/webhook/connections/%d/%s", webhookConnectionId, route),
		Payload:  body,
	})
	return nil
}

func (e *githubWebhookPreparedExport) findDeploymentCommitId(connectionID uint64, deploymentID string) (string, bool) {
	for i := range e.deployments {
		deployment := e.deployments[i]
		if deployment.Id != deploymentID || len(deployment.DeploymentCommits) == 0 {
			continue
		}
		commit := deployment.DeploymentCommits[0]
		if strings.TrimSpace(commit.RepoUrl) == "" || strings.TrimSpace(commit.CommitSha) == "" {
			return "", false
		}
		return webhookapi.GenerateDeploymentCommitId(connectionID, deployment.Id, commit.RepoUrl, commit.CommitSha), true
	}
	return "", false
}

func toBodyMap(payload interface{}) (map[string]interface{}, errors.Error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.Convert(err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, errors.Convert(err)
	}
	return body, nil
}

func logGithubWebhookPreparedCalls(calls []GithubWebhookPreparedCall) {
	logger := basicRes.GetLogger()
	for _, call := range calls {
		payload, err := json.MarshalIndent(call.Payload, "", "  ")
		if err != nil {
			logger.Error(err, "failed to marshal prepared webhook payload for logging")
			continue
		}
		logger.Info(
			"Prepared webhook %s %s for %s:\n%s",
			call.Method,
			call.Endpoint,
			call.Entity,
			string(payload),
		)
	}
}
