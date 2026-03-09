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
	"context"
	"database/sql"
	"reflect"
	"testing"

	corecontext "github.com/apache/incubator-devlake/core/context"
	"github.com/apache/incubator-devlake/core/config"
	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/log"
	coremodels "github.com/apache/incubator-devlake/core/models"
	"github.com/apache/incubator-devlake/core/plugin"
	doramodels "github.com/apache/incubator-devlake/plugins/dora/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGithubRepoScopeId(t *testing.T) {
	connectionId, githubId, ok := parseGithubRepoScopeId("github:GithubRepo:12:345")
	require.True(t, ok)
	assert.Equal(t, uint64(12), connectionId)
	assert.Equal(t, 345, githubId)

	_, _, ok = parseGithubRepoScopeId("gitlab:Project:12:345")
	assert.False(t, ok)
}

func TestIsCherryPickAutodetectEnabledForScopeCachesResult(t *testing.T) {
	db := &fakeDal{
		firstFunc: func(dst interface{}, clauses ...dal.Clause) errors.Error {
			switch typed := dst.(type) {
			case *githubRepoScopeRef:
				typed.ScopeConfigId = 99
			case *githubScopeConfigFlag:
				typed.AutodetectCherryPickedPrs = true
			default:
				t.Fatalf("unexpected First destination %T", dst)
			}
			return nil
		},
	}

	cache := map[string]bool{}
	enabled, err := isCherryPickAutodetectEnabledForScope(db, "github:GithubRepo:1:2", cache)
	require.NoError(t, err)
	assert.True(t, enabled)

	enabled, err = isCherryPickAutodetectEnabledForScope(db, "github:GithubRepo:1:2", cache)
	require.NoError(t, err)
	assert.True(t, enabled)
	assert.Equal(t, 2, db.firstCalls)
}

func TestFindPullRequestByScopeAndNumberCachesNotFound(t *testing.T) {
	notFoundErr := errors.NotFound.New("missing")
	db := &fakeDal{
		firstFunc: func(dst interface{}, clauses ...dal.Clause) errors.Error {
			return notFoundErr
		},
		isErrorNotFoundFunc: func(err error) bool {
			return err == notFoundErr
		},
	}

	cache := map[string]*pullRequestForCherryPick{}
	pr, err := findPullRequestByScopeAndNumber(db, "github:GithubRepo:1:2", 123, cache)
	require.NoError(t, err)
	assert.Nil(t, pr)

	pr, err = findPullRequestByScopeAndNumber(db, "github:GithubRepo:1:2", 123, cache)
	require.NoError(t, err)
	assert.Nil(t, pr)
	assert.Equal(t, 1, db.firstCalls)
}

