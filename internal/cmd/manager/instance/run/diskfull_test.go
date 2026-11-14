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

package run

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("exit for a full WAL volume", func() {
	It("writes the termination message", func(ctx SpecContext) {
		previous := terminationLogPath
		terminationLogPath = filepath.Join(GinkgoT().TempDir(), "termination-log")
		DeferCleanup(func() { terminationLogPath = previous })

		writeDiskFullTerminationMessage(ctx, "wal")

		written, err := os.ReadFile(terminationLogPath) //nolint:gosec // a file of the test
		Expect(err).ToNot(HaveOccurred())
		Expect(string(written)).To(Equal(`{"reason":"DiskFull","volumes":["wal"]}`))
	})
})
