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

package utils

import (
	"strings"

	"github.com/robfig/cron"
)

// CronFormat tells how the schedule of a ScheduledBackup is read. It is the
// value of the SCHEDULED_BACKUP_CRON_FORMAT operator setting.
type CronFormat string

const (
	// CronFormatSeconds is the six-field format of robfig/cron, with the
	// seconds field first. A five-field schedule is accepted and read with
	// the seconds field first too.
	CronFormatSeconds CronFormat = "seconds"

	// CronFormatSecondsStrict is the six-field format only: a five-field
	// schedule is rejected when the ScheduledBackup is created.
	CronFormatSecondsStrict CronFormat = "seconds-strict"

	// CronFormatStandard reads a five-field schedule as standard cron, with
	// the minute field first. A six-field schedule is still read with the
	// seconds field first.
	CronFormatStandard CronFormat = "standard"
)

// IsValid tells whether the format is one of the known values
func (f CronFormat) IsValid() bool {
	switch f {
	case CronFormatSeconds, CronFormatSecondsStrict, CronFormatStandard:
		return true
	default:
		return false
	}
}

// ParseCronSchedule parses the schedule of a ScheduledBackup under the given
// format. fiveField is true when the schedule is a five-field expression,
// the only one whose reading depends on the format: a six-field expression
// and a descriptor such as "@daily" or "@every 1h" are read the same way
// under every format.
func ParseCronSchedule(spec string, format CronFormat) (schedule cron.Schedule, fiveField bool, err error) {
	if strings.HasPrefix(strings.TrimSpace(spec), "@") || len(strings.Fields(spec)) != 5 {
		schedule, err = cron.Parse(spec)
		return schedule, false, err
	}

	if format == CronFormatStandard {
		schedule, err = cron.ParseStandard(spec)
	} else {
		schedule, err = cron.Parse(spec)
	}
	return schedule, true, err
}

// SixFieldSchedule returns the six-field equivalent of a five-field
// schedule, with a zero seconds field
func SixFieldSchedule(spec string) string {
	return "0 " + strings.Join(strings.Fields(spec), " ")
}
