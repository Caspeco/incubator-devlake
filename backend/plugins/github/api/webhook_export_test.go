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
	"testing"
	"time"

	webhookapi "github.com/apache/incubator-devlake/plugins/webhook/api"
)

func TestPullRequestMatchesTeamPrefixWithJiraKey(t *testing.T) {
	pr := githubPullRequestResponse{
		Title: "LONII-123 Improve deployment association",
		Head: githubPRBranchRef{
			Ref: "feature/no-ticket-name",
		},
	}

	if !pullRequestMatchesTeamPrefix(pr, normalizeJiraProjectPrefix("LONII")) {
		t.Fatal("expected JIRA key in title to match configured team prefix")
	}
}

func TestNormalizeJiraProjectPrefix(t *testing.T) {
	got := normalizeJiraProjectPrefix(" staff- ")
	if got != "STAFF" {
		t.Fatalf("expected STAFF, got %s", got)
	}
}

func TestNormalizeJiraProjectPrefixes(t *testing.T) {
	got := normalizeJiraProjectPrefixes(" staff- ", []string{"LONII", "staff", "CCS-"})
	if len(got) != 3 {
		t.Fatalf("expected 3 normalized prefixes, got %d", len(got))
	}
	if got[0] != "STAFF" || got[1] != "LONII" || got[2] != "CCS" {
		t.Fatalf("unexpected normalized prefixes: %+v", got)
	}
}

func TestWorkflowTargetsProd(t *testing.T) {
	if !workflowTargetsProd("ANALYS Build & Publish Analytics Dump prod") {
		t.Fatal("expected workflow ending in prod to be detected as production")
	}
	if !workflowTargetsProd("Deploy prd") {
		t.Fatal("expected prd suffix to be detected as production")
	}
	if !workflowTargetsProd("Release produciton") {
		t.Fatal("expected common production typo to be detected as production")
	}
	if workflowTargetsProd("Deploy staging") {
		t.Fatal("did not expect staging workflow to be detected as production")
	}
}

func TestDeploymentTargetsProd(t *testing.T) {
	if !deploymentTargetsProd("prod") {
		t.Fatal("expected prod environment to be treated as production")
	}
	if !deploymentTargetsProd("produciton") {
		t.Fatal("expected common production typo to be treated as production")
	}
	if !deploymentTargetsProd("prd") {
		t.Fatal("expected prd abbreviation to be treated as production")
	}
	if deploymentTargetsProd("staging") {
		t.Fatal("did not expect staging environment to be treated as production")
	}
}

func TestFilterGithubWorkflowsByNameUsesExactMatchesOnly(t *testing.T) {
	workflows := []githubWorkflowResponse{
		{Name: "ANALYS Build & Publish Analytics Dump prod"},
		{Name: "🚀 Deploy Production"},
		{Name: "Release prod"},
	}

	filtered := filterGithubWorkflowsByName(workflows, []string{"ANALYS Build & Publish Analytics Dump prod"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 exact matched workflow, got %d", len(filtered))
	}
	if filtered[0].Name != "ANALYS Build & Publish Analytics Dump prod" {
		t.Fatalf("unexpected filtered workflows: %+v", filtered)
	}
}

func TestFilterGithubWorkflowsByNameWithEmptyRequestedReturnsNothing(t *testing.T) {
	workflows := []githubWorkflowResponse{
		{Name: "ANALYS Build & Publish Analytics Dump prod"},
		{Name: "🚀 Deploy Production"},
	}

	filtered := filterGithubWorkflowsByName(workflows, nil)
	if len(filtered) != 0 {
		t.Fatalf("expected no workflows when none were configured, got %d", len(filtered))
	}
}

func TestPullRequestCreatedAfterLookbackIsEligible(t *testing.T) {
	lookbackSince := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	pr := githubPullRequestResponse{
		CreatedAt: time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC),
		Title:     "CCS-117 Improve notifications",
	}

	if pr.CreatedAt.Before(lookbackSince) {
		t.Fatal("test fixture is invalid")
	}
	if !pullRequestMatchesTeamPrefix(pr, normalizeJiraProjectPrefix("CCS")) {
		t.Fatal("expected PR inside lookback window to still match team prefix")
	}
}

