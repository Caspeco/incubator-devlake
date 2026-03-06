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

import "regexp"

// Team-assignable PR title starts with uppercase letters, hyphen, number, then a space.
// Example: "TEAM-123 Implement feature".
var teamAssignablePrTitleRegex = regexp.MustCompile(`^([A-Z]+-\d+)\s`)

func ExtractTeamKeyFromPrTitle(title string) (string, bool) {
	matches := teamAssignablePrTitleRegex.FindStringSubmatch(title)
	if len(matches) < 2 {
		return "", false
	}
	return matches[1], true
}
