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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("cluster default configuration", func() {
	It("defaults to creating an application database", func() {
		cluster := Cluster{}
		cluster.Default()
		Expect(cluster.Spec.Bootstrap.InitDB.Database).To(Equal("app"))
		Expect(cluster.Spec.Bootstrap.InitDB.Owner).To(Equal("app"))
	})

	It("defaults the owner user with the database name", func() {
		cluster := Cluster{
			Spec: ClusterSpec{
				Bootstrap: &BootstrapConfiguration{
					InitDB: &BootstrapInitDB{
						Database: "appdb",
					},
				},
			},
		}

		cluster.Default()
		Expect(cluster.Spec.Bootstrap.InitDB.Owner).To(Equal("appdb"))
	})

	It("defaults to create an application database if recovery is used", func() {
		cluster := Cluster{
			Spec: ClusterSpec{
				Bootstrap: &BootstrapConfiguration{
					Recovery: &BootstrapRecovery{},
				},
			},
		}
		cluster.Default()
		Expect(cluster.ShouldRecoveryCreateApplicationDatabase()).Should(BeTrue())
		Expect(cluster.Spec.Bootstrap.Recovery.Database).ShouldNot(BeEmpty())
		Expect(cluster.Spec.Bootstrap.Recovery.Owner).ShouldNot(BeEmpty())
		Expect(cluster.Spec.Bootstrap.Recovery.Secret).Should(BeNil())
	})

	It("defaults the owner user with the database name for recovery", func() {
		cluster := Cluster{
			Spec: ClusterSpec{
				Bootstrap: &BootstrapConfiguration{
					Recovery: &BootstrapRecovery{
						Database: "appdb",
					},
				},
			},
		}

		cluster.Default()
		Expect(cluster.Spec.Bootstrap.Recovery.Owner).To(Equal("appdb"))
	})

	It("defaults to create an application database if pg_basebackup is used", func() {
		cluster := Cluster{
			Spec: ClusterSpec{
				Bootstrap: &BootstrapConfiguration{
					PgBaseBackup: &BootstrapPgBaseBackup{},
				},
			},
		}
		cluster.Default()
		Expect(cluster.ShouldPgBaseBackupCreateApplicationDatabase()).Should(BeTrue())
		Expect(cluster.Spec.Bootstrap.PgBaseBackup.Database).ShouldNot(BeEmpty())
		Expect(cluster.Spec.Bootstrap.PgBaseBackup.Owner).ShouldNot(BeEmpty())
		Expect(cluster.Spec.Bootstrap.PgBaseBackup.Secret).Should(BeNil())
	})

	It("defaults the owner user with the database name for pg_basebackup", func() {
		cluster := Cluster{
			Spec: ClusterSpec{
				Bootstrap: &BootstrapConfiguration{
					PgBaseBackup: &BootstrapPgBaseBackup{
						Database: "appdb",
					},
				},
			},
		}

		cluster.Default()
		Expect(cluster.Spec.Bootstrap.PgBaseBackup.Owner).To(Equal("appdb"))
	})

	It("defaults the PostgreSQL configuration with parameters from the operator", func() {
		cluster := Cluster{}
		cluster.Default()
		Expect(cluster.Spec.PostgresConfiguration.Parameters).ToNot(BeEmpty())
	})

	It("defaults the anti-affinity", func() {
		cluster := Cluster{
			Spec: ClusterSpec{
				Affinity: AffinityConfiguration{},
			},
		}
		cluster.Default()
		Expect(cluster.Spec.Affinity.PodAntiAffinityType).To(BeEquivalentTo(PodAntiAffinityTypePreferred))
		Expect(cluster.Spec.Affinity.EnablePodAntiAffinity).To(BeNil())
	})

	It("should fill the image name if isn't already set", func() {
		cluster := Cluster{}
		cluster.Default()
		Expect(cluster.Spec.ImageName).To(Equal(configuration.Current.PostgresImageName))
	})

	It("shouldn't set the image name if already present", func() {
		cluster := Cluster{
			Spec: ClusterSpec{
				ImageName: "test:13",
			},
		}
		cluster.Default()
		Expect(cluster.Spec.ImageName).To(Equal("test:13"))
	})

	It("should setup the application database name", func() {
		cluster := Cluster{}
		cluster.Default()
		Expect(cluster.Spec.Bootstrap.InitDB.Database).To(Equal("app"))
		Expect(cluster.Spec.Bootstrap.InitDB.Owner).To(Equal("app"))
	})

	It("should set the owner name as the database name", func() {
		cluster := Cluster{
			Spec: ClusterSpec{
				Bootstrap: &BootstrapConfiguration{
					InitDB: &BootstrapInitDB{
						Database: "test",
					},
				},
			},
		}
		cluster.Default()
		Expect(cluster.Spec.Bootstrap.InitDB.Database).To(Equal("test"))
		Expect(cluster.Spec.Bootstrap.InitDB.Owner).To(Equal("test"))
	})

	It("should not overwrite application database and owner settings", func() {
		cluster := Cluster{
			Spec: ClusterSpec{
				Bootstrap: &BootstrapConfiguration{
					InitDB: &BootstrapInitDB{
						Database: "testdb",
						Owner:    "testuser",
					},
				},
			},
		}
		cluster.Default()
		Expect(cluster.Spec.Bootstrap.InitDB.Database).To(Equal("testdb"))
		Expect(cluster.Spec.Bootstrap.InitDB.Owner).To(Equal("testuser"))
	})
})

