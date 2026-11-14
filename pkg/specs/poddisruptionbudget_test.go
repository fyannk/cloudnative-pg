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

package specs

import (
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("POD Disruption Budget specifications", func() {
	instancesNum := 3
	minAvailablePrimary := 1
	replicas := instancesNum - minAvailablePrimary
	minAvailableReplicas := replicas - 1
	cluster := &apiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "thistest",
			Namespace: "default",
		},
		Spec: apiv1.ClusterSpec{
			ImageName: "postgres:18.0",
			Instances: instancesNum,
		},
	}

	It("have the same name as the PostgreSQL cluster", func() {
		result := BuildReplicasPodDisruptionBudget(cluster)
		Expect(result.Name).To(Equal(cluster.Name))
		Expect(result.Labels).To(BeEquivalentTo(map[string]string{
			utils.ClusterLabelName:                cluster.Name,
			utils.KubernetesAppLabelName:          utils.AppName,
			utils.KubernetesAppInstanceLabelName:  cluster.Name,
			utils.KubernetesAppVersionLabelName:   "18",
			utils.KubernetesAppComponentLabelName: utils.DatabaseComponentName,
			utils.KubernetesAppManagedByLabelName: utils.ManagerName,
		}))
		Expect(result.Namespace).To(Equal(cluster.Namespace))
	})

	It("require not more than one unavailable replicas", func() {
		result := BuildReplicasPodDisruptionBudget(cluster)
		Expect(result.Spec.MinAvailable.IntVal).To(Equal(int32(minAvailableReplicas)))
	})

	It("require at least one primary instance to be available at all times", func() {
		result := BuildPrimaryPodDisruptionBudget(cluster)
		Expect(result.Spec.MinAvailable.IntVal).To(Equal(int32(minAvailablePrimary)))
	})
})

// The PodDisruptionBudgets get the metadata of their kind
var _ = Describe("The metadata of the PodDisruptionBudgets", func() {
	It("comes from podDisruptionBudgets, never from the Pods'", func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
		configuration.Current.DefaultMetadata = `[` +
			`{"targets": ["podDisruptionBudgets"], "labels": {"team": "platform"}},` +
			`{"targets": ["pgPods"], "labels": {"tier": "pods"}}` +
			`]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())

		cluster := &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "payments"},
			Spec: apiv1.ClusterSpec{
				Instances: 3,
				MetadataPropagation: []apiv1.MetadataPropagationRule{
					{
						Targets:     []apiv1.MetadataPropagationTarget{"podDisruptionBudgets"},
						Annotations: map[string]string{"example.com/note": "pdb"},
					},
					{
						Targets:     []apiv1.MetadataPropagationTarget{"pgPods"},
						Annotations: map[string]string{"vault.hashicorp.com/agent-inject": "true"},
					},
				},
			},
		}
		for _, pdb := range []*policyv1.PodDisruptionBudget{
			BuildPrimaryPodDisruptionBudget(cluster), BuildReplicasPodDisruptionBudget(cluster),
		} {
			Expect(pdb.Annotations).To(HaveKeyWithValue("example.com/note", "pdb"), pdb.Name)
			Expect(pdb.Annotations).NotTo(HaveKey("vault.hashicorp.com/agent-inject"), pdb.Name)
			Expect(pdb.Labels).To(HaveKeyWithValue("team", "platform"), pdb.Name)
			Expect(pdb.Labels).NotTo(HaveKey("tier"), pdb.Name)
			Expect(pdb.Labels).To(HaveKeyWithValue(utils.ClusterLabelName, "payments"), pdb.Name)
		}
	})
})
