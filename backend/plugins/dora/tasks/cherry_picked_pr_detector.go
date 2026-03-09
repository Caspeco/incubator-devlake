/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tasks

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/plugins/dora/models"
)

var DetectCherryPickedPullRequestsMeta = plugin.SubTaskMeta{
	Name:             "detectCherryPickedPullRequests",
	EntryPoint:       DetectCherryPickedPullRequests,
	EnabledByDefault: true,
	Description:      "detect PR references like (#1234) from deployment commit diffs for cherry-picked deployment flows",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_CICD, plugin.DOMAIN_TYPE_CODE},
}

var cherryPickedPrPattern = regexp.MustCompile(`\(#(\d+)\)`)

type deploymentPairForCherryPick struct {
	Id            string
	CicdScopeId   string
	CommitSha     string
	PrevCommitSha string
}

type commitMessageForCherryPick struct {
	CommitSha string
	Message   []byte
}

type githubRepoScopeRef struct {
	ScopeConfigId uint64
}

type githubScopeConfigFlag struct {
	AutodetectCherryPickedPrs bool
}

type pullRequestForCherryPick struct {
	Id             string
	PullRequestKey int
}

func DetectCherryPickedPullRequests(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*DoraTaskData)
	projectName := data.Options.ProjectName
	if projectName == "" {
		return nil
	}

	err := db.Delete(
		&models.DeploymentCommitPullRequest{},
		dal.Where("project_name = ?", projectName),
	)
	if err != nil {
		return err
	}

	pairs := make([]*deploymentPairForCherryPick, 0)
	err = db.All(
		&pairs,
		dal.Select("dc.id, dc.cicd_scope_id, dc.commit_sha, p.commit_sha as prev_commit_sha"),
		dal.From("cicd_deployment_commits dc"),
		dal.Join("LEFT JOIN cicd_deployment_commits p ON (dc.prev_success_deployment_commit_id = p.id)"),
		dal.Join("LEFT JOIN project_mapping pm ON (pm.table = 'cicd_scopes' AND pm.row_id = dc.cicd_scope_id)"),
		dal.Where("pm.project_name = ?", projectName),
		dal.Where("dc.result = 'SUCCESS'"),
		dal.Where("dc.environment = 'PRODUCTION'"),
		dal.Where("dc.prev_success_deployment_commit_id <> ''"),
	)
	if err != nil {
		return err
	}

	enabledByScope := map[string]bool{}
	prCache := map[string]*pullRequestForCherryPick{}

	for _, pair := range pairs {
		enabled, err := isCherryPickAutodetectEnabledForScope(db, pair.CicdScopeId, enabledByScope)
		if err != nil {
			return err
		}
		if !enabled {
			continue
		}

		commits := make([]*commitMessageForCherryPick, 0)
		err = db.All(
			&commits,
			dal.Select("cd.commit_sha, c.message"),
			dal.From("commits_diffs cd"),
			dal.Join("INNER JOIN commits c ON (c.sha = cd.commit_sha)"),
			dal.Where("cd.new_commit_sha = ? AND cd.old_commit_sha = ?", pair.CommitSha, pair.PrevCommitSha),
		)
		if err != nil {
			return err
		}

		for _, commit := range commits {
			matches := cherryPickedPrPattern.FindAllSubmatch(commit.Message, -1)
			if len(matches) == 0 {
				continue
			}
			seenPrNumbers := map[int]struct{}{}
			for _, match := range matches {
				if len(match) < 2 {
					continue
				}
				prNumber, convErr := strconv.Atoi(string(match[1]))
				if convErr != nil {
					continue
				}
				if _, exists := seenPrNumbers[prNumber]; exists {
					continue
				}
				seenPrNumbers[prNumber] = struct{}{}

				pr, err := findPullRequestByScopeAndNumber(db, pair.CicdScopeId, prNumber, prCache)
				if err != nil {
					return err
				}
				if pr == nil {
					continue
				}

				record := &models.DeploymentCommitPullRequest{
					ProjectName:        projectName,
					DeploymentCommitId: pair.Id,
					PullRequestId:      pr.Id,
					MatchedCommitSha:   commit.CommitSha,
					PullRequestKey:     pr.PullRequestKey,
					DetectionMethod:    "commit_message_reference",
				}
				err = db.CreateOrUpdate(record)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func isCherryPickAutodetectEnabledForScope(db dal.Dal, scopeId string, cache map[string]bool) (bool, errors.Error) {
	if cached, ok := cache[scopeId]; ok {
		return cached, nil
	}

	connectionId, githubId, ok := parseGithubRepoScopeId(scopeId)
	if !ok {
		cache[scopeId] = false
		return false, nil
	}

	scopeRef := &githubRepoScopeRef{}
	err := db.First(
		scopeRef,
		dal.From("_tool_github_repos"),
		dal.Select("scope_config_id"),
		dal.Where("connection_id = ? AND github_id = ?", connectionId, githubId),
	)
	if err != nil {
		if db.IsErrorNotFound(err) {
			cache[scopeId] = false
			return false, nil
		}
		return false, err
	}
	if scopeRef.ScopeConfigId == 0 {
		cache[scopeId] = false
		return false, nil
	}

	scopeConfigFlag := &githubScopeConfigFlag{}
	err = db.First(
		scopeConfigFlag,
		dal.From("_tool_github_scope_configs"),
		dal.Select("autodetect_cherry_picked_prs"),
		dal.Where("id = ?", scopeRef.ScopeConfigId),
	)
	if err != nil {
		if db.IsErrorNotFound(err) {
			cache[scopeId] = false
			return false, nil
		}
		return false, err
	}

	cache[scopeId] = scopeConfigFlag.AutodetectCherryPickedPrs
	return scopeConfigFlag.AutodetectCherryPickedPrs, nil
}

func parseGithubRepoScopeId(scopeId string) (uint64, int, bool) {
	parts := strings.Split(scopeId, ":")
	if len(parts) != 4 || parts[0] != "github" || parts[1] != "GithubRepo" {
		return 0, 0, false
	}
	connectionId, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	githubId, err := strconv.Atoi(parts[3])
	if err != nil {
		return 0, 0, false
	}
	return connectionId, githubId, true
}

func findPullRequestByScopeAndNumber(
	db dal.Dal,
	scopeId string,
	prNumber int,
	cache map[string]*pullRequestForCherryPick,
) (*pullRequestForCherryPick, errors.Error) {
	cacheKey := scopeId + ":" + strconv.Itoa(prNumber)
	if cached, ok := cache[cacheKey]; ok {
		return cached, nil
	}

	pr := &pullRequestForCherryPick{}
	err := db.First(
		pr,
		dal.From("pull_requests"),
		dal.Select("id, pull_request_key"),
		dal.Where("base_repo_id = ? AND pull_request_key = ?", scopeId, prNumber),
		dal.Orderby("created_date DESC"),
	)
	if err != nil {
		if db.IsErrorNotFound(err) {
			cache[cacheKey] = nil
			return nil, nil
		}
		return nil, err
	}
	cache[cacheKey] = pr
	return pr, nil
}
