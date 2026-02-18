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
	"time"

	"github.com/apache/incubator-devlake/plugins/jira/models"
)

func TestIsConfiguredTimestampField(t *testing.T) {
	// Verifies configured incident fields are accepted only when present in the timestamp field map.
	fieldMap := map[string]struct{}{
		"customfield_10001": {},
	}

	if !isConfiguredTimestampField("customfield_10001", fieldMap) {
		t.Fatal("expected configured field to be accepted")
	}
	if isConfiguredTimestampField("customfield_10002", fieldMap) {
		t.Fatal("expected unknown field to be rejected")
	}
	if isConfiguredTimestampField("customfield_10001", map[string]struct{}{}) {
		t.Fatal("expected empty field map to reject all fields")
	}
}

func TestIsTimestampFieldType(t *testing.T) {
	// Verifies only date/datetime Jira schema types are treated as timestamp fields.
	tests := []struct {
		name       string
		schemaType string
		want       bool
	}{
		{name: "date", schemaType: "date", want: true},
		{name: "datetime", schemaType: "datetime", want: true},
		{name: "upper datetime", schemaType: "DateTime", want: true},
		{name: "string", schemaType: "string", want: false},
		{name: "number", schemaType: "number", want: false},
		{name: "empty", schemaType: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTimestampFieldType(tt.schemaType); got != tt.want {
				t.Fatalf("isTimestampFieldType(%q) = %v, want %v", tt.schemaType, got, tt.want)
			}
		})
	}
}

func TestResolveIncidentDuration_CustomFields(t *testing.T) {
	// Verifies custom incident start/stop fields override default created/resolution timestamps.
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	defaultStop := time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC)

	scopeConfig := &models.JiraScopeConfig{
		IncidentStartField: "customfield_start",
		IncidentStopField:  "customfield_stop",
	}
	allFields := map[string]interface{}{
		"customfield_start": "2025-01-01T01:00:00Z",
		"customfield_stop":  "2025-01-01T02:30:00Z",
	}
	timestampFieldMap := map[string]struct{}{
		"customfield_start": {},
		"customfield_stop":  {},
	}

	start, stop := resolveIncidentDuration(created, &defaultStop, allFields, scopeConfig, timestampFieldMap)
	wantStart := time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)
	wantStop := time.Date(2025, 1, 1, 2, 30, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Fatalf("start = %s, want %s", start, wantStart)
	}
	if stop == nil || !stop.Equal(wantStop) {
		t.Fatalf("stop = %v, want %s", stop, wantStop)
	}
}

func TestResolveIncidentDuration_NonTimestampConfiguredFieldsFallback(t *testing.T) {
	// Verifies non-timestamp configured fields are ignored and defaults are kept.
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	defaultStop := time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC)

	scopeConfig := &models.JiraScopeConfig{
		IncidentStartField: "customfield_start",
		IncidentStopField:  "customfield_stop",
	}
	allFields := map[string]interface{}{
		"customfield_start": "2025-01-01T01:00:00Z",
		"customfield_stop":  "2025-01-01T02:30:00Z",
	}
	timestampFieldMap := map[string]struct{}{}

	start, stop := resolveIncidentDuration(created, &defaultStop, allFields, scopeConfig, timestampFieldMap)
	if !start.Equal(created) {
		t.Fatalf("start = %s, want fallback %s", start, created)
	}
	if stop == nil || !stop.Equal(defaultStop) {
		t.Fatalf("stop = %v, want fallback %s", stop, defaultStop)
	}
}

func TestCalculateLeadTimeMinutes(t *testing.T) {
	// Verifies lead time is computed only for valid stop timestamps (non-nil and not before start).
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("normal", func(t *testing.T) {
		stop := time.Date(2025, 1, 1, 2, 15, 0, 0, time.UTC)
		got := calculateLeadTimeMinutes(start, &stop)
		if got == nil || *got != 135 {
			t.Fatalf("lead time = %v, want 135", got)
		}
	})

	t.Run("nil stop", func(t *testing.T) {
		got := calculateLeadTimeMinutes(start, nil)
		if got != nil {
			t.Fatalf("lead time = %v, want nil", *got)
		}
	})

	t.Run("stop before start", func(t *testing.T) {
		stop := time.Date(2024, 12, 31, 23, 59, 0, 0, time.UTC)
		got := calculateLeadTimeMinutes(start, &stop)
		if got != nil {
			t.Fatalf("lead time = %v, want nil", *got)
		}
	})
}

func TestShouldOverrideIncidentTimestamps(t *testing.T) {
	// Verifies timestamp override applies only to issue types mapped to INCIDENT.
	mappings := &typeMappings{
		TypeIdMappings: map[string]string{
			"10001": "Incident",
			"10002": "Bug",
		},
		StdTypeMappings: map[string]string{
			"Incident": "INCIDENT",
			"Bug":      "BUG",
		},
	}

	if !shouldOverrideIncidentTimestamps("10001", mappings) {
		t.Fatal("expected incident type to enable timestamp override")
	}
	if shouldOverrideIncidentTimestamps("10002", mappings) {
		t.Fatal("expected non-incident type to disable timestamp override")
	}
	if shouldOverrideIncidentTimestamps("10001", nil) {
		t.Fatal("expected nil mappings to disable timestamp override")
	}
}
