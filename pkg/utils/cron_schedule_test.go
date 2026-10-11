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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseCronSchedule", func() {
	// 12:08:39 on a Saturday
	now := time.Date(2026, 10, 10, 12, 8, 39, 0, time.UTC)

	DescribeTable("reads a schedule according to the format",
		func(spec string, format CronFormat, fiveField bool, next time.Time) {
			schedule, isFiveField, err := ParseCronSchedule(spec, format)
			Expect(err).ToNot(HaveOccurred())
			Expect(isFiveField).To(Equal(fiveField))
			Expect(schedule.Next(now)).To(Equal(next))
		},
		// five fields: "daily at 02:00" for standard cron, "every hour at minute 2" seconds first
		Entry("five fields, seconds", "0 2 * * *", CronFormatSeconds, true,
			time.Date(2026, 10, 10, 13, 2, 0, 0, time.UTC)),
		Entry("five fields, seconds-strict", "0 2 * * *", CronFormatSecondsStrict, true,
			time.Date(2026, 10, 10, 13, 2, 0, 0, time.UTC)),
		Entry("five fields, standard", "0 2 * * *", CronFormatStandard, true,
			time.Date(2026, 10, 11, 2, 0, 0, 0, time.UTC)),
		// six fields and descriptors are read the same way under every format
		Entry("six fields, seconds", "0 0 2 * * *", CronFormatSeconds, false,
			time.Date(2026, 10, 11, 2, 0, 0, 0, time.UTC)),
		Entry("six fields, standard", "0 0 2 * * *", CronFormatStandard, false,
			time.Date(2026, 10, 11, 2, 0, 0, 0, time.UTC)),
		Entry("descriptor, seconds", "@every 1h", CronFormatSeconds, false,
			time.Date(2026, 10, 10, 13, 8, 39, 0, time.UTC)),
		Entry("descriptor, standard", "@daily", CronFormatStandard, false,
			time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)),
	)

	DescribeTable("rejects what no format can read",
		func(spec string, format CronFormat) {
			_, _, err := ParseCronSchedule(spec, format)
			Expect(err).To(HaveOccurred())
		},
		Entry("four fields", "0 2 * *", CronFormatSeconds),
		Entry("seven fields", "0 0 2 * * * 1996", CronFormatStandard),
		Entry("a value out of range as standard cron", "0 25 * * *", CronFormatStandard),
		Entry("an unknown descriptor", "@fortnightly", CronFormatSeconds),
	)

	It("gives the six-field equivalent of a five-field schedule", func() {
		Expect(SixFieldSchedule("0 2  * * *")).To(Equal("0 0 2 * * *"))
	})

	It("knows its values", func() {
		Expect(CronFormatSeconds.IsValid()).To(BeTrue())
		Expect(CronFormatSecondsStrict.IsValid()).To(BeTrue())
		Expect(CronFormatStandard.IsValid()).To(BeTrue())
		Expect(CronFormat("strict").IsValid()).To(BeFalse())
	})
})
