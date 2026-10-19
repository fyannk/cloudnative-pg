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

package controller

import (
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Operator identity in the Cluster status", func() {
	instance := func(mode string) *configuration.Data {
		return &configuration.Data{
			OperatorNamespace:      "awesome-operators",
			OperatorDeploymentName: "cnpg-toto",
			OperatorStatusIdentity: mode,
		}
	}

	DescribeTable("follows the configured mode",
		func(config *configuration.Data, expected *apiv1.OperatorStatus) {
			Expect(operatorStatus(config)).To(Equal(expected))
		},
		Entry("off writes nothing, the field stays absent",
			instance(configuration.OperatorStatusIdentityOff), nil),
		Entry("version writes the version only",
			instance(configuration.OperatorStatusIdentityVersion),
			&apiv1.OperatorStatus{Version: versions.Version}),
		Entry("name adds the Deployment name",
			instance(configuration.OperatorStatusIdentityName),
			&apiv1.OperatorStatus{Version: versions.Version, Name: "cnpg-toto"}),
		Entry("full adds the namespace",
			instance(configuration.OperatorStatusIdentityFull),
			&apiv1.OperatorStatus{Version: versions.Version, Name: "cnpg-toto", Namespace: "awesome-operators"}),
		Entry("an unknown mode writes nothing",
			instance("verbose"), nil),
	)

	It("writes the version only on a stock installation", func() {
		stock := configuration.NewFromData(map[string]string{"OPERATOR_NAMESPACE": "cnpg-system"})
		Expect(operatorStatus(stock)).To(Equal(&apiv1.OperatorStatus{Version: versions.Version}))
	})

	It("leaves the name out when the Deployment name is not known", func() {
		config := instance(configuration.OperatorStatusIdentityFull)
		config.OperatorDeploymentName = ""
		Expect(operatorStatus(config)).To(Equal(
			&apiv1.OperatorStatus{Version: versions.Version, Namespace: "awesome-operators"}))
	})
})
