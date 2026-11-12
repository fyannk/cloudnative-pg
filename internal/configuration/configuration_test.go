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

// Goal: an explicit webhook configuration name is used as given, and the
// default name with its optional namespace suffix applies otherwise
var _ = Describe("Explicit webhook configuration names", func() {
	DescribeTable("resolve",
		func(config Data, mutating, validating string) {
			Expect(config.GetMutatingWebhookConfigurationName()).To(Equal(mutating))
			Expect(config.GetValidatingWebhookConfigurationName()).To(Equal(validating))
		},
		Entry("to the defaults when nothing is set",
			Data{},
			DefaultMutatingWebhookConfigurationName, DefaultValidatingWebhookConfigurationName),
		Entry("to the explicit names when set",
			Data{
				MutatingWebhookConfigurationName:   "platform-cnpg-mutating",
				ValidatingWebhookConfigurationName: "platform-cnpg-validating",
			},
			"platform-cnpg-mutating", "platform-cnpg-validating"),
		Entry("to the explicit names as given, suffix or not",
			Data{
				MutatingWebhookConfigurationName:   "platform-cnpg-mutating",
				ValidatingWebhookConfigurationName: "platform-cnpg-validating",
				EnableWebhookNamespaceSuffix:       true,
				OperatorNamespace:                  "cnpg-team-a",
			},
			"platform-cnpg-mutating", "platform-cnpg-validating"),
		Entry("one explicit, the other suffixed",
			Data{
				MutatingWebhookConfigurationName: "platform-cnpg-mutating",
				EnableWebhookNamespaceSuffix:     true,
				OperatorNamespace:                "cnpg-team-a",
			},
			"platform-cnpg-mutating", DefaultValidatingWebhookConfigurationName+"-cnpg-team-a"),
	)

	It("are read from the operator configuration", func() {
		config := newDefaultConfig()
		config.ReadConfigMap(map[string]string{
			"MUTATING_WEBHOOK_CONFIGURATION_NAME":   "platform-cnpg-mutating",
			"VALIDATING_WEBHOOK_CONFIGURATION_NAME": "platform-cnpg-validating",
		})
		Expect(config.GetMutatingWebhookConfigurationName()).To(Equal("platform-cnpg-mutating"))
		Expect(config.GetValidatingWebhookConfigurationName()).To(Equal("platform-cnpg-validating"))
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

var _ = Describe("Proxy propagation setting", func() {
	It("is on by default", func() {
		Expect(newDefaultConfig().PropagateProxyEnv).To(BeTrue())
	})

	It("is switched off from the operator configuration", func() {
		config := newDefaultConfig()
		config.ReadConfigMap(map[string]string{"PROPAGATE_PROXY_ENV": "false"})
		Expect(config.PropagateProxyEnv).To(BeFalse())
	})

	It("is switched off from the environment", func() {
		GinkgoT().Setenv("PROPAGATE_PROXY_ENV", "false")
		Expect(NewConfiguration().PropagateProxyEnv).To(BeFalse())
	})

	It("stays on with an invalid value", func() {
		config := newDefaultConfig()
		config.ReadConfigMap(map[string]string{"PROPAGATE_PROXY_ENV": "no thanks"})
		Expect(config.PropagateProxyEnv).To(BeTrue())
	})
})

var _ = Describe("Operator Deployment identity", func() {
	It("derives operator object names from the Deployment", func() {
		config := &Data{OperatorDeploymentName: "cnpg-toto"}
		Expect(config.GetCaSecretName()).To(Equal("cnpg-toto-ca-secret"))
		Expect(config.GetWebhookSecretName()).To(Equal("cnpg-toto-webhook-cert"))
		Expect(config.GetWebhookServiceName()).To(Equal("cnpg-toto-webhook-service"))
		Expect(config.GetLeaderElectionID()).To(Equal("cnpg-toto.cnpg.io"))
		Expect(config.GetMutatingWebhookConfigurationName()).To(Equal("cnpg-toto-mutating-webhook-configuration"))
		Expect(config.GetValidatingWebhookConfigurationName()).To(Equal("cnpg-toto-validating-webhook-configuration"))
	})

	It("keeps explicit names and applies suffixes only to derived names", func() {
		config := &Data{
			OperatorDeploymentName:           "cnpg-toto",
			EnableWebhookNamespaceSuffix:     true,
			OperatorNamespace:                "operators",
			MutatingWebhookConfigurationName: "custom-mutating",
		}
		Expect(config.GetMutatingWebhookConfigurationName()).To(Equal("custom-mutating"))
		Expect(config.GetValidatingWebhookConfigurationName()).
			To(Equal("cnpg-toto-validating-webhook-configuration-operators"))
	})

	DescribeTable("returns webhook Service SAN candidates in precedence order",
		func(config Data, expected []string) {
			Expect(config.GetWebhookServiceNames()).To(Equal(expected))
		},
		Entry("historical name", Data{}, []string{"cnpg-webhook-service"}),
		Entry("derived then historical", Data{OperatorDeploymentName: "cnpg-toto"},
			[]string{"cnpg-toto-webhook-service", "cnpg-webhook-service"}),
		Entry("explicit then derived then historical", Data{
			OperatorDeploymentName: "cnpg-toto", WebhookServiceName: "custom-webhook",
		}, []string{"custom-webhook", "cnpg-toto-webhook-service", "cnpg-webhook-service"}),
	)

	It("reads identity and overrides from operator configuration", func() {
		config := newDefaultConfig()
		config.ReadConfigMap(map[string]string{
			"OPERATOR_DEPLOYMENT_NAME":              "cnpg-toto",
			"CA_SECRET_NAME":                        "custom-ca",
			"WEBHOOK_SERVICE_NAME":                  "custom-webhook",
			"LEADER_ELECTION_ID":                    "custom-lease",
			"MUTATING_WEBHOOK_CONFIGURATION_NAME":   "custom-mutating",
			"VALIDATING_WEBHOOK_CONFIGURATION_NAME": "custom-validating",
		})
		Expect(config.GetCaSecretName()).To(Equal("custom-ca"))
		Expect(config.GetWebhookServiceName()).To(Equal("custom-webhook"))
		Expect(config.GetLeaderElectionID()).To(Equal("custom-lease"))
		Expect(config.GetMutatingWebhookConfigurationName()).To(Equal("custom-mutating"))
		Expect(config.GetValidatingWebhookConfigurationName()).To(Equal("custom-validating"))
	})
})
