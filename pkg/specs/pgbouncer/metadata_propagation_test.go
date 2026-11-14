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

package pgbouncer

import (
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	config "github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The metadataPropagation rules of a Pooler put labels and annotations on
// each kind of its objects, on top of its templates. The Deployment hash
// follows the metadata of the Pods and of the Deployment only when a rule
// reaches them: no rule keeps the hash of the previous versions, a rule on
// the Deployment updates it without rolling the Pods out, a rule on the Pods
// rolls them out once
var _ = Describe("The metadataPropagation rules of a Pooler", func() {
	var (
		pooler  *apiv1.Pooler
		cluster *apiv1.Cluster
	)

	rule := func(labels, annotations map[string]string, targets ...apiv1.MetadataPropagationTarget,
	) apiv1.MetadataPropagationRule {
		return apiv1.MetadataPropagationRule{Targets: targets, Labels: labels, Annotations: annotations}
	}
	deployment := func() *appsv1.Deployment {
		result, err := Deployment(pooler, cluster)
		Expect(err).ToNot(HaveOccurred())
		return result
	}
	hashOf := func(deployment *appsv1.Deployment) string {
		return deployment.Annotations[utils.PoolerSpecHashAnnotationName]
	}

	BeforeEach(func() {
		previous := config.Current
		config.Current = config.NewConfiguration()
		DeferCleanup(func() { config.Current = previous })

		pooler = &apiv1.Pooler{
			TypeMeta:   metav1.TypeMeta{Kind: apiv1.PoolerKind, APIVersion: apiv1.SchemeGroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{Name: "pooler-payments", Namespace: "payments"},
			Spec: apiv1.PoolerSpec{
				Cluster:   apiv1.LocalObjectReference{Name: "payments"},
				Type:      apiv1.PoolerTypeRW,
				Instances: ptr.To(int32(1)),
				Template: &apiv1.PodTemplateSpec{ObjectMeta: apiv1.Metadata{
					Labels: map[string]string{"team": "payments", "tier": "template"},
				}},
				ServiceTemplate: &apiv1.ServiceTemplateSpec{ObjectMeta: apiv1.Metadata{
					Annotations: map[string]string{"example.com/owner": "team-a"},
				}},
				PgBouncer: &apiv1.PgBouncerSpec{PoolMode: apiv1.PgBouncerPoolModeSession},
			},
			Status: apiv1.PoolerStatus{Image: config.Current.PgbouncerImageName, Secrets: &apiv1.PoolerSecrets{}},
		}
		cluster = &apiv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "payments"}}
	})

	It("change nothing without a rule: the hash and the objects of the previous versions", func() {
		result := deployment()
		Expect(hashOf(result)).To(Equal(mustComputeTemplateHash(pooler)))
		Expect(result.Spec.Template.Labels).To(HaveKeyWithValue("tier", "template"))
		Expect(ServiceAccount(pooler).Labels).To(BeEmpty())
		Expect(Role(pooler).Annotations).To(BeEmpty())
	})

	It("put each rule on its kind of object only", func() {
		pooler.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
			rule(map[string]string{"pods": "yes"}, nil, "poolerPods"),
			rule(map[string]string{"deployment": "yes"}, nil, "poolerDeployments"),
			rule(nil, map[string]string{"service": "yes"}, "services"),
			rule(map[string]string{"everywhere": "yes"}, nil, "all"),
		}
		result := deployment()
		Expect(result.Spec.Template.Labels).To(SatisfyAll(
			HaveKeyWithValue("pods", "yes"), HaveKeyWithValue("everywhere", "yes"), Not(HaveKey("deployment"))))
		Expect(result.Labels).To(SatisfyAll(
			HaveKeyWithValue("deployment", "yes"), HaveKeyWithValue("everywhere", "yes"), Not(HaveKey("pods"))))
		Expect(result.Spec.Selector.MatchLabels).To(Equal(map[string]string{utils.PgbouncerNameLabel: "pooler-payments"}))

		service, err := Service(pooler, cluster)
		Expect(err).ToNot(HaveOccurred())
		Expect(service.Annotations).To(HaveKeyWithValue("service", "yes"))
		Expect(service.Labels).To(SatisfyAll(HaveKeyWithValue("everywhere", "yes"), Not(HaveKey("pods"))))

		for _, meta := range []metav1.ObjectMeta{
			ServiceAccount(pooler).ObjectMeta, Role(pooler).ObjectMeta,
			RoleBinding(pooler, "pooler-payments").ObjectMeta,
			NewPoolerPodMonitorManager(pooler, cluster).BuildPodMonitor().ObjectMeta,
		} {
			Expect(meta.Labels).To(SatisfyAll(HaveKeyWithValue("everywhere", "yes"),
				Not(HaveKey("pods")), Not(HaveKey("deployment"))), meta.Name)
		}
	})

	It("win over the templates, below the mandatory metadata of the operator", func() {
		config.Current.MandatoryMetadata = `[{"targets": ["poolerPods"], "labels": {"team": "platform"}}]`
		config.Current.DefaultMetadata = `[{"targets": ["poolerPods"], "labels": {"mesh": "default", "tier": "default"}}]`
		Expect(config.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		pooler.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
			rule(map[string]string{"tier": "rule", "team": "rule"}, nil, "poolerPods"),
		}
		Expect(deployment().Spec.Template.Labels).To(SatisfyAll(
			HaveKeyWithValue("tier", "rule"),     // the rule over the template and the default
			HaveKeyWithValue("team", "platform"), // the mandatory value over the rule
			HaveKeyWithValue("mesh", "default"),  // a default nothing replaces
		))
	})

	It("never take the keys the operator sets itself", func() {
		pooler.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
			rule(map[string]string{utils.PgbouncerNameLabel: "fake", utils.PodRoleLabelName: "fake"}, nil, "all"),
		}
		result := deployment()
		Expect(result.Spec.Template.Labels).To(HaveKeyWithValue(utils.PgbouncerNameLabel, "pooler-payments"))
		Expect(result.Spec.Template.Labels).To(HaveKeyWithValue(utils.PodRoleLabelName, string(utils.PodRolePooler)))
	})

	Context("and the Deployment hash", func() {
		It("keeps the stock hash when the rules reach neither the Pods nor the Deployment", func() {
			pooler.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
				rule(nil, map[string]string{"service": "yes"}, "services", "secrets"),
			}
			Expect(hashOf(deployment())).To(Equal(mustComputeTemplateHash(pooler)))
		})

		It("changes it, without touching the Pods, for a rule on the Deployment", func() {
			before := deployment()
			pooler.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
				rule(map[string]string{"deployment": "yes"}, nil, "poolerDeployments"),
			}
			after := deployment()
			Expect(hashOf(after)).NotTo(Equal(hashOf(before)))
			Expect(after.Spec.Template).To(Equal(before.Spec.Template))
			Expect(after.Labels).To(HaveKeyWithValue("deployment", "yes"))
		})

		It("changes it with the Pods for a rule on the Pods", func() {
			before := deployment()
			pooler.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
				rule(map[string]string{"pods": "yes"}, nil, "poolerPods"),
			}
			after := deployment()
			Expect(hashOf(after)).NotTo(Equal(hashOf(before)))
			Expect(after.Spec.Template.Labels).To(HaveKeyWithValue("pods", "yes"))
		})

		DescribeTable("changes it again when a value of a rule changes",
			func(target apiv1.MetadataPropagationTarget) {
				pooler.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
					rule(map[string]string{"x": "1"}, nil, target),
				}
				first := hashOf(deployment())
				for range 10 {
					Expect(hashOf(deployment())).To(Equal(first))
				}
				pooler.Spec.MetadataPropagation[0].Labels["x"] = "2"
				Expect(hashOf(deployment())).NotTo(Equal(first))
			},
			Entry("on the Pods", apiv1.MetadataPropagationTarget("poolerPods")),
			Entry("on the Deployment", apiv1.MetadataPropagationTarget("poolerDeployments")),
			Entry("on every object", apiv1.MetadataPropagationTarget("all")),
		)

		// The hash of the Pooler spec already follows the rules of the
		// Pooler: the rules of the operator are those only the metadata in
		// the hash makes the Deployment follow
		DescribeTable("follows the rules of the operator",
			func(target string, podsChange bool) {
				before := deployment()
				config.Current.MandatoryMetadata = `[{"targets": ["` + target + `"], "labels": {"x": "1"}}]`
				Expect(config.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
				first := deployment()
				Expect(hashOf(first)).NotTo(Equal(hashOf(before)))
				if podsChange {
					Expect(first.Spec.Template.Labels).To(HaveKeyWithValue("x", "1"))
				} else {
					Expect(first.Spec.Template).To(Equal(before.Spec.Template))
				}

				config.Current.MandatoryMetadata = `[{"targets": ["` + target + `"], "labels": {"x": "2"}}]`
				Expect(config.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
				Expect(hashOf(deployment())).NotTo(Equal(hashOf(first)))
			},
			Entry("on the Deployment, without touching the Pods", "poolerDeployments", false),
			Entry("on the Pods", "poolerPods", true),
			Entry("on every Pod", "allPods", true),
			Entry("on every object", "all", true),
		)

		It("keeps the stock hash when the rules of the operator reach neither the Pods nor the Deployment", func() {
			config.Current.DefaultMetadata = rulesYAML([]string{"x=1@services+secrets+pgPods"}, nil)
			Expect(config.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
			Expect(hashOf(deployment())).To(Equal(mustComputeTemplateHash(pooler)))
		})

		It("leaves the Pooler spec untouched", func() {
			pooler.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
				rule(map[string]string{"x": "1"}, nil, "all"),
			}
			deployment()
			Expect(pooler.Spec.Template.ObjectMeta.Labels).To(Equal(map[string]string{"team": "payments", "tier": "template"}))
		})
	})
})
