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

var _ = Describe("Own object names", func() {
	type names struct {
		caSecret, webhookSecret, webhookService, lease, mutating, validating string
	}
	resolve := func(config Data) names {
		return names{
			caSecret:       config.GetCaSecretName(),
			webhookSecret:  config.GetWebhookSecretName(),
			webhookService: config.GetWebhookServiceName(),
			lease:          config.GetLeaderElectionID(),
			mutating:       config.GetMutatingWebhookConfigurationName(),
			validating:     config.GetValidatingWebhookConfigurationName(),
		}
	}
	literals := names{
		caSecret:       "cnpg-ca-secret",
		webhookSecret:  "cnpg-webhook-cert",
		webhookService: "cnpg-webhook-service",
		lease:          "db9c8771.cnpg.io",
		mutating:       "cnpg-mutating-webhook-configuration",
		validating:     "cnpg-validating-webhook-configuration",
	}

	DescribeTable("resolve",
		func(config Data, expected names) {
			Expect(resolve(config)).To(Equal(expected))
		},
		Entry("to the historical literals when the Deployment name is not known",
			Data{}, literals),
		Entry("to names derived from the Deployment name when it is known",
			Data{OperatorDeploymentName: "cnpg-toto"},
			names{
				caSecret:       "cnpg-toto-ca-secret",
				webhookSecret:  "cnpg-toto-webhook-cert",
				webhookService: "cnpg-toto-webhook-service",
				lease:          "cnpg-toto.cnpg.io",
				mutating:       "cnpg-toto-mutating-webhook-configuration",
				validating:     "cnpg-toto-validating-webhook-configuration",
			}),
		Entry("to the explicit names, which win over the derived ones",
			Data{
				OperatorDeploymentName:             "cnpg-toto",
				CaSecretName:                       "toto-ca",
				WebhookServiceName:                 "toto-webhook",
				LeaderElectionID:                   "toto-leader",
				MutatingWebhookConfigurationName:   "toto-mutating",
				ValidatingWebhookConfigurationName: "toto-validating",
			},
			names{
				caSecret:       "toto-ca",
				webhookSecret:  "cnpg-toto-webhook-cert",
				webhookService: "toto-webhook",
				lease:          "toto-leader",
				mutating:       "toto-mutating",
				validating:     "toto-validating",
			}),
		Entry("to the explicit names, which win over the literals too",
			Data{
				CaSecretName:       "toto-ca",
				WebhookServiceName: "toto-webhook",
				LeaderElectionID:   "toto-leader",
			},
			names{
				caSecret:       "toto-ca",
				webhookSecret:  "cnpg-webhook-cert",
				webhookService: "toto-webhook",
				lease:          "toto-leader",
				mutating:       "cnpg-mutating-webhook-configuration",
				validating:     "cnpg-validating-webhook-configuration",
			}),
	)

	It("composes the namespace suffix on the derived webhook configuration names", func() {
		config := Data{
			OperatorDeploymentName:       "cnpg-toto",
			EnableWebhookNamespaceSuffix: true,
			OperatorNamespace:            "operators",
		}
		Expect(config.GetMutatingWebhookConfigurationName()).
			To(Equal("cnpg-toto-mutating-webhook-configuration-operators"))
		Expect(config.GetValidatingWebhookConfigurationName()).
			To(Equal("cnpg-toto-validating-webhook-configuration-operators"))
	})

	It("keeps the explicit webhook configuration names as given, derived base or not", func() {
		config := Data{
			OperatorDeploymentName:           "cnpg-toto",
			EnableWebhookNamespaceSuffix:     true,
			OperatorNamespace:                "operators",
			MutatingWebhookConfigurationName: "toto-mutating",
		}
		Expect(config.GetMutatingWebhookConfigurationName()).To(Equal("toto-mutating"))
		Expect(config.GetValidatingWebhookConfigurationName()).
			To(Equal("cnpg-toto-validating-webhook-configuration-operators"))
	})

	It("reads the identity and the explicit names from the configuration", func() {
		config := newDefaultConfig()
		config.ReadConfigMap(map[string]string{
			"OPERATOR_DEPLOYMENT_NAME": "cnpg-toto",
			"CA_SECRET_NAME":           "toto-ca",
			"WEBHOOK_SERVICE_NAME":     "toto-webhook",
			"LEADER_ELECTION_ID":       "toto-leader",
		})
		Expect(config.OperatorDeploymentName).To(Equal("cnpg-toto"))
		Expect(config.GetCaSecretName()).To(Equal("toto-ca"))
		Expect(config.GetWebhookSecretName()).To(Equal("cnpg-toto-webhook-cert"))
		Expect(config.GetWebhookServiceName()).To(Equal("toto-webhook"))
		Expect(config.GetLeaderElectionID()).To(Equal("toto-leader"))
	})

	DescribeTable("list the webhook Service names, subject first",
		func(config Data, expected []string) {
			Expect(config.GetWebhookServiceNames()).To(Equal(expected))
		},
		Entry("the literal alone when nothing is set",
			Data{}, []string{"cnpg-webhook-service"}),
		Entry("the derived name then the literal",
			Data{OperatorDeploymentName: "cnpg-toto"},
			[]string{"cnpg-toto-webhook-service", "cnpg-webhook-service"}),
		Entry("the explicit name, the derived one, the literal",
			Data{OperatorDeploymentName: "cnpg-toto", WebhookServiceName: "toto-webhook"},
			[]string{"toto-webhook", "cnpg-toto-webhook-service", "cnpg-webhook-service"}),
		Entry("the explicit name then the literal, without the Deployment name",
			Data{WebhookServiceName: "toto-webhook"},
			[]string{"toto-webhook", "cnpg-webhook-service"}),
		Entry("no duplicate when the explicit name is the literal",
			Data{OperatorDeploymentName: "cnpg-toto", WebhookServiceName: "cnpg-webhook-service"},
			[]string{"cnpg-webhook-service", "cnpg-toto-webhook-service"}),
	)

	It("leaves the defaults untouched when nothing is configured", func() {
		Expect(resolve(*newDefaultConfig())).To(Equal(literals))
	})
})

