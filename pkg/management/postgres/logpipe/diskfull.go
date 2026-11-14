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
	"sync/atomic"
	"time"
)

const (
	// diskFullSQLState is the SQLSTATE of the disk_full error, which
	// PostgreSQL reports for a write failing with ENOSPC, whatever the
	// language of its messages
	diskFullSQLState = "53100"

	// diskFullWindow is how long a disk full error stays related to the
	// stop of PostgreSQL. A write failing in a session is only an error for
	// that session, and PostgreSQL keeps running: such an error must not be
	// held against a stop that happens hours later for another reason. The
	// severity cannot tell them apart, as it is translated with lc_messages.
	// The errors that stop PostgreSQL (a PANIC while writing the WAL or
	// syncing a file, a recovery that cannot extend a file) are logged right
	// before the postmaster exits
	diskFullWindow = 5 * time.Minute

	// csvSQLStateField is the position of the SQLSTATE in a CSV log record
	csvSQLStateField = 12
)

// diskFullLoggedAt is when PostgreSQL last logged a disk full error, in
// nanoseconds since the epoch, or zero when it did not since the last call
// to ResetDiskFullLogged
var diskFullLoggedAt atomic.Int64

// now is the clock, replaced by the tests
var now = time.Now

// observeDiskFull records a CSV log record reporting a disk full error
func observeDiskFull(content []string) {
	if len(content) > csvSQLStateField && content[csvSQLStateField] == diskFullSQLState {
		diskFullLoggedAt.Store(now().UnixNano())
	}
}

// IsDiskFullLogged tells whether PostgreSQL logged a disk full error, with
// the disk_full SQLSTATE, in the last diskFullWindow and since the last call
// to ResetDiskFullLogged
func IsDiskFullLogged() bool {
	loggedAt := diskFullLoggedAt.Load()
	return loggedAt != 0 && now().Sub(time.Unix(0, loggedAt)) <= diskFullWindow
}

// ResetDiskFullLogged forgets the disk full errors logged until now: it is
// called when the postmaster is started
func ResetDiskFullLogged() {
	diskFullLoggedAt.Store(0)
}
