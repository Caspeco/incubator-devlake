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

package migrationscripts

import (
	"github.com/apache/incubator-devlake/core/context"
	"github.com/apache/incubator-devlake/core/errors"
	"github.com/apache/incubator-devlake/core/plugin"
	"github.com/apache/incubator-devlake/helpers/migrationhelper"
)

var _ plugin.MigrationScript = (*addPrTitlePrefixTeamMappingsToScopeConfigs)(nil)

type scopeConfig20260305 struct {
	PrTitlePrefixTeamMappings string `mapstructure:"prTitlePrefixTeamMappings,omitempty" json:"prTitlePrefixTeamMappings" gorm:"type:text"`
}

func (scopeConfig20260305) TableName() string {
	return "_tool_github_scope_configs"
}

type addPrTitlePrefixTeamMappingsToScopeConfigs struct{}

func (script *addPrTitlePrefixTeamMappingsToScopeConfigs) Up(basicRes context.BasicRes) errors.Error {
	return migrationhelper.AutoMigrateTables(
		basicRes,
		&scopeConfig20260305{},
	)
}

func (*addPrTitlePrefixTeamMappingsToScopeConfigs) Version() uint64 {
	return 20260305100000
}

func (script *addPrTitlePrefixTeamMappingsToScopeConfigs) Name() string {
	return "add pr_title_prefix_team_mappings to _tool_github_scope_configs"
}
