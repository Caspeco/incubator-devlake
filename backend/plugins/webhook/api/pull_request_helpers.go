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

package api

import (
	"fmt"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/domainlayer/code"
	"github.com/apache/incubator-devlake/plugins/webhook/models"
)

func resolvePullRequestId(connection *models.WebhookConnection, pullRequestId string, pullRequestKey *int) (string, errors.Error) {
	return resolvePullRequestIdWithDal(basicRes.GetDal(), connection, pullRequestId, pullRequestKey)
}

func resolvePullRequestIdWithDal(db dal.Dal, connection *models.WebhookConnection, pullRequestId string, pullRequestKey *int) (string, errors.Error) {
	if pullRequestId != "" {
		return pullRequestId, nil
	}
	if pullRequestKey == nil {
		return "", errors.BadInput.New("either pullRequestId or pullRequestKey is required")
	}

	pr := &code.PullRequest{}
	err := db.First(
		pr,
		dal.From(&code.PullRequest{}),
		dal.Select("id"),
		dal.Where("base_repo_id = ? AND pull_request_key = ?", fmt.Sprintf("%s:%d", pluginName, connection.ID), *pullRequestKey),
	)
	if err != nil {
		if db.IsErrorNotFound(err) {
			return "", errors.NotFound.New(fmt.Sprintf("pull request not found for key %d", *pullRequestKey))
		}
		return "", err
	}
	return pr.Id, nil
}
