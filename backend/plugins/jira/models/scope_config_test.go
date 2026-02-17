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

package models

import "testing"

func TestJiraScopeConfig_TableName(t *testing.T) {
	var c JiraScopeConfig
	if got := c.TableName(); got != "_tool_jira_scope_configs" {
		t.Fatalf("TableName() = %s, want %s", got, "_tool_jira_scope_configs")
	}
}

func TestJiraScopeConfig_ValidateWithIncidentFields(t *testing.T) {
	c := &JiraScopeConfig{
		IncidentStartField: "customfield_10001",
		IncidentStopField:  "customfield_10002",
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestJiraScopeConfig_ValidateInvalidRemotelinkPattern(t *testing.T) {
	c := &JiraScopeConfig{
		RemotelinkCommitShaPattern: "(",
		IncidentStartField:         "customfield_10001",
		IncidentStopField:          "customfield_10002",
	}
	if err := c.Validate(); err == nil {
		t.Fatal("Validate() expected error, got nil")
	}
}