var _ = Describe("Default monitoring queries", func() {
	It("correctly set the default monitoring queries configmap and secret when none is already specified", func() {
		cluster := &Cluster{}
		cluster.defaultMonitoringQueries(&configuration.Data{
			MonitoringQueriesSecret:    "test-secret",
			MonitoringQueriesConfigmap: "test-configmap",
		})
		Expect(cluster.Spec.Monitoring).NotTo(BeNil())
		Expect(cluster.Spec.Monitoring.CustomQueriesConfigMap).NotTo(BeEmpty())
		Expect(cluster.Spec.Monitoring.CustomQueriesConfigMap).
			To(ContainElement(ConfigMapKeySelector{
				LocalObjectReference: LocalObjectReference{Name: DefaultMonitoringConfigMapName},
				Key:                  DefaultMonitoringKey,
			}))
		Expect(cluster.Spec.Monitoring.CustomQueriesSecret).NotTo(BeEmpty())
		Expect(cluster.Spec.Monitoring.CustomQueriesSecret).
			To(ContainElement(SecretKeySelector{
				LocalObjectReference: LocalObjectReference{Name: DefaultMonitoringSecretName},
				Key:                  DefaultMonitoringKey,
			}))
	})
	testCluster := &Cluster{Spec: ClusterSpec{Monitoring: &MonitoringConfiguration{
		CustomQueriesConfigMap: []ConfigMapKeySelector{
			{
				LocalObjectReference: LocalObjectReference{Name: DefaultMonitoringConfigMapName},
				Key:                  "test2",
			},
		},
		CustomQueriesSecret: []SecretKeySelector{
			{
				LocalObjectReference: LocalObjectReference{Name: DefaultMonitoringConfigMapName},
				Key:                  "test3",
			},
		},
	}}}
	It("correctly set the default monitoring queries configmap when other metrics are already specified", func() {
		modifiedCluster := testCluster.DeepCopy()
		modifiedCluster.defaultMonitoringQueries(&configuration.Data{
			MonitoringQueriesConfigmap: "test-configmap",
		})

		Expect(modifiedCluster.Spec.Monitoring).NotTo(BeNil())
		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesConfigMap).NotTo(BeEmpty())
		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesSecret).NotTo(BeEmpty())
		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesConfigMap).
			To(ContainElement(ConfigMapKeySelector{
				LocalObjectReference: LocalObjectReference{Name: DefaultMonitoringConfigMapName},
				Key:                  "test2",
			}))

		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesSecret).
			To(BeEquivalentTo(testCluster.Spec.Monitoring.CustomQueriesSecret))
		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesConfigMap).
			To(ContainElements(testCluster.Spec.Monitoring.CustomQueriesConfigMap))
	})
	It("correctly set the default monitoring queries secret when other metrics are already specified", func() {
		modifiedCluster := testCluster.DeepCopy()
		modifiedCluster.defaultMonitoringQueries(&configuration.Data{
			MonitoringQueriesSecret: "test-secret",
		})

		Expect(modifiedCluster.Spec.Monitoring).NotTo(BeNil())
		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesSecret).NotTo(BeEmpty())
		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesConfigMap).NotTo(BeEmpty())
		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesSecret).
			To(ContainElement(SecretKeySelector{
				LocalObjectReference: LocalObjectReference{Name: DefaultMonitoringSecretName},
				Key:                  "test3",
			}))

		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesConfigMap).
			To(BeEquivalentTo(testCluster.Spec.Monitoring.CustomQueriesConfigMap))
		Expect(modifiedCluster.Spec.Monitoring.CustomQueriesSecret).
			To(ContainElements(testCluster.Spec.Monitoring.CustomQueriesSecret))
	})
})

