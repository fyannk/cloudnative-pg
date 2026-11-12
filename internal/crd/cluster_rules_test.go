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

package crd

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	image17 = "ghcr.io/cloudnative-pg/postgresql:17.2"
	digest  = "@sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

// storedCluster creates a Cluster as the API server stores it once the
// operator has recorded the PostgreSQL major version in its status: the
// defaults are applied and `status.pgDataImageInfo` is set when major is
// not zero.
func storedCluster(ctx context.Context, major int) *apiv1.Cluster {
	cluster := &apiv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cel-" + rand.String(8),
			Namespace: "default",
		},
		Spec: apiv1.ClusterSpec{
			Instances:            1,
			ImageName:            image17,
			StorageConfiguration: apiv1.StorageConfiguration{Size: "1Gi"},
			WalStorage:           &apiv1.StorageConfiguration{Size: "1Gi"},
			Tablespaces: []apiv1.TablespaceConfiguration{
				{Name: "a", Storage: apiv1.StorageConfiguration{Size: "1Gi"}},
				{Name: "b", Storage: apiv1.StorageConfiguration{Size: "1Gi"}},
			},
			PostgresConfiguration: apiv1.PostgresConfiguration{
				Parameters: map[string]string{"wal_level": "replica"},
			},
			ExternalClusters: []apiv1.ExternalCluster{
				{Name: "up", ConnectionParameters: map[string]string{"host": "up.example"}},
			},
		},
	}
	Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
	if major > 0 {
		cluster.Status.PGDataImageInfo = &apiv1.ImageInfo{Image: image17, MajorVersion: major}
		Expect(k8sClient.Status().Update(ctx, cluster)).To(Succeed())
	}
	return cluster
}

func catalogRef(major int) *apiv1.ImageCatalogRef {
	return &apiv1.ImageCatalogRef{
		TypedLocalObjectReference: corev1.TypedLocalObjectReference{
			APIGroup: ptr.To("postgresql.cnpg.io"),
			Kind:     "ImageCatalog",
			Name:     "catalog",
		},
		Major: major,
	}
}

