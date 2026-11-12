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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The metadata of the objects of a Pooler goes through ApplyMetadataTo:
// without any rule it must leave the objects as the operator built them
// before, then each layer must land where, and only where, it is meant to.
var _ = Describe("The metadata of the objects of a Pooler", func() {
	settings := func(defaults, mandatory string) {
		configuration.Current.DefaultMetadata = defaults
		configuration.Current.MandatoryMetadata = mandatory
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
	}

	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
	})

	templated := func() *Pooler {
		return &Pooler{
			ObjectMeta: metav1.ObjectMeta{Name: "p"},
			Spec: PoolerSpec{
				Template: &PodTemplateSpec{ObjectMeta: Metadata{
					Labels:      map[string]string{"team": "pods"},
					Annotations: map[string]string{"note": "pods"},
				}},
				ServiceTemplate: &ServiceTemplateSpec{ObjectMeta: Metadata{
					Labels: map[string]string{"team": "service"},
				}},
			},
		}
	}

	Context("without rules, from the Pooler or the operator", func() {
		DescribeTable("leaves every kind of object as it is",
			func(existing metav1.ObjectMeta) {
				pooler := templated()
				for _, target := range PoolerMetadataTargets {
					meta := *existing.DeepCopy()
					pooler.ApplyMetadataTo(&meta, target)
					Expect(meta).To(Equal(existing), string(target))
					Expect(pooler.MetadataReaches(target)).To(BeFalse(), string(target))
				}
			},
			Entry("no metadata: kept nil, not emptied", metav1.ObjectMeta{}),
			Entry("existing metadata", metav1.ObjectMeta{
				Labels:      map[string]string{"cnpg.io/poolerName": "p"},
				Annotations: map[string]string{"cnpg.io/podSpec": "{}"},
			}),
		)

		It("gives the templates' metadata to the Pods and the Service only", func() {
			pooler := templated()
			Expect(pooler.GetPoolerLabelsFor(metadatarules.PoolerPods)).To(Equal(map[string]string{"team": "pods"}))
			Expect(pooler.GetPoolerAnnotationsFor(metadatarules.PoolerPods)).
				To(Equal(map[string]string{"note": "pods"}))
			Expect(pooler.GetPoolerLabelsFor(metadatarules.Services)).To(Equal(map[string]string{"team": "service"}))
			Expect(pooler.GetPoolerAnnotationsFor(metadatarules.Services)).To(BeEmpty())
			for _, target := range []metadatarules.Target{
				metadatarules.All, metadatarules.PoolerDeployments, metadatarules.Secrets,
			} {
				Expect(pooler.GetPoolerLabelsFor(target)).To(BeEmpty(), string(target))
				Expect(pooler.GetPoolerAnnotationsFor(target)).To(BeEmpty(), string(target))
			}
		})

		It("works without templates", func() {
			pooler := &Pooler{}
			Expect(pooler.GetPoolerLabelsFor(metadatarules.PoolerPods)).To(BeEmpty())
			Expect(pooler.GetPoolerLabelsFor(metadatarules.Services)).To(BeEmpty())
		})
	})

	Context("with rules", func() {
		It("puts the rules of the Pooler on top of the templates, the most specific target winning", func() {
			pooler := templated()
			pooler.Spec.MetadataPropagation = []MetadataPropagationRule{
				rule(map[string]string{"team": "all", "env": "prod"}, nil, "all"),
				rule(map[string]string{"team": "rule"}, map[string]string{"note": "rule"}, "poolerPods"),
			}
			Expect(pooler.GetPoolerLabelsFor(metadatarules.PoolerPods)).
				To(Equal(map[string]string{"team": "rule", "env": "prod"}))
			Expect(pooler.GetPoolerAnnotationsFor(metadatarules.PoolerPods)).
				To(Equal(map[string]string{"note": "rule"}))
			// The rule for all wins over the service template
			Expect(pooler.GetPoolerLabelsFor(metadatarules.Services)).
				To(Equal(map[string]string{"team": "all", "env": "prod"}))
			Expect(pooler.GetPoolerLabelsFor(metadatarules.PoolerDeployments)).
				To(Equal(map[string]string{"team": "all", "env": "prod"}))
		})

		It("reaches only the targeted kinds", func() {
			pooler := &Pooler{Spec: PoolerSpec{MetadataPropagation: []MetadataPropagationRule{
				rule(nil, map[string]string{"example.com/owner": "x"}, "poolerDeployments"),
			}}}
			for _, target := range PoolerMetadataTargets {
				Expect(pooler.MetadataReaches(target)).
					To(Equal(target == metadatarules.PoolerDeployments), string(target))
			}
		})

		It("stacks defaults, the object's metadata, the Pooler's rules and the mandatory values", func() {
			settings(
				`[{"targets": ["all"], "labels": {"env": "dev", "default": "d", "team": "default"}}]`,
				`[{"targets": ["secrets"], "labels": {"env": "prod"}}]`,
			)
			pooler := &Pooler{Spec: PoolerSpec{MetadataPropagation: []MetadataPropagationRule{
				rule(map[string]string{"env": "pooler", "own": "o"}, nil, "secrets"),
			}}}
			meta := metav1.ObjectMeta{Labels: map[string]string{"team": "object"}}
			pooler.ApplyMetadataTo(&meta, metadatarules.Secrets)
			Expect(meta.Labels).To(Equal(map[string]string{
				"default": "d", "team": "object", "own": "o", "env": "prod",
			}))
			Expect(meta.Annotations).To(BeNil())
			Expect(pooler.MetadataReaches(metadatarules.PoolerPods)).To(BeTrue())
		})

		It("ignores the targets of a Cluster and the invalid part of the rules", func() {
			pooler := &Pooler{Spec: PoolerSpec{MetadataPropagation: []MetadataPropagationRule{
				rule(map[string]string{"a": "1"}, nil, "pgPods", "poolerPods"),
				rule(map[string]string{"cnpg.io/poolerName": "x"}, nil, "poolerPods"),
			}}}
			Expect(pooler.GetPoolerLabelsFor(metadatarules.PoolerPods)).To(Equal(map[string]string{"a": "1"}))
			Expect(pooler.GetPoolerLabelsFor(metadatarules.PgPods)).To(BeEmpty())
		})
	})
})
