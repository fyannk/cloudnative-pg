/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package pgbouncer

import (
	"encoding/json"
	"strings"
)

// rulesYAML writes the default or mandatory metadata of the operator from
// short `key=value@target+target` entries
func rulesYAML(labels, annotations []string) string {
	var rules []map[string]any
	for _, kind := range []struct {
		field   string
		entries []string
	}{{"labels", labels}, {"annotations", annotations}} {
		for _, entry := range kind.entries {
			separator := strings.LastIndex(entry, "@")
			key, value, _ := strings.Cut(entry[:separator], "=")
			rules = append(rules, map[string]any{
				"targets":  strings.Split(entry[separator+1:], "+"),
				kind.field: map[string]string{key: value},
			})
		}
	}
	if len(rules) == 0 {
		return ""
	}
	result, _ := json.Marshal(rules)
	return string(result)
}
