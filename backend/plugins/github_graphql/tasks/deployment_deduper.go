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
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/plugins/github/models"
	githubTasks "github.com/apache/incubator-devlake/plugins/github/tasks"
)

var _ plugin.SubTaskEntryPoint = DedupDeployments

var DedupDeploymentsMeta = plugin.SubTaskMeta{
	Name:             "Deduplicate Deployments",
	EntryPoint:       DedupDeployments,
	EnabledByDefault: true,
	Description:      "deduplicate github deployments by ref_name and commit_oid, keeping the latest updated row",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_CICD},
	Dependencies:     []*plugin.SubTaskMeta{&ExtractDeploymentsMeta},
	DependencyTables: []string{models.GithubDeployment{}.TableName()},
	ProductTables:    []string{models.GithubDeployment{}.TableName()},
}

func DedupDeployments(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*githubTasks.GithubTaskData)
	db := taskCtx.GetDal()

	// Keep the latest row per deployment key for this repo in this run scope.
	return db.Exec(`
DELETE FROM _tool_github_deployments
WHERE (connection_id, id) IN (
	SELECT connection_id, id
	FROM (
		SELECT
			connection_id,
			id,
			ROW_NUMBER() OVER (
				PARTITION BY connection_id, github_id, environment, ref_name, commit_oid
				ORDER BY COALESCE(latest_updated_date, updated_date) DESC, id DESC
			) AS rn
		FROM _tool_github_deployments
		WHERE connection_id = ?
		  AND github_id = ?
		  AND ref_name IS NOT NULL AND ref_name <> ''
		  AND commit_oid IS NOT NULL AND commit_oid <> ''
	) ranked
	WHERE rn > 1
);`,
		data.Options.ConnectionId,
		data.Options.GithubId,
	)
}
