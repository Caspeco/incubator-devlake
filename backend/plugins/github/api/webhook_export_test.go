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
	if workflowTargetsProd("Deploy staging") {
		t.Fatal("did not expect staging workflow to be detected as production")
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
	if deployment.DeploymentCommits[0].CommitSha != "merge-sha-7" {
		t.Fatalf("expected deployment commit sha merge-sha-7, got %s", deployment.DeploymentCommits[0].CommitSha)
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
