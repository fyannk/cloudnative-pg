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
	"os"
	"path/filepath"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The operator ClusterRole is generated from the kubebuilder markers of this
// package: these specs read the generated manifest back, so that a marker
// added or kept by mistake is caught here and not at installation time.
var _ = Describe("Generated operator ClusterRole", func() {
	var role rbacv1.ClusterRole

	BeforeEach(func() {
		content, err := os.ReadFile(filepath.Join("..", "..", "config", "rbac", "role.yaml"))
		Expect(err).ToNot(HaveOccurred())
		Expect(yaml.Unmarshal(content, &role)).To(Succeed())
		Expect(role.Rules).ToNot(BeEmpty())
	})

	// The operator only reads and patches its own two webhook configurations,
	// by name, to inject its CA bundle. Control over any webhook configuration
	// reaches every object admitted to the cluster, so the rules are bounded
	// with resourceNames. kustomize's namePrefix does not rewrite resourceNames:
	// the markers carry the final names, checked here against the names the
	// operator uses.
	It("bounds the webhook configuration rules to the operator's own two objects", func() {
		expected := map[string]string{
			"mutatingwebhookconfigurations":   configuration.DefaultMutatingWebhookConfigurationName,
			"validatingwebhookconfigurations": configuration.DefaultValidatingWebhookConfigurationName,
		}
		found := map[string]bool{}
		for _, rule := range role.Rules {
			for _, resource := range rule.Resources {
				name, isWebhookConfiguration := expected[resource]
				if !isWebhookConfiguration {
					continue
				}
				Expect(rule.ResourceNames).To(ConsistOf(name), resource)
				Expect(rule.Verbs).To(ConsistOf("get", "patch"), resource)
				found[resource] = true
			}
		}
		Expect(found).To(HaveLen(len(expected)))
	})
})