func TestDetectCherryPickedPullRequestsCreatesUniqueMappingsForEnabledScopes(t *testing.T) {
	projectName := "demo"
	repoScope := "github:GithubRepo:1:2"
	disabledScope := "github:GithubRepo:1:3"

	db := &fakeDal{}
	db.deleteFunc = func(entity interface{}, clauses ...dal.Clause) errors.Error {
		_, ok := entity.(*doramodels.DeploymentCommitPullRequest)
		require.True(t, ok)
		return nil
	}
	db.allFunc = func(dst interface{}, clauses ...dal.Clause) errors.Error {
			switch typed := dst.(type) {
			case *[]*deploymentPairForCherryPick:
				*typed = []*deploymentPairForCherryPick{
					{Id: "deploy-1", CicdScopeId: repoScope, CommitSha: "new-sha", PrevCommitSha: "old-sha"},
					{Id: "deploy-2", CicdScopeId: disabledScope, CommitSha: "new-sha-2", PrevCommitSha: "old-sha-2"},
				}
			case *[]*commitMessageForCherryPick:
				*typed = []*commitMessageForCherryPick{
					{CommitSha: "commit-a", Message: []byte("cherry pick (#12) and (#12) plus (#34)")},
					{CommitSha: "commit-b", Message: []byte("no pr reference here")},
				}
			default:
				t.Fatalf("unexpected All destination %T", dst)
			}
			return nil
	}
	db.firstFunc = func(dst interface{}, clauses ...dal.Clause) errors.Error {
			switch typed := dst.(type) {
			case *githubRepoScopeRef:
				switch db.firstCalls {
				case 1:
					typed.ScopeConfigId = 100
				case 5:
					typed.ScopeConfigId = 200
				default:
					t.Fatalf("unexpected githubRepoScopeRef call %d", db.firstCalls)
				}
			case *githubScopeConfigFlag:
				switch db.firstCalls {
				case 2:
					typed.AutodetectCherryPickedPrs = true
				case 6:
					typed.AutodetectCherryPickedPrs = false
				default:
					t.Fatalf("unexpected githubScopeConfigFlag call %d", db.firstCalls)
				}
			case *pullRequestForCherryPick:
				switch db.firstCalls {
				case 3:
					typed.Id = "pr-12"
					typed.PullRequestKey = 12
				case 4:
					typed.Id = "pr-34"
					typed.PullRequestKey = 34
				default:
					t.Fatalf("unexpected pullRequestForCherryPick call %d", db.firstCalls)
				}
			default:
				t.Fatalf("unexpected First destination %T", dst)
			}
			return nil
	}

	taskCtx := &fakeSubTaskContext{
		db:     db,
		logger: &noopLogger{},
		data: &DoraTaskData{
			Options: &DoraOptions{ProjectName: projectName},
		},
	}

	err := DetectCherryPickedPullRequests(taskCtx)
	require.NoError(t, err)

	require.Len(t, db.createdRecords, 2)
	assert.Equal(t, &doramodels.DeploymentCommitPullRequest{
		ProjectName:        projectName,
		DeploymentCommitId: "deploy-1",
		PullRequestId:      "pr-12",
		MatchedCommitSha:   "commit-a",
		PullRequestKey:     12,
		DetectionMethod:    "commit_message_reference",
	}, db.createdRecords[0])
	assert.Equal(t, &doramodels.DeploymentCommitPullRequest{
		ProjectName:        projectName,
		DeploymentCommitId: "deploy-1",
		PullRequestId:      "pr-34",
		MatchedCommitSha:   "commit-a",
		PullRequestKey:     34,
		DetectionMethod:    "commit_message_reference",
	}, db.createdRecords[1])
	assert.Equal(t, 1, db.deleteCalls)
	assert.Equal(t, 2, db.allCalls)
}

type fakeDal struct {
	allCalls            int
	firstCalls          int
	deleteCalls         int
	createdRecords      []*doramodels.DeploymentCommitPullRequest
	allFunc             func(dst interface{}, clauses ...dal.Clause) errors.Error
	firstFunc           func(dst interface{}, clauses ...dal.Clause) errors.Error
	deleteFunc          func(entity interface{}, clauses ...dal.Clause) errors.Error
	createOrUpdateFunc  func(entity interface{}, clauses ...dal.Clause) errors.Error
	isErrorNotFoundFunc func(err error) bool
}

func (f *fakeDal) AutoMigrate(entity interface{}, clauses ...dal.Clause) errors.Error {
	panic("unexpected AutoMigrate call")
}

func (f *fakeDal) AddColumn(table, columnName string, columnType dal.ColumnType) errors.Error {
	panic("unexpected AddColumn call")
}

func (f *fakeDal) DropColumns(table string, columnName ...string) errors.Error {
	panic("unexpected DropColumns call")
}

func (f *fakeDal) Exec(query string, params ...interface{}) errors.Error {
	panic("unexpected Exec call")
}

func (f *fakeDal) Cursor(clauses ...dal.Clause) (dal.Rows, errors.Error) {
	panic("unexpected Cursor call")
}

func (f *fakeDal) Fetch(cursor dal.Rows, dst interface{}) errors.Error {
	panic("unexpected Fetch call")
}

func (f *fakeDal) All(dst interface{}, clauses ...dal.Clause) errors.Error {
	f.allCalls++
	return f.allFunc(dst, clauses...)
}

func (f *fakeDal) First(dst interface{}, clauses ...dal.Clause) errors.Error {
	f.firstCalls++
	return f.firstFunc(dst, clauses...)
}