var _ = Describe("Cluster CRD transition rules", func() {
	ctx := context.Background()

	refused := func(cluster *apiv1.Cluster, mutate func(*apiv1.Cluster), message string) {
		mutate(cluster)
		err := k8sClient.Update(ctx, cluster)
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), err.Error())
		Expect(err.Error()).To(ContainSubstring(message))
	}
	accepted := func(cluster *apiv1.Cluster, mutate func(*apiv1.Cluster)) {
		mutate(cluster)
		Expect(k8sClient.Update(ctx, cluster)).To(Succeed())
	}

	DescribeTable("refuses an update of a cluster running major 17 that",
		func(mutate func(*apiv1.Cluster), message string) {
			refused(storedCluster(ctx, 17), mutate, message)
		},
		Entry("changes postgresUID",
			func(c *apiv1.Cluster) { c.Spec.PostgresUID = 1000 },
			"postgresUID is immutable"),
		Entry("changes postgresGID",
			func(c *apiv1.Cluster) { c.Spec.PostgresGID = 1000 },
			"postgresGID is immutable"),
		Entry("removes walStorage",
			func(c *apiv1.Cluster) { c.Spec.WalStorage = nil },
			"walStorage cannot be disabled once configured"),
		Entry("removes the tablespaces section",
			func(c *apiv1.Cluster) { c.Spec.Tablespaces = nil },
			"tablespaces section cannot be deleted once created"),
		Entry("deletes a tablespace",
			func(c *apiv1.Cluster) { c.Spec.Tablespaces = c.Spec.Tablespaces[:1] },
			"no tablespace can be deleted once created"),
		Entry("shrinks a tablespace",
			func(c *apiv1.Cluster) { c.Spec.Tablespaces[1].Storage.Size = "500Mi" },
			"the size of a tablespace cannot be decreased"),
		Entry("shrinks the storage",
			func(c *apiv1.Cluster) { c.Spec.StorageConfiguration.Size = "500Mi" },
			"spec.storage.size: Invalid value"),
		Entry("shrinks the WAL storage",
			func(c *apiv1.Cluster) { c.Spec.WalStorage.Size = "500Mi" },
			"spec.walStorage.size: Invalid value"),
		Entry("sets wal_level to minimal",
			func(c *apiv1.Cluster) { c.Spec.PostgresConfiguration.Parameters["wal_level"] = "minimal" },
			"wal_level cannot be changed to minimal on an existing cluster"),
		Entry("changes the slot prefix while highAvailability is enabled",
			func(c *apiv1.Cluster) { c.Spec.ReplicationSlots.HighAvailability.SlotPrefix = "other_" },
			"highAvailability.slotPrefix cannot change while highAvailability is enabled"),
		Entry("changes the slot prefix while disabling highAvailability",
			func(c *apiv1.Cluster) {
				c.Spec.ReplicationSlots.HighAvailability.Enabled = ptr.To(false)
				c.Spec.ReplicationSlots.HighAvailability.SlotPrefix = "other_"
			},
			"highAvailability.slotPrefix cannot change while highAvailability is enabled"),
		Entry("downgrades the major version through imageName",
			func(c *apiv1.Cluster) { c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:16.4" },
			"the PostgreSQL major version cannot be downgraded"),
		Entry("downgrades the major version through an imageName with a digest",
			func(c *apiv1.Cluster) { c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:16.4" + digest },
			"the PostgreSQL major version cannot be downgraded"),
		Entry("downgrades the major version through imageCatalogRef",
			func(c *apiv1.Cluster) {
				c.Spec.ImageName = ""
				c.Spec.ImageCatalogRef = catalogRef(16)
			},
			"the PostgreSQL major version cannot be downgraded"),
	)

	DescribeTable("refuses an upgrade of a cluster running major 14 that",
		func(mutate func(*apiv1.Cluster), message string) {
			refused(storedCluster(ctx, 14), mutate, message)
		},
		Entry("targets major 15 through imageName",
			func(c *apiv1.Cluster) { c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:15.1" },
			"major upgrades are only supported to version 16 or higher"),
		Entry("targets major 15 through imageCatalogRef",
			func(c *apiv1.Cluster) {
				c.Spec.ImageName = ""
				c.Spec.ImageCatalogRef = catalogRef(15)
			},
			"major upgrades are only supported to version 16 or higher"),
	)

	DescribeTable("accepts an update of a cluster running major 17 that",
		func(mutate func(*apiv1.Cluster)) {
			accepted(storedCluster(ctx, 17), mutate)
		},
		Entry("grows the storage",
			func(c *apiv1.Cluster) { c.Spec.StorageConfiguration.Size = "2Gi" }),
		Entry("grows the WAL storage",
			func(c *apiv1.Cluster) { c.Spec.WalStorage.Size = "2Gi" }),
		Entry("grows a tablespace",
			func(c *apiv1.Cluster) { c.Spec.Tablespaces[0].Storage.Size = "2Gi" }),
		Entry("adds a tablespace",
			func(c *apiv1.Cluster) {
				c.Spec.Tablespaces = append(c.Spec.Tablespaces,
					apiv1.TablespaceConfiguration{Name: "c", Storage: apiv1.StorageConfiguration{Size: "1Gi"}})
			}),
		Entry("re-sets postgresUID and postgresGID to their current values",
			func(c *apiv1.Cluster) {
				c.Spec.PostgresUID = apiv1.DefaultPostgresUID
				c.Spec.PostgresGID = apiv1.DefaultPostgresGID
			}),
		Entry("changes wal_level from replica to logical",
			func(c *apiv1.Cluster) { c.Spec.PostgresConfiguration.Parameters["wal_level"] = "logical" }),
		Entry("changes parameters alone under the default primaryUpdateMethod",
			func(c *apiv1.Cluster) { c.Spec.PostgresConfiguration.Parameters["max_connections"] = "120" }),
		Entry("upgrades the major version through imageName",
			func(c *apiv1.Cluster) { c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:18.1" }),
		Entry("upgrades the major version through an imageName with a digest",
			func(c *apiv1.Cluster) { c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:18.1" + digest }),
		Entry("upgrades the major version through imageCatalogRef",
			func(c *apiv1.Cluster) {
				c.Spec.ImageName = ""
				c.Spec.ImageCatalogRef = catalogRef(18)
			}),
		Entry("changes to a minor version of the same major",
			func(c *apiv1.Cluster) { c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:17.4" }),
		Entry("uses an imageName whose tag does not start with a number",
			func(c *apiv1.Cluster) { c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:latest" }),
		Entry("uses an imageName with a registry port and no tag",
			func(c *apiv1.Cluster) { c.Spec.ImageName = "registry.example:5000/postgresql" }),
		Entry("uses an imageName without a tag",
			func(c *apiv1.Cluster) { c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql" }),
		Entry("changes the replica role while no switch is in progress",
			func(c *apiv1.Cluster) {
				c.Spec.ReplicaCluster = &apiv1.ReplicaClusterConfiguration{Enabled: ptr.To(true), Source: "up"}
			}),
		Entry("only changes an annotation",
			func(c *apiv1.Cluster) { c.Annotations = map[string]string{"example.com/note": "1"} }),
	)

	It("ignores the image until the status records a major version", func() {
		accepted(storedCluster(ctx, 0), func(c *apiv1.Cluster) {
			c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:15.1"
		})
	})

	It("keeps accepting wal_level minimal once set at creation", func() {
		cluster := storedCluster(ctx, 0)
		Expect(k8sClient.Delete(ctx, cluster)).To(Succeed())
		cluster.ResourceVersion = ""
		cluster.Spec.PostgresConfiguration.Parameters["wal_level"] = "minimal"
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
		accepted(cluster, func(c *apiv1.Cluster) {
			c.Spec.PostgresConfiguration.Parameters["max_connections"] = "120"
		})
	})

	It("ties the image and the parameters together under primaryUpdateMethod switchover", func() {
		cluster := storedCluster(ctx, 17)
		accepted(cluster, func(c *apiv1.Cluster) { c.Spec.PrimaryUpdateMethod = apiv1.PrimaryUpdateMethodSwitchover })
		refused(cluster, func(c *apiv1.Cluster) {
			c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:17.4"
			c.Spec.PostgresConfiguration.Parameters["max_connections"] = "120"
		}, "imageName and postgresql.parameters cannot change together when primaryUpdateMethod is switchover")

		cluster = storedCluster(ctx, 17)
		accepted(cluster, func(c *apiv1.Cluster) { c.Spec.PrimaryUpdateMethod = apiv1.PrimaryUpdateMethodSwitchover })
		accepted(cluster, func(c *apiv1.Cluster) { c.Spec.PostgresConfiguration.Parameters["max_connections"] = "120" })
		accepted(cluster, func(c *apiv1.Cluster) { c.Spec.ImageName = "ghcr.io/cloudnative-pg/postgresql:17.4" })
	})

	It("lets the slot prefix change once highAvailability is disabled", func() {
		cluster := storedCluster(ctx, 17)
		accepted(cluster, func(c *apiv1.Cluster) { c.Spec.ReplicationSlots.HighAvailability.Enabled = ptr.To(false) })
		accepted(cluster, func(c *apiv1.Cluster) { c.Spec.ReplicationSlots.HighAvailability.SlotPrefix = "other_" })
	})

	It("requires synchronizeReplicas to be disabled before its section is removed", func() {
		cluster := storedCluster(ctx, 17)
		accepted(cluster, func(c *apiv1.Cluster) {
			c.Spec.ReplicationSlots.SynchronizeReplicas = &apiv1.SynchronizeReplicasConfiguration{Enabled: ptr.To(true)}
		})
		refused(cluster, func(c *apiv1.Cluster) { c.Spec.ReplicationSlots.SynchronizeReplicas = nil },
			"synchronizeReplicas cannot be removed while enabled")
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
		accepted(cluster, func(c *apiv1.Cluster) { c.Spec.ReplicationSlots.SynchronizeReplicas.Enabled = ptr.To(false) })
		accepted(cluster, func(c *apiv1.Cluster) { c.Spec.ReplicationSlots.SynchronizeReplicas = nil })
	})

	It("freezes the replica role while a switch to replica cluster is in progress", func() {
		cluster := storedCluster(ctx, 17)
		cluster.Status.SwitchReplicaClusterStatus.InProgress = true
		Expect(k8sClient.Status().Update(ctx, cluster)).To(Succeed())
		refused(cluster, func(c *apiv1.Cluster) {
			c.Spec.ReplicaCluster = &apiv1.ReplicaClusterConfiguration{Enabled: ptr.To(true), Source: "up"}
		}, "the replica role cannot change while the switch to replica cluster is in progress")

		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
		accepted(cluster, func(c *apiv1.Cluster) { c.Spec.PostgresConfiguration.Parameters["max_connections"] = "120" })
	})
})