var _ = Describe("setDefaultPlugins", func() {
	It("adds pre-defined plugins if not already present", func() {
		cluster := &Cluster{
			Spec: ClusterSpec{
				Plugins: []PluginConfiguration{
					{Name: "existing-plugin", Enabled: ptr.To(true)},
				},
			},
		}
		config := &configuration.Data{
			IncludePlugins: "predefined-plugin1,predefined-plugin2",
		}

		cluster.setDefaultPlugins(config)

		Expect(cluster.Spec.Plugins).To(
			ContainElement(PluginConfiguration{Name: "existing-plugin", Enabled: ptr.To(true)}))
		Expect(cluster.Spec.Plugins).To(
			ContainElement(PluginConfiguration{Name: "predefined-plugin1", Enabled: ptr.To(true)}))
		Expect(cluster.Spec.Plugins).To(
			ContainElement(PluginConfiguration{Name: "predefined-plugin2", Enabled: ptr.To(true)}))
	})

	It("does not add pre-defined plugins if already present", func() {
		cluster := &Cluster{
			Spec: ClusterSpec{
				Plugins: []PluginConfiguration{
					{Name: "predefined-plugin1", Enabled: ptr.To(false)},
				},
			},
		}
		config := &configuration.Data{
			IncludePlugins: "predefined-plugin1,predefined-plugin2",
		}

		cluster.setDefaultPlugins(config)

		Expect(cluster.Spec.Plugins).To(HaveLen(2))
		Expect(cluster.Spec.Plugins).To(
			ContainElement(PluginConfiguration{Name: "predefined-plugin1", Enabled: ptr.To(false)}))
		Expect(cluster.Spec.Plugins).To(
			ContainElement(PluginConfiguration{Name: "predefined-plugin2", Enabled: ptr.To(true)}))
	})

	It("handles empty plugin list gracefully", func() {
		cluster := &Cluster{}
		config := &configuration.Data{
			IncludePlugins: "predefined-plugin1",
		}

		cluster.setDefaultPlugins(config)

		Expect(cluster.Spec.Plugins).To(HaveLen(1))
		Expect(cluster.Spec.Plugins).To(
			ContainElement(PluginConfiguration{Name: "predefined-plugin1", Enabled: ptr.To(true)}))
	})
})

