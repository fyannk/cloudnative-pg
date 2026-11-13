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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Termination message", func() {
	It("names the full volumes", func() {
		message := NewDiskFullTerminationMessage(TablespaceVolumePrefix+"alpha", PgDataVolumeName).String()
		Expect(message).To(Equal(`{"reason":"DiskFull","volumes":["tbs-alpha","pgdata"]}`))
		Expect(GetDiskFullVolumes(message)).To(Equal([]string{"tbs-alpha", "pgdata"}))
	})

	DescribeTable("names no volume for a message that is not a disk full one",
		func(message string) {
			Expect(GetDiskFullVolumes(message)).To(BeNil())
		},
		Entry("empty, as written by an older instance manager", ""),
		Entry("not JSON", "panic: runtime error"),
		Entry("another reason", `{"reason":"Other","volumes":["wal"]}`),
		Entry("no volume", `{"reason":"DiskFull"}`),
	)
})
