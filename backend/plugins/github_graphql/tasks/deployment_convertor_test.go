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
	"testing"

	"github.com/apache/incubator-devlake/core/config"
	corecontext "github.com/apache/incubator-devlake/core/context"
	"github.com/apache/incubator-devlake/core/dal"
	"github.com/apache/incubator-devlake/core/log"
	"github.com/apache/incubator-devlake/core/plugin"
	githubModels "github.com/apache/incubator-devlake/plugins/github/models"
	githubTasks "github.com/apache/incubator-devlake/plugins/github/tasks"
	"github.com/stretchr/testify/assert"
)

type noopLogger struct{}

func (noopLogger) IsLevelEnabled(level log.LogLevel) bool                  { return false }
func (noopLogger) Printf(format string, a ...interface{})                  {}
func (noopLogger) Log(level log.LogLevel, format string, a ...interface{}) {}
func (noopLogger) Debug(format string, a ...interface{})                   {}
func (noopLogger) Info(format string, a ...interface{})                    {}
func (noopLogger) Warn(err error, format string, a ...interface{})         {}
func (noopLogger) Error(err error, format string, a ...interface{})        {}
func (noopLogger) Nested(name string) log.Logger                           { return noopLogger{} }
func (noopLogger) GetConfig() *log.LoggerConfig                            { return &log.LoggerConfig{} }
func (noopLogger) SetStream(config *log.LoggerStreamConfig)                {}

type fakeSubTaskContext struct {
	data interface{}
}

func (f *fakeSubTaskContext) GetConfigReader() config.ConfigReader          { return nil }
func (f *fakeSubTaskContext) GetConfig(name string) string                  { return "" }
func (f *fakeSubTaskContext) GetLogger() log.Logger                         { return noopLogger{} }
func (f *fakeSubTaskContext) NestedLogger(name string) corecontext.BasicRes { return f }
func (f *fakeSubTaskContext) ReplaceLogger(logger log.Logger) corecontext.BasicRes {
	return f
}
func (f *fakeSubTaskContext) GetDal() dal.Dal                    { return nil }
func (f *fakeSubTaskContext) GetName() string                    { return "test" }
func (f *fakeSubTaskContext) GetContext() context.Context        { return context.Background() }
func (f *fakeSubTaskContext) GetData() interface{}               { return f.data }
func (f *fakeSubTaskContext) SetProgress(current int, total int) {}
func (f *fakeSubTaskContext) IncProgress(quantity int)           {}
func (f *fakeSubTaskContext) TaskContext() plugin.TaskContext    { return nil }

func TestConvertDeployment_SkipsWhenConvertGithubDeploymentDisabled(t *testing.T) {
	disabled := false
	taskData := &githubTasks.GithubTaskData{
		Options: &githubTasks.GithubOptions{
			ConnectionId: 1,
			GithubId:     123,
			Name:         "apache/incubator-devlake",
			ScopeConfig: &githubModels.GithubScopeConfig{
				ConvertGithubDeployment: &disabled,
			},
		},
	}
	ctx := &fakeSubTaskContext{data: taskData}

	assert.NotPanics(t, func() {
		assert.Nil(t, ConvertDeployment(ctx))
	})
}
