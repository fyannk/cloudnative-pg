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

package metricsserver

import (
	"database/sql"
	"errors"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres/pool"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// unreachablePooler stands for a PgBouncer whose admin console cannot be reached
type unreachablePooler struct{}

func (unreachablePooler) Connection(_ string) (*sql.DB, error) {
	return nil, errors.New("connection refused")
}

func (unreachablePooler) GetDsn(dbName string) string {
	return dbName
}

func (unreachablePooler) ShutdownConnections() {
}

// Goal: a scrape of the Exporter registered as a whole, as Setup does, must
// expose cnpg_pgbouncer_up. The per-collector tests register the gauge by
// hand on their registry, which hid its absence from Describe and Collect.
var _ = Describe("Exporter scrape", func() {
	// scrape registers a fresh Exporter on a fresh registry and gathers it once,
	// returning the up and last_collection_error gauges
	scrape := func(ctx SpecContext, pooler pool.Pooler) (up, lastError float64) {
		exp := &Exporter{
			ctx:     ctx,
			Metrics: newMetrics(),
			pool:    pooler,
		}
		registry := prometheus.NewRegistry()
		registry.MustRegister(exp)

		metrics, err := registry.Gather()
		Expect(err).ToNot(HaveOccurred())

		upFamily := getMetric(metrics, pgBouncerUpKey)
		Expect(upFamily).ToNot(BeNil(), "cnpg_pgbouncer_up is missing from the scrape")
		errorFamily := getMetric(metrics, lastCollectionErrorKey)
		Expect(errorFamily).ToNot(BeNil())

		return upFamily.GetMetric()[0].GetGauge().GetValue(),
			errorFamily.GetMetric()[0].GetGauge().GetValue()
	}

	It("exposes cnpg_pgbouncer_up set to 1 when the admin console answers", func(ctx SpecContext) {
		db, mock, err := sqlmock.New()
		Expect(err).ShouldNot(HaveOccurred())

		// Step 1: the three admin console queries succeed
		mock.ExpectQuery("SHOW LISTS;").
			WillReturnRows(sqlmock.NewRows([]string{"list", "items"}).AddRow("databases", 1))
		mock.ExpectQuery("SHOW POOLS;").
			WillReturnRows(sqlmock.NewRows([]string{
				"database", "user", "cl_active", "cl_waiting", "cl_active_cancel_req",
				"cl_waiting_cancel_req", "sv_active", "sv_active_cancel", "sv_being_canceled",
				"sv_idle", "sv_used", "sv_tested", "sv_login", "maxwait", "maxwait_us", "pool_mode",
			}).AddRow("db1", "user1", 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, "session"))
		mock.ExpectQuery("SHOW STATS;").WillReturnRows(getShowStatsRows())

		// Step 2: one scrape through the registry
		up, lastError := scrape(ctx, fakePooler{db: db})
		Expect(up).To(BeEquivalentTo(1))
		Expect(lastError).To(BeEquivalentTo(0))
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	It("exposes cnpg_pgbouncer_up set to 0 when the admin console cannot be reached", func(ctx SpecContext) {
		// No connection, so no query reaches PgBouncer: the gauge must still
		// be exposed, and say that PgBouncer is down
		up, lastError := scrape(ctx, unreachablePooler{})
		Expect(up).To(BeEquivalentTo(0))
		Expect(lastError).To(BeEquivalentTo(1))
	})
})
