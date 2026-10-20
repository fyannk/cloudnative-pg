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

package configuration

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Annotation and label inheritance", func() {
	It("manages inherited annotations", func() {
		config := Data{
			InheritedAnnotations: []string{"one", "two"},
		}

		Expect(config.IsAnnotationInherited("one")).To(BeTrue())
		Expect(config.IsAnnotationInherited("two")).To(BeTrue())
		Expect(config.IsAnnotationInherited("three")).To(BeFalse())
	})

	It("manages inherited labels", func() {
		config := Data{
			InheritedLabels: []string{"alpha", "beta"},
		}

		Expect(config.IsLabelInherited("alpha")).To(BeTrue())
		Expect(config.IsLabelInherited("beta")).To(BeTrue())
		Expect(config.IsLabelInherited("gamma")).To(BeFalse())
	})

	It("manages inherited annotations containing glob patterns", func() {
		config := Data{
			InheritedAnnotations: []string{"qa.test.com/*", "prod.test.com/*"},
		}

		Expect(config.IsAnnotationInherited("qa.test.com/one")).To(BeTrue())
		Expect(config.IsAnnotationInherited("prod.test.com/two")).To(BeTrue())
		Expect(config.IsAnnotationInherited("testing.test.com/three")).To(BeFalse())
	})

	It("manages inherited labels containing glob patterns", func() {
		config := Data{
			InheritedLabels: []string{"qa.testing.com/*", "prod.testing.com/*"},
		}

		Expect(config.IsLabelInherited("qa.testing.com/one")).To(BeTrue())
		Expect(config.IsLabelInherited("prod.testing.com/two")).To(BeTrue())
		Expect(config.IsLabelInherited("testing.testing.com/three")).To(BeFalse())
	})

	It("skips invalid patterns during evaluation", func() {
		config := Data{
			InheritedLabels: []string{"[abc", "prod.testing.com/*"},
		}

		Expect(config.IsLabelInherited("prod.testing.com/two")).To(BeTrue())
		Expect(config.IsLabelInherited("testing.testing.com/three")).To(BeFalse())
	})

	When("every namespace is watched", func() {
		It("sets the watched namespaces to empty", func() {
			config := Data{
				WatchNamespace: "",
			}
			Expect(config.WatchedNamespaces()).To(BeEmpty())

			// Additional commas and spaces doesn't change the meaning
			config = Data{
				WatchNamespace: ",  ,,",
			}
			Expect(config.WatchedNamespaces()).To(BeEmpty())
		})
	})

	When("a single namespace is watched", func() {
		It("sets the watched namespaces to that one", func() {
			config := Data{
				WatchNamespace: "pg",
			}
			Expect(config.WatchedNamespaces()).To(HaveLen(1))
			Expect(config.WatchedNamespaces()[0]).To(Equal("pg"))

			// Additional commas and spaces doesn't change the meaning
			config = Data{
				WatchNamespace: ",  ,pg, ",
			}
			Expect(config.WatchedNamespaces()).To(HaveLen(1))
			Expect(config.WatchedNamespaces()[0]).To(Equal("pg"))
		})
	})

	When("multiple namespaces are specified", func() {
		It("sets the watched namespaces to the correct list", func() {
			config := Data{
				WatchNamespace: "pg,pg_staging,pg_prod",
			}
			Expect(config.WatchedNamespaces()).To(HaveLen(3))
			Expect(config.WatchedNamespaces()).To(Equal([]string{
				"pg",
				"pg_staging",
				"pg_prod",
			}))

			// Additional commas and spaces doesn't change the meaning
			config = Data{
				WatchNamespace: ",  ,pg ,pg_staging   ,  pg_prod, ",
			}
			Expect(config.WatchedNamespaces()).To(HaveLen(3))
			Expect(config.WatchedNamespaces()).To(Equal([]string{
				"pg",
				"pg_staging",
				"pg_prod",
			}))
		})
	})

	Context("included plugin list", func() {
		It("is empty by default", func() {
			Expect(newDefaultConfig().GetIncludePlugins()).To(BeEmpty())
		})

		It("contains a set of comma-separated plugins", func() {
			Expect((&Data{
				IncludePlugins: "a,b,c",
			}).GetIncludePlugins()).To(ContainElements("a", "b", "c"))
			Expect((&Data{
				IncludePlugins: "a,,,b,c",
			}).GetIncludePlugins()).To(ContainElements("a", "b", "c"))
			Expect((&Data{
				IncludePlugins: "a,,,b , c",
			}).GetIncludePlugins()).To(ContainElements("a", "b", "c"))
		})
	})

	It("returns correct delay for clusters rollout", func() {
		config := Data{ClustersRolloutDelay: 10}
		Expect(config.GetClustersRolloutDelay()).To(Equal(10 * time.Second))
	})

	It("returns zero as default delay for clusters rollout when not set", func() {
		config := Data{}
		Expect(config.GetClustersRolloutDelay()).To(BeZero())
	})

	It("returns correct delay for instances rollout", func() {
		config := Data{InstancesRolloutDelay: 5}
		Expect(config.GetInstancesRolloutDelay()).To(Equal(5 * time.Second))
	})

	It("returns zero as default delay for instances rollout when not set", func() {
		config := Data{}
		Expect(config.GetInstancesRolloutDelay()).To(BeZero())
	})
})

