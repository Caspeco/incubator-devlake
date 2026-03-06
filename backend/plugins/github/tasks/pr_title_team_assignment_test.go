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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractTeamKeyFromPrTitle(t *testing.T) {
	t.Run("matches expected pattern and captures team key", func(t *testing.T) {
		teamKey, ok := ExtractTeamKeyFromPrTitle("TEAM-123 Improve deployment flow")
		assert.True(t, ok)
		assert.Equal(t, "TEAM-123", teamKey)
	})

	t.Run("does not match when lowercase prefix is used", func(t *testing.T) {
		teamKey, ok := ExtractTeamKeyFromPrTitle("team-123 Improve deployment flow")
		assert.False(t, ok)
		assert.Equal(t, "", teamKey)
	})

	t.Run("does not match when space after number is missing", func(t *testing.T) {
		teamKey, ok := ExtractTeamKeyFromPrTitle("TEAM-123Improve deployment flow")
		assert.False(t, ok)
		assert.Equal(t, "", teamKey)
	})
}