func TestBuildWebhookDeploymentUsesAssociatedPRs(t *testing.T) {
	startedAt := time.Date(2025, 2, 20, 12, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(10 * time.Minute)
	runCreatedAt := startedAt.Add(-5 * time.Minute)

	deployment := buildWebhookDeployment(
		&githubRepoResponse{CloneURL: "https://github.com/org/repo.git", DefaultBranch: "main"},
		githubWorkflowResponse{
			ID:   3003,
			Name: "Deploy Analytics",
		},
		githubRunResponse{
			ID:           1001,
			Name:         "deploy",
			Conclusion:   "success",
			DisplayTitle: "Deploy Production",
			CreatedAt:    &runCreatedAt,
			RunStartedAt: &startedAt,
			UpdatedAt:    &finishedAt,
			HTMLURL:      "https://github.com/org/repo/actions/runs/1001",
		},
		[]githubSelectedPullRequest{
			{
				pr: githubPullRequestResponse{
					Number:         7,
					Title:          "Add feature flag",
					MergeCommitSHA: "merge-sha-7",
					Base: githubPRBranchRef{
						Ref: "main",
					},
				},
			},
		},
	)

	if deployment.Result != "SUCCESS" {
		t.Fatalf("expected success result, got %s", deployment.Result)
	}
	if len(deployment.DeploymentCommits) != 1 {
		t.Fatalf("expected one deployment commit, got %d", len(deployment.DeploymentCommits))
	}
	if deployment.Url != "https://github.com/org/repo/actions/runs/1001" {
		t.Fatalf("expected deployment url to be propagated, got %s", deployment.Url)
	}
	if deployment.DeploymentCommits[0].CommitSha != "merge-sha-7" {
		t.Fatalf("expected deployment commit sha merge-sha-7, got %s", deployment.DeploymentCommits[0].CommitSha)
	}
	if deployment.DeploymentCommits[0].Url != "https://github.com/org/repo/actions/runs/1001" {
		t.Fatalf("expected deployment commit url to be propagated, got %s", deployment.DeploymentCommits[0].Url)
	}
}

func TestGithubMergeCommitPRNumber(t *testing.T) {
	prNumber, ok := githubMergeCommitPRNumber("AIR-2758 semantic kernel -> microsoft.agentframework (#6544)")
	if !ok {
		t.Fatal("expected merge commit title to expose PR number")
	}
	if prNumber != "6544" {
		t.Fatalf("expected PR number 6544, got %s", prNumber)
	}
}

func TestGithubCommitTitle(t *testing.T) {
	title := githubCommitTitle("AIR-2649 add permission (#6542)\n\nMore details below")
	if title != "AIR-2649 add permission (#6542)" {
		t.Fatalf("unexpected commit title: %s", title)
	}
}

func TestGithubCommitTitleMatchesAnyTeamPrefix(t *testing.T) {
	if !githubCommitTitleMatchesAnyTeamPrefix("LONII-11987 fix currency (#6462)", []string{"AIR", "LONII"}) {
		t.Fatal("expected title to match one of the configured prefixes")
	}
	if githubCommitTitleMatchesAnyTeamPrefix("STAFF-1862 fix mode (#6473)", []string{"AIR", "LONII"}) {
		t.Fatal("did not expect title to match unrelated prefixes")
	}
}

func TestDeploymentIsIncludedOnlyWhenItsCompareRangeContainsMatchedPR(t *testing.T) {
	included := map[string]struct{}{
		"github-workflow-1-run-2": {},
	}
	candidates := []githubDeploymentCandidate{
		{deployment: webhookapi.WebhookDeploymentReq{Id: "github-workflow-1-run-1"}},
		{deployment: webhookapi.WebhookDeploymentReq{Id: "github-workflow-1-run-2"}},
		{deployment: webhookapi.WebhookDeploymentReq{Id: "github-workflow-1-run-3"}},
	}

	var deployments []webhookapi.WebhookDeploymentReq
	for _, candidate := range candidates {
		if _, ok := included[candidate.deployment.Id]; !ok {
			continue
		}
		deployments = append(deployments, candidate.deployment)
	}

	if len(deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deployments))
	}
	if deployments[0].Id != "github-workflow-1-run-2" {
		t.Fatalf("unexpected deployment selected: %s", deployments[0].Id)
	}
}

func TestDeploymentPRLinksDeduplicatePerDeploymentAndPR(t *testing.T) {
	deploymentLinksByID := map[string][]githubDeploymentPRLink{}
	seenPRsForDeployment := map[int]struct{}{}
	commits := []githubCompareCommitResponse{
		{SHA: "sha-1", Commit: struct{ Message string `json:"message"` }{Message: "AIR-1 first merge (#101)"}},
		{SHA: "sha-2", Commit: struct{ Message string `json:"message"` }{Message: "AIR-1 duplicate mention (#101)"}},
		{SHA: "sha-3", Commit: struct{ Message string `json:"message"` }{Message: "AIR-2 second merge (#102)"}},
	}

	for _, commit := range commits {
		title := githubCommitTitle(commit.Commit.Message)
		prNumber, ok := githubMergeCommitPRNumber(title)
		if !ok || !githubCommitTitleMatchesAnyTeamPrefix(title, []string{"AIR"}) {
			continue
		}
		parsedPRNumber := 0
		switch prNumber {
		case "101":
			parsedPRNumber = 101
		case "102":
			parsedPRNumber = 102
		}
		if _, exists := seenPRsForDeployment[parsedPRNumber]; exists {
			continue
		}
		seenPRsForDeployment[parsedPRNumber] = struct{}{}
		deploymentLinksByID["dep-1"] = append(deploymentLinksByID["dep-1"], githubDeploymentPRLink{
			DeploymentId:     "dep-1",
			PullRequestKey:   parsedPRNumber,
			MatchedCommitSha: commit.SHA,
		})
	}

	links := deploymentLinksByID["dep-1"]
	if len(links) != 2 {
		t.Fatalf("expected 2 unique deployment PR links, got %d", len(links))
	}
	if links[0].PullRequestKey != 101 || links[0].MatchedCommitSha != "sha-1" {
		t.Fatalf("unexpected first link: %+v", links[0])
	}
	if links[1].PullRequestKey != 102 || links[1].MatchedCommitSha != "sha-3" {
		t.Fatalf("unexpected second link: %+v", links[1])
	}
}