var _ = Describe("Node status source", func() {
	It("is unset by default, with a ten-minute eviction age", func() {
		config := NewConfiguration()
		Expect(config.NodeStatusSource).To(BeEmpty())
		Expect(config.NodeStatusPodKeyPrefix).To(BeEmpty())
		Expect(config.NodeStatusLabelPrefix).To(BeEmpty())
		Expect(config.GetNodeStatusEvictionMaxAge()).To(Equal(10 * time.Minute))
	})

	It("follows the Node API when unset", func() {
		config := NewConfiguration()
		Expect(config.ResolveNodeStatusSource()).To(Succeed())
		Expect(config.NodeStatusSource).To(Equal(NodeStatusSourceAPI))
		Expect(config.ReadsNodeAPI()).To(BeTrue())

		config = NewConfiguration()
		config.APINodeEnabled = false
		Expect(config.ResolveNodeStatusSource()).To(Succeed())
		Expect(config.NodeStatusSource).To(Equal(NodeStatusSourceNone))
		Expect(config.ReadsNodeAPI()).To(BeFalse())
		Expect(config.ReadsNodeStatusFromPods()).To(BeFalse())
	})

	It("accepts an explicit api source while the Node API is read", func() {
		config := NewConfiguration()
		config.NodeStatusSource = NodeStatusSourceAPI
		Expect(config.ResolveNodeStatusSource()).To(Succeed())
		Expect(config.ReadsNodeAPI()).To(BeTrue())
	})

	It("refuses an explicit api source without the Node API", func() {
		config := NewConfiguration()
		config.APINodeEnabled = false
		config.NodeStatusSource = NodeStatusSourceAPI
		Expect(config.ResolveNodeStatusSource()).To(MatchError(ContainSubstring("API_NODE_ENABLED")))
	})

	It("reads the settings of the pods source from the configuration data, with or without the Node API", func() {
		for _, apiNode := range []bool{true, false} {
			config := NewConfiguration()
			config.APINodeEnabled = apiNode
			config.ReadConfigMap(map[string]string{
				"NODE_STATUS_SOURCE":           "pods",
				"NODE_STATUS_POD_KEY_PREFIX":   "bridge.example.com/",
				"NODE_STATUS_LABEL_PREFIX":     "node.example.com/",
				"NODE_STATUS_EVICTION_MAX_AGE": "120",
			})
			Expect(config.ResolveNodeStatusSource()).To(Succeed())
			Expect(config.ReadsNodeAPI()).To(BeFalse())
			Expect(config.ReadsNodeStatusFromPods()).To(BeTrue())
			Expect(config.NodeStatusPodKeyPrefix).To(Equal("bridge.example.com/"))
			Expect(config.NodeStatusLabelPrefix).To(Equal("node.example.com/"))
			Expect(config.GetNodeStatusEvictionMaxAge()).To(Equal(2 * time.Minute))
		}
	})

	It("accepts the none source with or without the Node API, and reads nothing", func() {
		for _, apiNode := range []bool{true, false} {
			config := Data{NodeStatusSource: NodeStatusSourceNone, APINodeEnabled: apiNode}
			Expect(config.ResolveNodeStatusSource()).To(Succeed())
			Expect(config.ReadsNodeAPI()).To(BeFalse())
			Expect(config.ReadsNodeStatusFromPods()).To(BeFalse())
		}
	})

	It("refuses the pods source without a key prefix", func() {
		config := Data{NodeStatusSource: NodeStatusSourcePods}
		Expect(config.ResolveNodeStatusSource()).To(MatchError(ContainSubstring("NODE_STATUS_POD_KEY_PREFIX")))
	})

	It("refuses a negative eviction age with the pods source", func() {
		config := Data{NodeStatusSource: NodeStatusSourcePods, NodeStatusPodKeyPrefix: "b/", NodeStatusEvictionMaxAge: -1}
		Expect(config.ResolveNodeStatusSource()).To(MatchError(ContainSubstring("NODE_STATUS_EVICTION_MAX_AGE")))
	})

	It("accepts no eviction age limit with the pods source", func() {
		config := Data{NodeStatusSource: NodeStatusSourcePods, NodeStatusPodKeyPrefix: "b/"}
		Expect(config.ResolveNodeStatusSource()).To(Succeed())
		Expect(config.GetNodeStatusEvictionMaxAge()).To(BeZero())
	})

	It("refuses an unknown source", func() {
		config := Data{NodeStatusSource: "nodes"}
		Expect(config.ResolveNodeStatusSource()).To(MatchError(ContainSubstring(`unknown NODE_STATUS_SOURCE "nodes"`)))
	})
})