var _ = Describe("default dataDurability", func() {
	It("should default dataDurability to 'required' when synchronous is present", func() {
		cluster := &Cluster{
			Spec: ClusterSpec{
				PostgresConfiguration: PostgresConfiguration{
					Synchronous: &SynchronousReplicaConfiguration{},
				},
			},
		}
		cluster.SetDefaults()
		Expect(cluster.Spec.PostgresConfiguration.Synchronous).ToNot(BeNil())
		Expect(cluster.Spec.PostgresConfiguration.Synchronous.DataDurability).To(Equal(DataDurabilityLevelRequired))
	})

	It("should not touch synchronous if nil", func() {
		cluster := &Cluster{
			Spec: ClusterSpec{
				PostgresConfiguration: PostgresConfiguration{
					Synchronous: nil,
				},
			},
		}
		cluster.SetDefaults()
		Expect(cluster.Spec.PostgresConfiguration.Synchronous).To(BeNil())
	})

	It("should not change the dataDurability when set", func() {
		cluster := &Cluster{
			Spec: ClusterSpec{
				PostgresConfiguration: PostgresConfiguration{
					Synchronous: &SynchronousReplicaConfiguration{
						DataDurability: DataDurabilityLevelPreferred,
					},
				},
			},
		}
		cluster.SetDefaults()
		Expect(cluster.Spec.PostgresConfiguration.Synchronous).ToNot(BeNil())
		Expect(cluster.Spec.PostgresConfiguration.Synchronous.DataDurability).To(Equal(DataDurabilityLevelPreferred))
	})
})

var _ = Describe("NewLivenessPingerConfigFromAnnotations", func() {
	It("returns a nil configuration when annotation is not present", func() {
		annotations := map[string]string{}

		config, err := NewLivenessPingerConfigFromAnnotations(annotations)

		Expect(err).ToNot(HaveOccurred())
		Expect(config).To(BeNil())
	})

	It("returns an error when annotation contains invalid JSON", func() {
		annotations := map[string]string{
			utils.LivenessPingerAnnotationName: "{invalid_json",
		}

		config, err := NewLivenessPingerConfigFromAnnotations(annotations)

		Expect(err).To(HaveOccurred())
		Expect(config).To(BeNil())
	})

	It("applies default values when timeouts are not specified", func() {
		annotations := map[string]string{
			utils.LivenessPingerAnnotationName: `{"enabled": true}`,
		}

		config, err := NewLivenessPingerConfigFromAnnotations(annotations)

		Expect(err).ToNot(HaveOccurred())
		Expect(config).ToNot(BeNil())
		Expect(config.Enabled).To(HaveValue(BeTrue()))
		Expect(config.RequestTimeout).To(Equal(1000))
		Expect(config.ConnectionTimeout).To(Equal(1000))
	})

	It("preserves values when all fields are specified", func() {
		annotations := map[string]string{
			utils.LivenessPingerAnnotationName: `{"enabled": true, "requestTimeout": 300, "connectionTimeout": 600}`,
		}

		config, err := NewLivenessPingerConfigFromAnnotations(annotations)

		Expect(err).ToNot(HaveOccurred())
		Expect(config).ToNot(BeNil())
		Expect(config.Enabled).To(HaveValue(BeTrue()))
		Expect(config.RequestTimeout).To(Equal(300))
		Expect(config.ConnectionTimeout).To(Equal(600))
	})

	It("correctly sets enabled to false when specified", func() {
		annotations := map[string]string{
			utils.LivenessPingerAnnotationName: `{"enabled": false, "requestTimeout": 300, "connectionTimeout": 600}`,
		}

		config, err := NewLivenessPingerConfigFromAnnotations(annotations)

		Expect(err).ToNot(HaveOccurred())
		Expect(config).ToNot(BeNil())
		Expect(config.Enabled).To(HaveValue(BeFalse()))
		Expect(config.RequestTimeout).To(Equal(300))
		Expect(config.ConnectionTimeout).To(Equal(600))
	})

	It("correctly handles zero values for timeouts", func() {
		annotations := map[string]string{
			utils.LivenessPingerAnnotationName: `{"enabled": true, "requestTimeout": 0, "connectionTimeout": 0}`,
		}

		config, err := NewLivenessPingerConfigFromAnnotations(annotations)

		Expect(err).ToNot(HaveOccurred())
		Expect(config).ToNot(BeNil())
		Expect(config.RequestTimeout).To(Equal(1000))
		Expect(config.ConnectionTimeout).To(Equal(1000))
	})
})

