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

package postgres

import (
	"encoding/json"
	"strings"
)

// TerminationReasonDiskFull is the reason of the termination message of an
// instance manager exiting because a volume is full
const TerminationReasonDiskFull = "DiskFull"

// TerminationMessage is the message the instance manager writes to the
// termination log of the PostgreSQL container when it exits with the
// MissingWALDiskSpaceExitCode exit code, for the kubelet to report it in the
// status of the container, such as:
//
//	{"reason":"DiskFull","volumes":["tbs-alpha"]}
type TerminationMessage struct {
	// Reason is TerminationReasonDiskFull
	Reason string `json:"reason"`

	// Volumes are the full volumes, named as in VolumeUsage
	Volumes []string `json:"volumes,omitempty"`
}

// NewDiskFullTerminationMessage returns the termination message naming the
// full volumes
func NewDiskFullTerminationMessage(volumes ...string) TerminationMessage {
	return TerminationMessage{Reason: TerminationReasonDiskFull, Volumes: volumes}
}

// String returns the message as written to the termination log
func (message TerminationMessage) String() string {
	data, err := json.Marshal(message)
	if err != nil {
		return ""
	}
	return string(data)
}

// GetDiskFullVolumes returns the volumes named full by the termination
// message of the PostgreSQL container. It returns nil when the message is
// empty, is not a disk full message or names no volume, as written by an
// instance manager knowing no message: the full volume is then the one
// holding the WAL files.
func GetDiskFullVolumes(message string) []string {
	var parsed TerminationMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(message)), &parsed); err != nil {
		return nil
	}
	if parsed.Reason != TerminationReasonDiskFull {
		return nil
	}
	return parsed.Volumes
}
