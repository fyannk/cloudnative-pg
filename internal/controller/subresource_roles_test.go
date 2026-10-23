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
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The roles for the scale and status subresources are hand-written manifests
// shipped with the operator: these specs read them back, so that a wildcard,
// an extra resource or an aggregation label that would grant them at upgrade
// time is caught here.
var _ = Describe("Roles for the subresources", func() {
	readRole := func(file string) rbacv1.ClusterRole {
		var role rbacv1.ClusterRole
		content, err := os.ReadFile(filepath.Join("..", "..", "config", "rbac", file)) //nolint: gosec
		Expect(err).ToNot(HaveOccurred())
		Expect(yaml.Unmarshal(content, &role)).To(Succeed())
		return role
	}

	DescribeTable("grant exactly their subresources",
		func(file, name string, expected []rbacv1.PolicyRule) {
			role := readRole(file)
			Expect(role.Name).To(Equal(name))
			Expect(role.Rules).To(Equal(expected))
		},
		Entry("scale", "scale_editor_role.yaml", "scale-editor-role", []rbacv1.PolicyRule{{
			APIGroups: []string{"postgresql.cnpg.io"},
			Resources: []string{"clusters/scale", "poolers/scale"},
			Verbs:     []string{"get", "patch", "update"},
		}}),
		Entry("status", "cluster_status_editor_role.yaml", "cluster-status-editor-role", []rbacv1.PolicyRule{{
			APIGroups: []string{"postgresql.cnpg.io"},
			Resources: []string{"clusters/status"},
			Verbs:     []string{"patch", "update"},
		}}),
	)

	DescribeTable("are not aggregated by default",
		func(file string) {
			role := readRole(file)
			Expect(role.AggregationRule).To(BeNil())
			for label := range role.Labels {
				Expect(strings.HasPrefix(label, "rbac.authorization.k8s.io/aggregate-to-")).To(BeFalse(), label)
			}
		},
		Entry("scale", "scale_editor_role.yaml"),
		Entry("status", "cluster_status_editor_role.yaml"),
	)

	It("are installed with the operator", func() {
		content, err := os.ReadFile(filepath.Join("..", "..", "config", "rbac", "kustomization.yaml"))
		Expect(err).ToNot(HaveOccurred())
		Expect(string(content)).To(ContainSubstring("- scale_editor_role.yaml"))
		Expect(string(content)).To(ContainSubstring("- cluster_status_editor_role.yaml"))
	})
})