var _ = Describe("probe defaults", func() {
	It("should set isolationCheck probe to true by default when no probes are specified", func() {
		cluster := &Cluster{}
		cluster.Default()
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck).ToNot(BeNil())
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.Enabled).To(HaveValue(BeTrue()))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.RequestTimeout).To(Equal(1000))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.ConnectionTimeout).To(Equal(1000))
	})

	It("should not override isolationCheck probe if already set", func() {
		cluster := &Cluster{
			Spec: ClusterSpec{
				Probes: &ProbesConfiguration{
					Liveness: &LivenessProbe{
						IsolationCheck: &IsolationCheckConfiguration{
							Enabled:           ptr.To(false),
							RequestTimeout:    300,
							ConnectionTimeout: 600,
						},
					},
				},
			},
		}
		cluster.Default()
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck).ToNot(BeNil())
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.Enabled).To(HaveValue(BeFalse()))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.RequestTimeout).To(Equal(300))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.ConnectionTimeout).To(Equal(600))
	})

	It("should set isolationCheck probe when it is not set but liveness probe is present", func() {
		cluster := &Cluster{
			Spec: ClusterSpec{
				Probes: &ProbesConfiguration{
					Liveness: &LivenessProbe{},
				},
			},
		}
		cluster.Default()
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck).ToNot(BeNil())
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.Enabled).To(HaveValue(BeTrue()))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.RequestTimeout).To(Equal(1000))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.ConnectionTimeout).To(Equal(1000))
	})

	It("should convert the existing annotations if set to true", func() {
		cluster := &Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					utils.LivenessPingerAnnotationName: `{"enabled": true, "requestTimeout": 300, "connectionTimeout": 600}`,
				},
			},
		}
		cluster.Default()
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck).ToNot(BeNil())
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.Enabled).To(HaveValue(BeTrue()))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.RequestTimeout).To(Equal(300))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.ConnectionTimeout).To(Equal(600))
	})

	It("should convert the existing annotations if set to false", func() {
		cluster := &Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					utils.LivenessPingerAnnotationName: `{"enabled": false, "requestTimeout": 300, "connectionTimeout": 600}`,
				},
			},
		}
		cluster.Default()
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck).ToNot(BeNil())
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.Enabled).To(HaveValue(BeFalse()))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.RequestTimeout).To(Equal(300))
		Expect(cluster.Spec.Probes.Liveness.IsolationCheck.ConnectionTimeout).To(Equal(600))
	})
})

var _ = Describe("failover quorum defaults", func() {
	clusterWithFailoverQuorumAnnotation := func(value string) *Cluster {
		return &Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					utils.FailoverQuorumAnnotationName: value,
				},
			},
			Spec: ClusterSpec{
				PostgresConfiguration: PostgresConfiguration{
					Synchronous: &SynchronousReplicaConfiguration{},
				},
			},
		}
	}

	It("should convert the annotation if present and set to true", func() {
		cluster := clusterWithFailoverQuorumAnnotation("t")
		cluster.Default()
		Expect(cluster.Spec.PostgresConfiguration.Synchronous.FailoverQuorum).To(BeTrue())
	})

	It("should convert the annotation if present and set to false", func() {
		cluster := clusterWithFailoverQuorumAnnotation("f")
		cluster.Spec.PostgresConfiguration.Synchronous.FailoverQuorum = true
		cluster.Default()
		Expect(cluster.Spec.PostgresConfiguration.Synchronous.FailoverQuorum).To(BeFalse())
	})

	It("should not convert the annotation if the value is wrong", func() {
		cluster := clusterWithFailoverQuorumAnnotation("toast")
		cluster.Spec.PostgresConfiguration.Synchronous.FailoverQuorum = true
		cluster.Default()
		Expect(cluster.Spec.PostgresConfiguration.Synchronous.FailoverQuorum).To(BeTrue())
	})

	It("should not convert the annotation if the synchronous replication stanza has not been set", func() {
		cluster := clusterWithFailoverQuorumAnnotation("t")
		cluster.Spec.PostgresConfiguration.Synchronous = nil

		// This would panic if the code tried to convert the annotation
		cluster.Default()
	})

	It("should not override the existing setting if the annotation has not been set", func() {
		cluster := &Cluster{
			Spec: ClusterSpec{
				PostgresConfiguration: PostgresConfiguration{
					Synchronous: &SynchronousReplicaConfiguration{
						FailoverQuorum: true,
					},
				},
			},
		}

		cluster.Default()
		Expect(cluster.Spec.PostgresConfiguration.Synchronous.FailoverQuorum).To(BeTrue())
	})
})

