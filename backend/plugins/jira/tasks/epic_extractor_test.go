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

import "testing"

func TestIsTimestampFieldType(t *testing.T) {
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