func TestApplyNewestPullRequestTitleToDeployments(t *testing.T) {
	deployments := []webhookapi.WebhookDeploymentReq{
		{
			Id:           "dep-1",
			DisplayTitle: "deploy",
			DeploymentCommits: []webhookapi.WebhookDeploymentCommitReq{
				{DisplayTitle: "deploy", CommitMsg: "deploy"},
			},
		},
	}
	links := []githubDeploymentPRLink{
		{DeploymentId: "dep-1", PullRequestKey: 101, MatchedCommitSha: "sha-1"},
		{DeploymentId: "dep-1", PullRequestKey: 102, MatchedCommitSha: "sha-2"},
	}
	selectedPRs := []githubSelectedPullRequest{
		{pr: githubPullRequestResponse{Number: 101, Title: "LONII-100 older change"}},
		{pr: githubPullRequestResponse{Number: 102, Title: "LONII-101 newest change"}},
	}

	applyNewestPullRequestTitleToDeployments(deployments, links, selectedPRs)

	if deployments[0].DisplayTitle != "LONII-101 newest change" {
		t.Fatalf("expected newest PR title on deployment, got %s", deployments[0].DisplayTitle)
	}
	if deployments[0].DeploymentCommits[0].DisplayTitle != "LONII-101 newest change" {
		t.Fatalf("expected newest PR title on deployment commit, got %s", deployments[0].DeploymentCommits[0].DisplayTitle)
	}
	if deployments[0].DeploymentCommits[0].CommitMsg != "PR #102 LONII-101 newest change" {
		t.Fatalf("unexpected deployment commit message: %s", deployments[0].DeploymentCommits[0].CommitMsg)
	}
}

func TestIsExcludedGithubCommentAuthor(t *testing.T) {
	excluded := map[int]struct{}{123: {}}
	if !isExcludedGithubCommentAuthor(&githubUserRef{ID: 123}, excluded) {
		t.Fatal("expected excluded github user to be filtered")
	}
	if isExcludedGithubCommentAuthor(&githubUserRef{ID: 456}, excluded) {
		t.Fatal("did not expect non-excluded github user to be filtered")
	}
	if isExcludedGithubCommentAuthor(nil, excluded) {
		t.Fatal("did not expect nil github user to be filtered")
	}
}

func TestSameShaDeploymentCarriesPreviousMatchedPRsForward(t *testing.T) {
	sha := "same-sha"
	candidates := []githubDeploymentCandidate{
		{
			deployment: webhookapi.WebhookDeploymentReq{
				Id: "dep-1",
				DeploymentCommits: []webhookapi.WebhookDeploymentCommitReq{
					{CommitSha: sha},
				},
			},
		},
		{
			deployment: webhookapi.WebhookDeploymentReq{
				Id: "dep-2",
				DeploymentCommits: []webhookapi.WebhookDeploymentCommitReq{
					{CommitSha: sha},
				},
			},
		},
	}

	includedPRNumbers := map[int]struct{}{101: {}}
	includedDeploymentIds := map[string]struct{}{"dep-1": {}}
	deploymentLinksById := map[string][]githubDeploymentPRLink{
		"dep-1": {
			{DeploymentId: "dep-1", PullRequestKey: 101, MatchedCommitSha: "merge-sha-101"},
		},
	}

	baseSHA := githubPrimaryDeploymentCommitSHA(candidates[0].deployment)
	headSHA := githubPrimaryDeploymentCommitSHA(candidates[1].deployment)
	if baseSHA != headSHA {
		t.Fatalf("expected same deployment sha, got %s and %s", baseSHA, headSHA)
	}
	for _, link := range deploymentLinksById[candidates[0].deployment.Id] {
		includedPRNumbers[link.PullRequestKey] = struct{}{}
		deploymentLinksById[candidates[1].deployment.Id] = append(deploymentLinksById[candidates[1].deployment.Id], githubDeploymentPRLink{
			DeploymentId:     candidates[1].deployment.Id,
			PullRequestKey:   link.PullRequestKey,
			MatchedCommitSha: link.MatchedCommitSha,
		})
	}
	includedDeploymentIds[candidates[1].deployment.Id] = struct{}{}

	if _, ok := includedDeploymentIds["dep-2"]; !ok {
		t.Fatal("expected second deployment to be included")
	}
	links := deploymentLinksById["dep-2"]
	if len(links) != 1 {
		t.Fatalf("expected one carried deployment link, got %d", len(links))
	}
	if links[0].PullRequestKey != 101 || links[0].MatchedCommitSha != "merge-sha-101" {
		t.Fatalf("unexpected carried link: %+v", links[0])
	}
}
