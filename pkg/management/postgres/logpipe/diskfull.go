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

package logpipe

import (
	"strings"
	"sync/atomic"
)

const (
	// diskFullSQLState is the SQLSTATE of the disk_full error, which
	// PostgreSQL reports for a write failing with ENOSPC
	diskFullSQLState = "53100"

	// noSpaceLeftOnDevice is the text of ENOSPC, in the messages of a
	// PostgreSQL using the English messages
	noSpaceLeftOnDevice = "No space left on device"

	// csvSQLStateField and csvMessageField are the positions of the
	// SQLSTATE and of the message in a CSV log record
	csvSQLStateField = 12
	csvMessageField  = 13
)

// diskFullLogged is set when PostgreSQL logged a disk full error since the
// last call to ResetDiskFullLogged
var diskFullLogged atomic.Bool

// observeDiskFull records a CSV log record reporting a disk full error
func observeDiskFull(content []string) {
	if len(content) > csvSQLStateField && content[csvSQLStateField] == diskFullSQLState {
		diskFullLogged.Store(true)
		return
	}
	if len(content) > csvMessageField && strings.Contains(content[csvMessageField], noSpaceLeftOnDevice) {
		diskFullLogged.Store(true)
	}
}

// IsDiskFullLogged tells whether PostgreSQL logged a disk full error, with
// the disk_full SQLSTATE or "No space left on device", since the last call to
// ResetDiskFullLogged
func IsDiskFullLogged() bool {
	return diskFullLogged.Load()
}

// ResetDiskFullLogged forgets the disk full errors logged until now: it is
// called when the postmaster is started
func ResetDiskFullLogged() {
	diskFullLogged.Store(false)
}
