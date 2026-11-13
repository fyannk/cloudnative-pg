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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("disk full errors in the PostgreSQL logs", func() {
	const (
		checkpoint = `2026-11-13 10:00:00.000 UTC,,,9298,,601c20b5.2452,1,,2026-11-13 09:00:00 UTC,,0,` +
			`LOG,00000,"checkpoint starting: time",,,,,,,,,"",checkpointer` + "\n"
		// the error of a recovery failing to extend a relation of a full
		// tablespace, translated: only the SQLSTATE tells it
		translated = `2026-11-13 10:00:01.000 UTC,,,9300,,601c20b5.2454,1,,2026-11-13 09:00:00 UTC,1/0,0,` +
			`FATAL,53100,"n'a pas pu étendre le fichier « pg_tblspc/16385/PG_17/5/16386 » : ` +
			`Aucun espace disponible sur le périphérique",,,,,,,,,"",startup` + "\n"
		// the same in English, as reported with another SQLSTATE
		english = `2026-11-13 10:00:01.000 UTC,,,9300,,601c20b5.2454,1,,2026-11-13 09:00:00 UTC,1/0,0,` +
			`PANIC,58030,"could not write to file ""pg_wal/xlogtemp.9300"": No space left on device",` +
			`,,,,,,,,"",startup` + "\n"
	)

	read := func(ctx SpecContext, lines string) {
		p := LogPipe{record: &LoggingRecord{}, fieldsValidator: LogFieldValidator}
		Expect(p.streamLogFromCSVFile(ctx, strings.NewReader(lines), &SpyRecordWriter{})).To(Succeed())
	}

	BeforeEach(func() {
		ResetDiskFullLogged()
		DeferCleanup(ResetDiskFullLogged)
	})

	It("is not reported for the other records", func(ctx SpecContext) {
		read(ctx, checkpoint+checkpoint)
		Expect(IsDiskFullLogged()).To(BeFalse())
	})

	It("is reported by the disk_full SQLSTATE, in any language", func(ctx SpecContext) {
		read(ctx, checkpoint+translated)
		Expect(IsDiskFullLogged()).To(BeTrue())

		ResetDiskFullLogged()
		Expect(IsDiskFullLogged()).To(BeFalse())
	})

	It("is reported by the text of ENOSPC, in the first record too", func(ctx SpecContext) {
		read(ctx, english)
		Expect(IsDiskFullLogged()).To(BeTrue())
	})
})
