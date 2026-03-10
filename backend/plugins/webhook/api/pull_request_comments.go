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
	"net/http"
	"time"

	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/models/domainlayer"
	"github.com/apache/incubator-devlake/core/models/domainlayer/code"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/helpers/dbhelper"
	"github.com/apache/incubator-devlake/helpers/pluginhelper/api"
	"github.com/apache/incubator-devlake/plugins/webhook/models"
	"github.com/go-playground/validator/v10"
)

type WebhookPullRequestCommentReq struct {
	Id             string     `mapstructure:"id" validate:"required"`
	PullRequestId  string     `mapstructure:"pullRequestId"`
	PullRequestKey *int       `mapstructure:"pullRequestKey"`
	Body           string     `mapstructure:"body" validate:"required"`
	AccountId      string     `mapstructure:"accountId"`
	CreatedDate    *time.Time `mapstructure:"createdDate" validate:"required"`
	CommitSha      string     `mapstructure:"commitSha"`
	Type           string     `mapstructure:"type" validate:"omitempty,oneof=NORMAL DIFF REVIEW"`
	ReviewId       string     `mapstructure:"reviewId"`
	Status         string     `mapstructure:"status"`
}

func PostPullRequestComments(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	connection := &models.WebhookConnection{}
	err := connectionHelper.First(connection, input.Params)
	return postPullRequestComments(input, connection, err)
}

func PostPullRequestCommentsByName(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	connection := &models.WebhookConnection{}
	err := connectionHelper.FirstByName(connection, input.Params)
	return postPullRequestComments(input, connection, err)
}

func postPullRequestComments(input *plugin.ApiResourceInput, connection *models.WebhookConnection, err errors.Error) (*plugin.ApiResourceOutput, errors.Error) {
	if err != nil {
		return nil, err
	}
	request := &WebhookPullRequestCommentReq{}
	err = api.DecodeMapStruct(input.Body, request, true)
	if err != nil {
		return &plugin.ApiResourceOutput{Body: err.Error(), Status: http.StatusBadRequest}, nil
	}

	vld = validator.New()
	err = errors.Convert(vld.Struct(request))
	if err != nil {
		return nil, errors.BadInput.Wrap(vld.Struct(request), "input json error")
	}

	txHelper := dbhelper.NewTxHelper(basicRes, &err)
	defer txHelper.End()
	tx := txHelper.Begin()
	if err := CreatePullRequestComment(connection, request, tx); err != nil {
		logger.Error(err, "create pull request comment")
		return nil, err
	}

	return &plugin.ApiResourceOutput{Body: nil, Status: http.StatusOK}, nil
}

func CreatePullRequestComment(connection *models.WebhookConnection, request *WebhookPullRequestCommentReq, tx dal.Transaction) errors.Error {
	if request == nil {
		return errors.BadInput.New("request body is nil")
	}
	pullRequestId, err := resolvePullRequestId(connection, request.PullRequestId, request.PullRequestKey)
	if err != nil {
		return err
	}

	commentType := request.Type
	if commentType == "" {
		commentType = code.NORMAL_COMMENT
	}

	accountId := ""
	if request.AccountId != "" {
		accountId = fmt.Sprintf("%s:%d:%s", pluginName, connection.ID, request.AccountId)
	}

	prComment := &code.PullRequestComment{
		DomainEntity: domainlayer.DomainEntity{
			Id: fmt.Sprintf("%s:%d:%s", pluginName, connection.ID, request.Id),
		},
		PullRequestId: pullRequestId,
		Body:          request.Body,
		AccountId:     accountId,
		CreatedDate:   *request.CreatedDate,
		CommitSha:     request.CommitSha,
		Type:          commentType,
		ReviewId:      request.ReviewId,
		Status:        request.Status,
	}
	return tx.CreateOrUpdate(prComment)
}