var _ = Describe("Webhook configuration names", func() {
	It("uses default names when suffix is disabled", func() {
		config := &Data{}
		Expect(config.GetMutatingWebhookConfigurationName()).To(Equal(DefaultMutatingWebhookConfigurationName))
		Expect(config.GetValidatingWebhookConfigurationName()).To(Equal(DefaultValidatingWebhookConfigurationName))
	})

	It("appends operator namespace when suffix is enabled", func() {
		config := &Data{
			EnableWebhookNamespaceSuffix: true,
			OperatorNamespace:            "cnpg-team-a",
		}
		Expect(config.GetMutatingWebhookConfigurationName()).To(
			Equal(DefaultMutatingWebhookConfigurationName + "-cnpg-team-a"))
		Expect(config.GetValidatingWebhookConfigurationName()).To(
			Equal(DefaultValidatingWebhookConfigurationName + "-cnpg-team-a"))
	})

	It("getter falls back to default names when suffix is enabled but OperatorNamespace is empty", func() {
		config := &Data{EnableWebhookNamespaceSuffix: true}
		Expect(config.GetMutatingWebhookConfigurationName()).To(Equal(DefaultMutatingWebhookConfigurationName))
		Expect(config.GetValidatingWebhookConfigurationName()).To(Equal(DefaultValidatingWebhookConfigurationName))
	})
})

var _ = Describe("Optional APIs", func() {
	It("are enabled by default and disabled by their settings", func() {
		config := NewConfiguration()
		Expect(config.APIPodMonitorEnabled).To(BeTrue())
		Expect(config.APIVolumeSnapshotEnabled).To(BeTrue())
		Expect(config.APIClusterImageCatalogEnabled).To(BeTrue())
		Expect(config.APINodeEnabled).To(BeTrue())

		config.ReadConfigMap(map[string]string{
			"API_NODE_ENABLED":                  "false",
			"API_POD_MONITOR_ENABLED":           "false",
			"API_VOLUME_SNAPSHOT_ENABLED":       "false",
			"API_CLUSTER_IMAGE_CATALOG_ENABLED": "false",
		})
		Expect(config.APIPodMonitorEnabled).To(BeFalse())
		Expect(config.APIVolumeSnapshotEnabled).To(BeFalse())
		Expect(config.APIClusterImageCatalogEnabled).To(BeFalse())
		Expect(config.APINodeEnabled).To(BeFalse())
	})
})