var _ = Describe("Operator status identity", func() {
	type reported struct {
		version, name, namespace bool
	}
	resolve := func(config Data) reported {
		return reported{
			version:   config.ReportsOperatorVersion(),
			name:      config.ReportsOperatorName(),
			namespace: config.ReportsOperatorNamespace(),
		}
	}

	DescribeTable("decides what the operator reports",
		func(mode string, expected reported) {
			config := Data{OperatorStatusIdentity: mode}
			Expect(config.ValidateOperatorStatusIdentity()).To(Succeed())
			Expect(resolve(config)).To(Equal(expected))
		},
		Entry("off: nothing", OperatorStatusIdentityOff, reported{}),
		Entry("version: the version", OperatorStatusIdentityVersion, reported{version: true}),
		Entry("name: the version and the Deployment name", OperatorStatusIdentityName,
			reported{version: true, name: true}),
		Entry("full: the version, the Deployment name and the namespace", OperatorStatusIdentityFull,
			reported{version: true, name: true, namespace: true}),
	)

	It("reports the version by default", func() {
		Expect(newDefaultConfig().OperatorStatusIdentity).To(Equal(OperatorStatusIdentityVersion))
		Expect(resolve(*newDefaultConfig())).To(Equal(reported{version: true}))
	})

	It("refuses an unknown value and reports nothing with it", func() {
		config := Data{OperatorStatusIdentity: "verbose"}
		Expect(config.ValidateOperatorStatusIdentity()).To(MatchError(
			ContainSubstring("OPERATOR_STATUS_IDENTITY \"verbose\" is not one of off, version, name, full")))
		Expect(resolve(config)).To(Equal(reported{}))
	})

	It("treats an empty value as the default", func() {
		Expect((&Data{}).ValidateOperatorStatusIdentity()).To(Succeed())
		Expect(resolve(Data{})).To(Equal(reported{version: true}))
	})

	It("is read from the configuration data", func() {
		config := NewFromData(map[string]string{"OPERATOR_STATUS_IDENTITY": "full"})
		Expect(resolve(*config)).To(Equal(reported{version: true, name: true, namespace: true}))
	})
})