var _ = Describe("default storage class", func() {
	const defaultClass = "cnpg-ssd"

	withDefault := &configuration.Data{DefaultStorageClass: defaultClass}

	newCluster := func() *Cluster {
		return &Cluster{
			Spec: ClusterSpec{
				StorageConfiguration: StorageConfiguration{Size: "1Gi"},
				WalStorage:           &StorageConfiguration{Size: "1Gi"},
				Tablespaces: []TablespaceConfiguration{
					{Name: "first", Storage: StorageConfiguration{Size: "1Gi"}},
					{Name: "second", Storage: StorageConfiguration{Size: "1Gi"}},
				},
			},
		}
	}

	It("does nothing when the operator configuration sets no class", func() {
		cluster := newCluster()
		cluster.setDefaultStorageClass(&configuration.Data{})
		Expect(cluster.Spec.StorageConfiguration.StorageClass).To(BeNil())
		Expect(cluster.Spec.WalStorage.StorageClass).To(BeNil())
		Expect(cluster.Spec.Tablespaces[0].Storage.StorageClass).To(BeNil())
		Expect(cluster.Spec.Tablespaces[1].Storage.StorageClass).To(BeNil())
	})

	It("writes the class into every storage section that names none", func() {
		cluster := newCluster()
		cluster.setDefaultStorageClass(withDefault)
		Expect(cluster.Spec.StorageConfiguration.StorageClass).To(HaveValue(Equal(defaultClass)))
		Expect(cluster.Spec.WalStorage.StorageClass).To(HaveValue(Equal(defaultClass)))
		Expect(cluster.Spec.Tablespaces[0].Storage.StorageClass).To(HaveValue(Equal(defaultClass)))
		Expect(cluster.Spec.Tablespaces[1].Storage.StorageClass).To(HaveValue(Equal(defaultClass)))
	})

	It("leaves a Cluster without WAL storage and tablespaces as it is", func() {
		cluster := &Cluster{Spec: ClusterSpec{StorageConfiguration: StorageConfiguration{Size: "1Gi"}}}
		cluster.setDefaultStorageClass(withDefault)
		Expect(cluster.Spec.StorageConfiguration.StorageClass).To(HaveValue(Equal(defaultClass)))
		Expect(cluster.Spec.WalStorage).To(BeNil())
		Expect(cluster.Spec.Tablespaces).To(BeEmpty())
	})

	It("keeps the class set by the section", func() {
		cluster := newCluster()
		cluster.Spec.StorageConfiguration.StorageClass = ptr.To("data-class")
		cluster.Spec.WalStorage.StorageClass = ptr.To("wal-class")
		cluster.Spec.Tablespaces[1].Storage.StorageClass = ptr.To("tbs-class")
		cluster.setDefaultStorageClass(withDefault)
		Expect(cluster.Spec.StorageConfiguration.StorageClass).To(HaveValue(Equal("data-class")))
		Expect(cluster.Spec.WalStorage.StorageClass).To(HaveValue(Equal("wal-class")))
		Expect(cluster.Spec.Tablespaces[0].Storage.StorageClass).To(HaveValue(Equal(defaultClass)))
		Expect(cluster.Spec.Tablespaces[1].Storage.StorageClass).To(HaveValue(Equal("tbs-class")))
	})

	It("keeps the class set by the PVC template, an empty one included", func() {
		cluster := newCluster()
		cluster.Spec.StorageConfiguration.PersistentVolumeClaimTemplate = &corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To("template-class"),
		}
		cluster.Spec.WalStorage.PersistentVolumeClaimTemplate = &corev1.PersistentVolumeClaimSpec{
			StorageClassName: ptr.To(""),
		}
		cluster.Spec.Tablespaces[0].Storage.PersistentVolumeClaimTemplate = &corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		}
		cluster.setDefaultStorageClass(withDefault)
		Expect(cluster.Spec.StorageConfiguration.StorageClass).To(BeNil())
		Expect(cluster.Spec.WalStorage.StorageClass).To(BeNil())
		Expect(cluster.Spec.WalStorage.PersistentVolumeClaimTemplate.StorageClassName).To(HaveValue(Equal("")))
		Expect(cluster.Spec.Tablespaces[0].Storage.StorageClass).To(HaveValue(Equal(defaultClass)))
		Expect(cluster.Spec.Tablespaces[1].Storage.StorageClass).To(HaveValue(Equal(defaultClass)))
	})

	It("is idempotent", func() {
		cluster := newCluster()
		cluster.setDefaultStorageClass(withDefault)
		once := cluster.DeepCopy()
		cluster.setDefaultStorageClass(&configuration.Data{DefaultStorageClass: "another"})
		Expect(cluster.Spec).To(Equal(once.Spec))
	})

	Context("through the Cluster defaults", func() {
		BeforeEach(func() {
			current := configuration.Current
			config := *current
			config.DefaultStorageClass = defaultClass
			configuration.Current = &config
			DeferCleanup(func() { configuration.Current = current })
		})

		It("is not part of the webhook defaults, which run on updates too", func() {
			cluster := newCluster()
			cluster.Default()
			Expect(cluster.Spec.StorageConfiguration.StorageClass).To(BeNil())
			Expect(cluster.Spec.WalStorage.StorageClass).To(BeNil())
			Expect(cluster.Spec.Tablespaces[0].Storage.StorageClass).To(BeNil())
		})

		It("is applied by SetDefaultStorageClass", func() {
			cluster := newCluster()
			cluster.SetDefaultStorageClass()
			Expect(cluster.Spec.StorageConfiguration.StorageClass).To(HaveValue(Equal(defaultClass)))
			Expect(cluster.Spec.WalStorage.StorageClass).To(HaveValue(Equal(defaultClass)))
			Expect(cluster.Spec.Tablespaces[0].Storage.StorageClass).To(HaveValue(Equal(defaultClass)))
		})

		It("is part of the reconcile-time defaults while the Cluster has created no instance", func() {
			cluster := newCluster()
			cluster.SetDefaults()
			Expect(cluster.Spec.StorageConfiguration.StorageClass).To(HaveValue(Equal(defaultClass)))
			Expect(cluster.Spec.WalStorage.StorageClass).To(HaveValue(Equal(defaultClass)))
			Expect(cluster.Spec.Tablespaces[0].Storage.StorageClass).To(HaveValue(Equal(defaultClass)))
		})

		DescribeTable("is not part of the reconcile-time defaults once the Cluster has created an instance",
			func(status ClusterStatus) {
				cluster := newCluster()
				cluster.Status = status
				cluster.SetDefaults()
				Expect(cluster.Spec.StorageConfiguration.StorageClass).To(BeNil())
				Expect(cluster.Spec.WalStorage.StorageClass).To(BeNil())
				Expect(cluster.Spec.Tablespaces[0].Storage.StorageClass).To(BeNil())
			},
			Entry("target primary recorded, bootstrap in progress", ClusterStatus{
				TargetPrimary: "cluster-example-1",
				Conditions: []metav1.Condition{{
					Type:   string(ConditionInitialized),
					Status: metav1.ConditionFalse,
				}},
			}),
			Entry("initialized", ClusterStatus{
				TargetPrimary: "cluster-example-1",
				Conditions: []metav1.Condition{{
					Type:   string(ConditionInitialized),
					Status: metav1.ConditionTrue,
				}},
			}),
			Entry("status written by an operator version without the Initialized condition", ClusterStatus{
				LatestGeneratedNode: 1,
			}),
		)
	})
})
