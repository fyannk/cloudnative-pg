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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres/volumeusage"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("exit for a full volume", func() {
	var paths volumeusage.Paths

	BeforeEach(func() {
		root := GinkgoT().TempDir()
		paths = volumeusage.Paths{
			PgData:      filepath.Join(root, "data"),
			WalVolume:   filepath.Join(root, "wal"),
			Tablespaces: filepath.Join(root, "tablespaces"),
		}
		Expect(os.Mkdir(paths.PgData, 0o700)).To(Succeed())
		Expect(os.Mkdir(paths.WalVolume, 0o700)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(paths.Tablespaces, "alpha"), 0o700)).To(Succeed())
	})

	// statfs reports the tablespace volume full, the others half used
	statfs := func(path string, stat *syscall.Statfs_t) error {
		*stat = syscall.Statfs_t{Bsize: 4096, Blocks: 262144, Bfree: 131072, Bavail: 131072}
		if filepath.Base(path) == "alpha" {
			stat.Bfree, stat.Bavail = 10, 0
		}
		return nil
	}

	It("names the full volumes once PostgreSQL logged a disk full error", func(ctx SpecContext) {
		Expect(getFullVolumes(ctx, false, paths, statfs)).To(BeEmpty())
		Expect(getFullVolumes(ctx, true, paths, statfs)).To(Equal([]string{"tbs-alpha"}))
	})

	It("names no volume when none is full", func(ctx SpecContext) {
		Expect(getFullVolumes(ctx, true, paths, func(_ string, stat *syscall.Statfs_t) error {
			*stat = syscall.Statfs_t{Bsize: 4096, Blocks: 262144, Bfree: 131072, Bavail: 131072}
			return nil
		})).To(BeEmpty())
	})

	It("exits for the full volumes, or for the one holding the WAL files", func() {
		postmaster := &exec.ExitError{}
		volumes, ok := getMissingDiskSpaceVolumes(makeUnretryableError(
			&errDiskFull{volumes: []string{"tbs-alpha"}, err: postmaster}))
		Expect(ok).To(BeTrue())
		Expect(volumes).To(Equal([]string{"tbs-alpha"}))

		volumes, ok = getMissingDiskSpaceVolumes(makeUnretryableError(
			fmt.Errorf("%w: %w", postgres.ErrNoFreeWALSpace, postmaster)))
		Expect(ok).To(BeTrue())
		Expect(volumes).To(HaveLen(1))

		_, ok = getMissingDiskSpaceVolumes(makeUnretryableError(errors.New("other")))
		Expect(ok).To(BeFalse())
	})

	It("writes the termination message", func(ctx SpecContext) {
		previous := terminationLogPath
		terminationLogPath = filepath.Join(GinkgoT().TempDir(), "termination-log")
		DeferCleanup(func() { terminationLogPath = previous })

		writeDiskFullTerminationMessage(ctx, []string{"tbs-alpha"})

		written, err := os.ReadFile(terminationLogPath) //nolint:gosec // a file of the test
		Expect(err).ToNot(HaveOccurred())
		Expect(string(written)).To(Equal(`{"reason":"DiskFull","volumes":["tbs-alpha"]}`))
	})
})
