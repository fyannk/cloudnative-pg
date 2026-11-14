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

package e2e

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	"github.com/cloudnative-pg/cloudnative-pg/tests/internal/resources"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The Cluster CRD carries the rules comparing an update with the previous
// state, and the API server enforces them whatever the webhook does, see
// "Defaulting and validation without admission webhooks" in
// docs/src/operator_conf.md.
var _ = Describe("Cluster CRD update rules", Label(tests.LabelFork, tests.LabelOperator), func() {
	const (
		clusterFile     = forkFixturesDir + "/cluster-crd-rules.yaml.template"
		namespacePrefix = "fork-crd-rules"
		level           = tests.Medium
	)

	BeforeEach(func() {
		if testLevelEnv.Depth < int(level) {
			Skip("Test depth is lower than the amount requested for this test")
		}
	})

	It("enforces the rules with the validating webhook bypassed, but for a storage size decrease", func() {
		namespace, err := env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
		Expect(err).ToNot(HaveOccurred())
		clusterName, err := yaml.GetResourceNameFromYAML(env.Scheme, clusterFile)
		Expect(err).ToNot(HaveOccurred())

		// The rules are enforced by the API server on the stored object: the
		// instances are not needed, and the namespace is deleted while the
		// cluster is still being created.
		resources.CreateResourceFromFile(env, namespace, clusterFile)

		cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
		Expect(err).ToNot(HaveOccurred())
		Expect(cluster.Annotations).To(HaveKeyWithValue(utils.WebhookValidationAnnotationName, "disabled"))
		Expect(cluster.Spec.WalStorage).ToNot(BeNil())

		// patch applies a JSON patch to the Cluster. An update refused by the
		// API server is never retried, while a failure of another kind is.
		patch := func(jsonPatch string) error {
			var err error
			Eventually(func() bool {
				cluster := &apiv1.Cluster{}
				cluster.Namespace = namespace
				cluster.Name = clusterName
				err = env.Client.Patch(env.Ctx, cluster,
					ctrlclient.RawPatch(types.JSONPatchType, []byte(jsonPatch)))
				return err == nil || apierrors.IsInvalid(err)
			}, RetryTimeout, PollingTime).Should(BeTrue(), func() string { return err.Error() })
			return err
		}

		By("refusing the removal of walStorage", func() {
			err := patch(`[{"op": "remove", "path": "/spec/walStorage"}]`)
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "%v", err)
			Expect(err.Error()).To(ContainSubstring("walStorage cannot be disabled once configured"))
			// the answer comes from the API server, not from a webhook
			Expect(err.Error()).ToNot(ContainSubstring("admission webhook"))
		})

		By("refusing a change of postgresUID", func() {
			err := patch(`[{"op": "add", "path": "/spec/postgresUID", "value": 1001}]`)
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "%v", err)
			Expect(err.Error()).To(ContainSubstring("postgresUID is immutable"))
			Expect(err.Error()).ToNot(ContainSubstring("admission webhook"))
		})

		By("refusing a change of postgresGID", func() {
			err := patch(`[{"op": "add", "path": "/spec/postgresGID", "value": 1001}]`)
			Expect(apierrors.IsInvalid(err)).To(BeTrue(), "%v", err)
			Expect(err.Error()).To(ContainSubstring("postgresGID is immutable"))
		})

		By("keeping the Cluster as it was", func() {
			cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())
			Expect(cluster.Spec.WalStorage).ToNot(BeNil())
			Expect(cluster.Spec.PostgresUID).To(BeEquivalentTo(26))
			Expect(cluster.Spec.PostgresGID).To(BeEquivalentTo(26))
		})

		By("accepting a storage size decrease, which is left to the webhook", func() {
			Expect(patch(`[{"op": "replace", "path": "/spec/storage/size", "value": "1Gi"}]`)).To(Succeed())

			cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())
			Expect(cluster.Spec.StorageConfiguration.Size).To(Equal("1Gi"))
		})
	})
})
