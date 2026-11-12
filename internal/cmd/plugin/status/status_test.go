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

package status

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("getPrimaryPromotionTime", func() {
	var cluster *apiv1.Cluster

	Context("when CurrentPrimaryTimestamp is empty", func() {
		BeforeEach(func() {
			cluster = &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					CurrentPrimaryTimestamp: "",
				},
			}
		})

		It("should return an empty string", func() {
			Expect(getPrimaryPromotionTime(cluster)).To(Equal(""))
		})
	})

	Context("when CurrentPrimaryTimestamp is valid", func() {
		It("should return the formatted timestamp", func() {
			now := time.Now()
			uptime := 1 * time.Hour
			currentPrimaryTimestamp := now.Add(-uptime)

			cluster = &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					CurrentPrimaryTimestamp: currentPrimaryTimestamp.Format(metav1.RFC3339Micro),
				},
			}

			expected := fmt.Sprintf("%s (%s)", currentPrimaryTimestamp.Round(time.Second), uptime)
			Expect(getPrimaryPromotionTimeIdempotent(cluster, now)).To(Equal(expected))
		})
	})

	Context("when CurrentPrimaryTimestamp is invalid", func() {
		BeforeEach(func() {
			cluster = &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					CurrentPrimaryTimestamp: "invalid timestamp",
				},
			}
		})

		It("should return the error message", func() {
			Expect(getPrimaryPromotionTime(cluster)).To(ContainSubstring("error"))
		})
	})
})

var _ = Describe("operatorSummary", func() {
	DescribeTable("formats what the status says about the operator",
		func(operator *apiv1.OperatorStatus, expected string) {
			Expect(operatorSummary(operator)).To(Equal(expected))
		},
		Entry("nothing reported", nil, ""),
		Entry("empty block", &apiv1.OperatorStatus{}, ""),
		Entry("version alone", &apiv1.OperatorStatus{Version: "1.30.1"}, "1.30.1"),
		Entry("version and Deployment name",
			&apiv1.OperatorStatus{Version: "1.30.1", Name: "cnpg-toto"}, "cnpg-toto (1.30.1)"),
		Entry("version, Deployment name and namespace",
			&apiv1.OperatorStatus{Version: "1.30.1", Name: "cnpg-toto", Namespace: "operators"},
			"operators/cnpg-toto (1.30.1)"),
		Entry("name without version", &apiv1.OperatorStatus{Name: "cnpg-toto"}, "cnpg-toto"),
		Entry("namespace without name is not shown",
			&apiv1.OperatorStatus{Version: "1.30.1", Namespace: "operators"}, "1.30.1"),
	)
})

var _ = Describe("getWalArchivingStatus", func() {
	It("should return 'Disabled' when WAL archiving is disabled", func() {
		result := getWalArchivingStatus(false, "", true)
		Expect(result).To(ContainSubstring("Disabled"))
	})

	It("should return 'OK' when archiving is working", func() {
		result := getWalArchivingStatus(true, "", false)
		Expect(result).To(ContainSubstring("OK"))
	})

	It("should return 'Failing' when there is a failed WAL", func() {
		result := getWalArchivingStatus(false, "000000010000000000000001", false)
		Expect(result).To(ContainSubstring("Failing"))
	})

	It("should return 'OK' even when there is a failed WAL if archiving is working", func() {
		result := getWalArchivingStatus(true, "000000010000000000000001", false)
		Expect(result).To(ContainSubstring("OK"))
	})

	It("should return 'Starting Up' when archiving hasn't started yet", func() {
		result := getWalArchivingStatus(false, "", false)
		Expect(result).To(ContainSubstring("Starting Up"))
	})

	It("should prioritize 'Disabled' over other statuses", func() {
		// Even if archiving is working, disabled should take precedence
		result := getWalArchivingStatus(true, "", true)
		Expect(result).To(ContainSubstring("Disabled"))
	})
})
