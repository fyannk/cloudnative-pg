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

// Package filteredcache restricts the operator cache to the objects the
// operator manages, the ones carrying the cnpg.io/reconcile=true label, and
// reads the other objects it is asked for from the API server.
package filteredcache

import "fmt"

// Mode is the value of the FILTERED_CACHE_MODE setting
type Mode string

const (
	// ModeOff caches every object of the watched kinds, as before the setting existed
	ModeOff Mode = "off"

	// ModeStrict caches only the objects carrying the cnpg.io/reconcile=true
	// label. Other objects are read from the API server when a reconciliation
	// needs them, and changes to them are not watched.
	ModeStrict Mode = "strict"

	// ModeReload is ModeStrict plus a metadata-only watch on the Secrets and
	// ConfigMaps carrying cnpg.io/reload=true, so that a change to them still
	// triggers a reconciliation.
	ModeReload Mode = "reload"
)

// ParseMode returns the mode named by a setting value, which is one of
// "off", "strict" or "reload". An empty value means off.
func ParseMode(value string) (Mode, error) {
	switch Mode(value) {
	case "", ModeOff:
		return ModeOff, nil
	case ModeStrict, ModeReload:
		return Mode(value), nil
	default:
		return "", fmt.Errorf("FILTERED_CACHE_MODE %q is not one of %q, %q or %q",
			value, ModeOff, ModeStrict, ModeReload)
	}
}

// Enabled tells whether the cache is filtered at all
func (m Mode) Enabled() bool {
	return m == ModeStrict || m == ModeReload
}