func (f *fakeDal) Count(clauses ...dal.Clause) (int64, errors.Error) {
	panic("unexpected Count call")
}

func (f *fakeDal) Pluck(column string, dest interface{}, clauses ...dal.Clause) errors.Error {
	panic("unexpected Pluck call")
}

func (f *fakeDal) Create(entity interface{}, clauses ...dal.Clause) errors.Error {
	panic("unexpected Create call")
}

func (f *fakeDal) CreateWithMap(entity interface{}, record map[string]interface{}) errors.Error {
	panic("unexpected CreateWithMap call")
}

func (f *fakeDal) Update(entity interface{}, clauses ...dal.Clause) errors.Error {
	panic("unexpected Update call")
}

func (f *fakeDal) UpdateColumn(entityOrTable interface{}, columnName string, value interface{}, clauses ...dal.Clause) errors.Error {
	panic("unexpected UpdateColumn call")
}

func (f *fakeDal) UpdateColumns(entityOrTable interface{}, set []dal.DalSet, clauses ...dal.Clause) errors.Error {
	panic("unexpected UpdateColumns call")
}

func (f *fakeDal) UpdateAllColumn(entity interface{}, clauses ...dal.Clause) errors.Error {
	panic("unexpected UpdateAllColumn call")
}

func (f *fakeDal) CreateOrUpdate(entity interface{}, clauses ...dal.Clause) errors.Error {
	record, ok := entity.(*doramodels.DeploymentCommitPullRequest)
	if !ok {
		panic("unexpected CreateOrUpdate entity type")
	}
	f.createdRecords = append(f.createdRecords, record)
	if f.createOrUpdateFunc != nil {
		return f.createOrUpdateFunc(entity, clauses...)
	}
	return nil
}

func (f *fakeDal) CreateIfNotExist(entity interface{}, clauses ...dal.Clause) errors.Error {
	panic("unexpected CreateIfNotExist call")
}

func (f *fakeDal) Delete(entity interface{}, clauses ...dal.Clause) errors.Error {
	f.deleteCalls++
	return f.deleteFunc(entity, clauses...)
}

func (f *fakeDal) AllTables() ([]string, errors.Error) {
	panic("unexpected AllTables call")
}

func (f *fakeDal) DropTables(dst ...interface{}) errors.Error {
	panic("unexpected DropTables call")
}

func (f *fakeDal) HasTable(table interface{}) bool {
	panic("unexpected HasTable call")
}

func (f *fakeDal) HasColumn(table interface{}, columnName string) bool {
	panic("unexpected HasColumn call")
}

func (f *fakeDal) RenameTable(oldName, newName string) errors.Error {
	panic("unexpected RenameTable call")
}

func (f *fakeDal) GetColumns(dst dal.Tabler, filter func(columnMeta dal.ColumnMeta) bool) ([]dal.ColumnMeta, errors.Error) {
	panic("unexpected GetColumns call")
}

func (f *fakeDal) GetPrimaryKeyFields(t reflect.Type) []reflect.StructField {
	panic("unexpected GetPrimaryKeyFields call")
}

func (f *fakeDal) RenameColumn(table, oldColumnName, newColumnName string) errors.Error {
	panic("unexpected RenameColumn call")
}

func (f *fakeDal) ModifyColumnType(table, columnName, columnType string) errors.Error {
	panic("unexpected ModifyColumnType call")
}

func (f *fakeDal) DropIndexes(table string, indexes ...string) errors.Error {
	panic("unexpected DropIndexes call")
}

func (f *fakeDal) DropIndex(table string, columnNames ...string) errors.Error {
	panic("unexpected DropIndex call")
}

func (f *fakeDal) Dialect() string {
	panic("unexpected Dialect call")
}

func (f *fakeDal) Session(config dal.SessionConfig) dal.Dal {
	panic("unexpected Session call")
}

func (f *fakeDal) Begin() dal.Transaction {
	panic("unexpected Begin call")
}

func (f *fakeDal) IsErrorNotFound(err error) bool {
	if f.isErrorNotFoundFunc != nil {
		return f.isErrorNotFoundFunc(err)
	}
	return false
}

func (f *fakeDal) IsDuplicationError(err error) bool {
	panic("unexpected IsDuplicationError call")
}

func (f *fakeDal) RawCursor(query string, params ...interface{}) (*sql.Rows, errors.Error) {
	panic("unexpected RawCursor call")
}

type fakeSubTaskContext struct {
	db     dal.Dal
	logger log.Logger
	data   interface{}
}

func (f *fakeSubTaskContext) GetConfigReader() config.ConfigReader { return nil }
func (f *fakeSubTaskContext) GetConfig(name string) string         { return "" }
func (f *fakeSubTaskContext) GetLogger() log.Logger                { return f.logger }
func (f *fakeSubTaskContext) NestedLogger(name string) corecontext.BasicRes {
	return f
}
func (f *fakeSubTaskContext) ReplaceLogger(logger log.Logger) corecontext.BasicRes {
	f.logger = logger
	return f
}
func (f *fakeSubTaskContext) GetDal() dal.Dal             { return f.db }
func (f *fakeSubTaskContext) GetName() string             { return "test" }
func (f *fakeSubTaskContext) GetContext() context.Context { return context.Background() }
func (f *fakeSubTaskContext) GetData() interface{}        { return f.data }
func (f *fakeSubTaskContext) SetProgress(current int, total int) {}
func (f *fakeSubTaskContext) IncProgress(quantity int)          {}
func (f *fakeSubTaskContext) TaskContext() plugin.TaskContext   { return nil }

type noopLogger struct{}

func (n *noopLogger) IsLevelEnabled(level log.LogLevel) bool                         { return false }
func (n *noopLogger) Printf(format string, a ...interface{})                         {}
func (n *noopLogger) Log(level log.LogLevel, format string, a ...interface{})        {}
func (n *noopLogger) Debug(format string, a ...interface{})                          {}
func (n *noopLogger) Info(format string, a ...interface{})                           {}
func (n *noopLogger) Warn(err error, format string, a ...interface{})                {}
func (n *noopLogger) Error(err error, format string, a ...interface{})               {}
func (n *noopLogger) Nested(name string) log.Logger                                  { return n }
func (n *noopLogger) GetConfig() *log.LoggerConfig                                   { return &log.LoggerConfig{} }
func (n *noopLogger) SetStream(config *log.LoggerStreamConfig)                       {}

var _ plugin.SubTaskContext = (*fakeSubTaskContext)(nil)
var _ dal.Dal = (*fakeDal)(nil)
var _ log.Logger = (*noopLogger)(nil)
var _ plugin.TaskContext = (*fakeTaskContext)(nil)

type fakeTaskContext struct{}

func (f *fakeTaskContext) GetConfigReader() config.ConfigReader                    { return nil }
func (f *fakeTaskContext) GetConfig(name string) string                            { return "" }
func (f *fakeTaskContext) GetLogger() log.Logger                                   { return &noopLogger{} }
func (f *fakeTaskContext) NestedLogger(name string) corecontext.BasicRes           { return f }
func (f *fakeTaskContext) ReplaceLogger(logger log.Logger) corecontext.BasicRes    { return f }
func (f *fakeTaskContext) GetDal() dal.Dal                                         { return nil }
func (f *fakeTaskContext) GetName() string                                         { return "test" }
func (f *fakeTaskContext) GetContext() context.Context                             { return context.Background() }
func (f *fakeTaskContext) GetData() interface{}                                    { return nil }
func (f *fakeTaskContext) SetProgress(current int, total int)                      {}
func (f *fakeTaskContext) IncProgress(quantity int)                                {}
func (f *fakeTaskContext) SetData(data interface{})                                {}
func (f *fakeTaskContext) SetSyncPolicy(syncPolicy *coremodels.SyncPolicy)         {}
func (f *fakeTaskContext) SyncPolicy() *coremodels.SyncPolicy                      { return nil }
func (f *fakeTaskContext) SubTaskContext(subtask string) (plugin.SubTaskContext, errors.Error) {
	return nil, nil
}
