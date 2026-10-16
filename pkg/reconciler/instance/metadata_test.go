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

package instance

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/internal/scheme"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("object metadata test", func() {
	Context("updateRoleLabelsOnPods", func() {
		It("Should update the role labels correctly", func() {
			cluster := &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					CurrentPrimary: "primaryPod",
					TargetPrimary:  "primaryPod",
				},
			}

			primaryPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "primaryPod",
					Labels: map[string]string{},
				},
			}

			replicaPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "replicaPod",
					Labels: map[string]string{},
				},
			}

			updated := updateRoleLabels(context.Background(), cluster, primaryPod)
			Expect(updated).To(BeTrue())

			//nolint:staticcheck
			Expect(primaryPod.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))
			Expect(primaryPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))

			updated = updateRoleLabels(context.Background(), cluster, replicaPod)
			Expect(updated).To(BeTrue())
			//nolint:staticcheck
			Expect(replicaPod.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelReplica))
			Expect(replicaPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelReplica))
		})

		//nolint: dupl
		It("Should update the role labels when the primary and the replica switch roles", func() {
			cluster := &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					CurrentPrimary: "newPrimaryPod",
					TargetPrimary:  "newPrimaryPod",
				},
			}

			newPrimaryPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "newPrimaryPod",
					Labels: map[string]string{
						//nolint:staticcheck
						utils.ClusterRoleLabelName:         specs.ClusterRoleLabelReplica,
						utils.ClusterInstanceRoleLabelName: specs.ClusterRoleLabelReplica,
					},
				},
			}

			newReplicaPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "newReplicaPod",
					Labels: map[string]string{
						//nolint:staticcheck
						utils.ClusterRoleLabelName:         specs.ClusterRoleLabelPrimary,
						utils.ClusterInstanceRoleLabelName: specs.ClusterRoleLabelPrimary,
					},
				},
			}

			updated := updateRoleLabels(context.Background(), cluster, newPrimaryPod)
			Expect(updated).To(BeTrue())
			//nolint:staticcheck
			Expect(newPrimaryPod.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))
			Expect(newPrimaryPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))

			updated = updateRoleLabels(context.Background(), cluster, newReplicaPod)
			Expect(updated).To(BeTrue())
			//nolint:staticcheck
			Expect(newReplicaPod.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelReplica))
			Expect(newReplicaPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelReplica))
		})

		It("Should not perform role reconciliation when there is no current primary", func() {
			cluster := &apiv1.Cluster{}

			oldPrimaryPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "oldPrimaryPod",
					Labels: map[string]string{
						utils.ClusterInstanceRoleLabelName: specs.ClusterRoleLabelPrimary,
					},
				},
			}

			oldReplicaPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "oldReplicaPod",
					Labels: map[string]string{
						utils.ClusterInstanceRoleLabelName: specs.ClusterRoleLabelReplica,
					},
				},
			}

			updated := updateRoleLabels(context.Background(), cluster, oldPrimaryPod)
			Expect(updated).To(BeFalse())
			//nolint:staticcheck
			Expect(oldPrimaryPod.Labels).ToNot(ContainElement(utils.ClusterRoleLabelName))
			Expect(oldPrimaryPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))

			updated = updateRoleLabels(context.Background(), cluster, oldReplicaPod)
			Expect(updated).To(BeFalse())
			//nolint:staticcheck
			Expect(oldReplicaPod.Labels).ToNot(ContainElement(utils.ClusterRoleLabelName))
			Expect(oldReplicaPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelReplica))
		})

		//nolint: dupl
		It("should not perform any changes if everything is ok", func() {
			cluster := &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					CurrentPrimary: "primaryPod",
					TargetPrimary:  "primaryPod",
				},
			}

			primaryPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "primaryPod",
					Labels: map[string]string{
						//nolint:staticcheck
						utils.ClusterRoleLabelName:         specs.ClusterRoleLabelPrimary,
						utils.ClusterInstanceRoleLabelName: specs.ClusterRoleLabelPrimary,
					},
				},
			}

			replicaPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "replicaPod",
					Labels: map[string]string{
						//nolint:staticcheck
						utils.ClusterRoleLabelName:         specs.ClusterRoleLabelReplica,
						utils.ClusterInstanceRoleLabelName: specs.ClusterRoleLabelReplica,
					},
				},
			}

			updated := updateRoleLabels(context.Background(), cluster, primaryPod)
			Expect(updated).To(BeFalse())
			//nolint:staticcheck
			Expect(primaryPod.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))
			Expect(primaryPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))

			updated = updateRoleLabels(context.Background(), cluster, replicaPod)
			Expect(updated).To(BeFalse())
			//nolint:staticcheck
			Expect(replicaPod.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelReplica))
			Expect(replicaPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelReplica))
		})

		It("should update existing instances with the old role label", func() {
			cluster := &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					CurrentPrimary: "primaryPod",
					TargetPrimary:  "primaryPod",
				},
			}

			primaryPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "primaryPod",
					Labels: map[string]string{
						//nolint:staticcheck
						utils.ClusterRoleLabelName: specs.ClusterRoleLabelPrimary,
					},
				},
			}

			replicaPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "replicaPod",
					Labels: map[string]string{
						//nolint:staticcheck
						utils.ClusterRoleLabelName: specs.ClusterRoleLabelReplica,
					},
				},
			}

			updated := updateRoleLabels(context.Background(), cluster, primaryPod)
			Expect(updated).To(BeTrue())
			//nolint:staticcheck
			Expect(primaryPod.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))
			Expect(primaryPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))

			updated = updateRoleLabels(context.Background(), cluster, replicaPod)
			Expect(updated).To(BeTrue())
			//nolint:staticcheck
			Expect(replicaPod.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelReplica))
			Expect(replicaPod.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelReplica))
		})
	})

	Context("updateOperatorLabelsOnInstances", func() {
		const instanceName = "instance1"
		It("Should create labels if the instance has no labels", func() {
			instance := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: instanceName,
				},
			}

			updated := updateOperatorLabels(context.Background(), instance)
			Expect(updated).To(BeTrue())
			Expect(instance.Labels[utils.PodRoleLabelName]).To(Equal(string(utils.PodRoleInstance)))
		})

		It("Should not update labels if the instance already has the correct labels", func() {
			labels := map[string]string{
				utils.InstanceNameLabelName: instanceName,
				utils.PodRoleLabelName:      string(utils.PodRoleInstance),
			}
			instance := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:   instanceName,
					Labels: labels,
				},
			}

			updated := updateOperatorLabels(context.Background(), instance)
			Expect(updated).To(BeFalse())
			Expect(instance.Labels).To(Equal(labels))
		})

		It("Should update name label if it's incorrect", func() {
			instance := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: instanceName,
					Labels: map[string]string{
						utils.InstanceNameLabelName: "incorrectName",
						utils.PodRoleLabelName:      string(utils.PodRoleInstance),
					},
				},
			}

			updated := updateOperatorLabels(context.Background(), instance)
			Expect(updated).To(BeTrue())
			Expect(instance.Labels[utils.InstanceNameLabelName]).To(Equal(instanceName))
		})

		It("Should update role label if it's incorrect", func() {
			instance := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: instanceName,
					Labels: map[string]string{
						utils.InstanceNameLabelName: instanceName,
						utils.PodRoleLabelName:      "incorrectRole",
					},
				},
			}

			updated := updateOperatorLabels(context.Background(), instance)
			Expect(updated).To(BeTrue())
			Expect(instance.Labels[utils.PodRoleLabelName]).To(Equal(string(utils.PodRoleInstance)))
		})

		Context("updateClusterLabelsOnPods", func() {
			const (
				labelKey      = "label1"
				labelValue    = "value1"
				labelKeyTwo   = "label2"
				labelValueTwo = "value2"
			)

			It("Should correctly add missing labels from cluster to pods", func() {
				cluster := &apiv1.Cluster{
					Spec: apiv1.ClusterSpec{
						InheritedMetadata: &apiv1.EmbeddedObjectMetadata{
							Labels: map[string]string{
								labelKey:    labelValue,
								labelKeyTwo: labelValueTwo,
							},
						},
					},
				}

				pods := corev1.PodList{
					Items: []corev1.Pod{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod1",
								Labels: map[string]string{
									labelKey: labelValue,
								},
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "pod2",
							},
						},
					},
				}

				for idx := range pods.Items {
					updated := updateClusterLabels(context.Background(), cluster, &pods.Items[idx])
					Expect(updated).To(BeTrue())
				}

				for _, pod := range pods.Items {
					Expect(pod.Labels).To(Equal(cluster.GetFixedInheritedLabels()))
				}
			})

			It("Should not change labels if they already match the cluster's", func() {
				cluster := &apiv1.Cluster{
					Spec: apiv1.ClusterSpec{
						InheritedMetadata: &apiv1.EmbeddedObjectMetadata{
							Labels: map[string]string{
								labelKey:    labelValue,
								labelKeyTwo: labelValueTwo,
							},
						},
					},
				}
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
						Labels: map[string]string{
							labelKey:    labelValue,
							labelKeyTwo: labelValueTwo,
						},
					},
				}

				Expect(cluster.Spec.InheritedMetadata.Labels).To(Equal(cluster.GetFixedInheritedLabels()))

				updated := updateClusterLabels(context.Background(), cluster, pod)
				Expect(updated).To(BeFalse())
				Expect(pod.Labels).To(Equal(cluster.GetFixedInheritedLabels()))
			})

			It("Should correctly handle the case of no fixed inherited labels from the cluster", func() {
				cluster := &apiv1.Cluster{
					Spec: apiv1.ClusterSpec{
						InheritedMetadata: &apiv1.EmbeddedObjectMetadata{},
					},
				}
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}}

				updated := updateClusterLabels(context.Background(), cluster, pod)
				Expect(updated).To(BeFalse())
				Expect(pod.Labels).To(BeEmpty())
			})
		})

		Context("updateClusterAnnotationsOnPods", func() {
			const key = "annotation1"
			const value = "value1"

			It("Should correctly add missing annotations from cluster to pods", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
					},
				}

				cluster := &apiv1.Cluster{
					Spec: apiv1.ClusterSpec{
						InheritedMetadata: &apiv1.EmbeddedObjectMetadata{
							Annotations: map[string]string{key: value},
						},
					},
				}
				updated := updateClusterAnnotations(context.Background(), cluster, pod)
				Expect(updated).To(BeTrue())

				Expect(pod.Annotations[key]).To(Equal(value))
			})

			It("Should not change annotations if they already match the cluster's", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
						Annotations: map[string]string{
							key:                                 value,
							utils.OperatorVersionAnnotationName: versions.Version,
						},
					},
				}

				cluster := &apiv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{},
					Spec: apiv1.ClusterSpec{
						InheritedMetadata: &apiv1.EmbeddedObjectMetadata{
							Annotations: map[string]string{key: value},
						},
					},
				}

				expectedAnnotations := cluster.GetFixedInheritedAnnotations()
				expectedAnnotations[utils.OperatorVersionAnnotationName] = versions.Version

				Expect(expectedAnnotations).To(Equal(cluster.GetFixedInheritedAnnotations()))

				updated := updateClusterAnnotations(context.Background(), cluster, pod)
				Expect(updated).To(BeFalse())
				Expect(pod.Annotations).To(HaveLen(2))
				Expect(pod.Annotations[key]).To(Equal(value))
				Expect(pod.Annotations[utils.OperatorVersionAnnotationName]).To(Equal(versions.Version))
			})

			It("Should correctly add AppArmor annotations if present in the cluster's annotations", func() {
				const (
					key   = utils.AppArmorAnnotationPrefix + "/postgres"
					value = "runtime/default"
				)

				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
					},
					Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "postgres"}}},
				}

				cluster := &apiv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{key: value},
					},
				}

				updated := updateClusterAnnotations(context.Background(), cluster, pod)
				Expect(updated).To(BeTrue())
				Expect(pod.Annotations[key]).To(Equal(value))
			})

			It("Should correctly handle the case of no fixed inherited annotations from the cluster", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod1",
						Annotations: map[string]string{
							utils.OperatorVersionAnnotationName: versions.Version,
						},
					},
				}
				cluster := &apiv1.Cluster{}

				updated := updateClusterAnnotations(context.Background(), cluster, pod)
				Expect(updated).To(BeFalse())
				Expect(pod.Annotations).To(HaveLen(1))
			})
		})
	})
})

var _ = Describe("metadata reconciliation test", func() {
	Context("ReconcileMetadata", func() {
		It("Should update all pods metadata successfully", func() {
			instances := []corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "pod1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "pod2"}},
			}

			cluster := &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					CurrentPrimary: "pod1",
					TargetPrimary:  "pod1",
				},
				Spec: apiv1.ClusterSpec{
					InheritedMetadata: &apiv1.EmbeddedObjectMetadata{
						Labels:      map[string]string{"label1": "value1"},
						Annotations: map[string]string{"annotation1": "value1"},
					},
				},
			}

			cli := fake.NewClientBuilder().
				WithScheme(scheme.BuildWithAllKnownScheme()).
				WithObjects(&instances[0], &instances[1]).
				Build()

			err := ReconcileMetadata(context.Background(), cli, cluster, instances)
			Expect(err).ToNot(HaveOccurred())

			var updatedInstanceList corev1.PodList
			err = cli.List(context.Background(), &updatedInstanceList)
			Expect(err).ToNot(HaveOccurred())
			Expect(updatedInstanceList.Items).To(HaveLen(len(instances)))

			for _, pod := range updatedInstanceList.Items {
				Expect(pod.Labels[utils.PodRoleLabelName]).To(Equal(string(utils.PodRoleInstance)))
				Expect(pod.Labels[utils.InstanceNameLabelName]).To(Equal(pod.Name))
				//nolint:staticcheck
				Expect(pod.Labels[utils.ClusterRoleLabelName]).To(Or(Equal(specs.ClusterRoleLabelPrimary),
					Equal(specs.ClusterRoleLabelReplica)))
				Expect(pod.Labels[utils.ClusterInstanceRoleLabelName]).To(Or(Equal(specs.ClusterRoleLabelPrimary),
					Equal(specs.ClusterRoleLabelReplica)))
				Expect(pod.Labels["label1"]).To(Equal("value1"))
				Expect(pod.Annotations["annotation1"]).To(Equal("value1"))
			}
		})
	})
})

var _ = Describe("metadata update functions", func() {
	Context("Given nil labels or annotations in the pod", func() {
		var (
			ctx      context.Context
			cluster  *apiv1.Cluster
			instance *corev1.Pod
		)

		BeforeEach(func() {
			ctx = context.Background()
			cluster = &apiv1.Cluster{
				Status: apiv1.ClusterStatus{
					CurrentPrimary: "pod1",
					TargetPrimary:  "pod1",
				},
				Spec: apiv1.ClusterSpec{
					InheritedMetadata: &apiv1.EmbeddedObjectMetadata{
						Labels: map[string]string{
							"label1": "value1",
						},
						Annotations: map[string]string{
							"annotation1": "value1",
						},
					},
				},
			}

			instance = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "pod1",
					Labels:      nil,
					Annotations: nil,
				},
			}
		})

		It("Should updateRoleLabels correctly", func() {
			modified := updateRoleLabels(ctx, cluster, instance)
			Expect(modified).To(BeTrue())
			//nolint:staticcheck
			Expect(instance.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))
			Expect(instance.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelPrimary))
		})

		It("Should updateOperatorLabels correctly", func() {
			modified := updateOperatorLabels(ctx, instance)
			Expect(modified).To(BeTrue())
			Expect(instance.Labels).To(Equal(map[string]string{
				utils.PodRoleLabelName:      string(utils.PodRoleInstance),
				utils.InstanceNameLabelName: "pod1",
			}))
		})

		It("Should updateClusterLabels correctly", func() {
			modified := updateClusterLabels(ctx, cluster, instance)
			Expect(modified).To(BeTrue())
			Expect(instance.Labels).To(Equal(cluster.Spec.InheritedMetadata.Labels))
		})

		It("Should updateClusterAnnotations correctly", func() {
			modified := updateClusterAnnotations(ctx, cluster, instance)
			Expect(modified).To(BeTrue())
			Expect(instance.Annotations).To(Equal(cluster.GetFixedInheritedAnnotations()))
		})
	})
})

var _ = Describe("updateRoleLabels during a pending failover", func() {
	It("should not re-promote an old primary already marked unhealthy while a failover is pending", func() {
		cluster := &apiv1.Cluster{
			Status: apiv1.ClusterStatus{
				CurrentPrimary: "old-primary",
				TargetPrimary:  apiv1.PendingFailoverMarker,
			},
		}

		oldPrimary := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "old-primary",
				Labels: map[string]string{
					//nolint:staticcheck
					utils.ClusterRoleLabelName:         specs.ClusterRoleLabelUnhealthy,
					utils.ClusterInstanceRoleLabelName: specs.ClusterRoleLabelUnhealthy,
				},
			},
		}

		updateRoleLabels(context.Background(), cluster, oldPrimary)

		//nolint:staticcheck
		Expect(oldPrimary.Labels[utils.ClusterRoleLabelName]).To(Equal(specs.ClusterRoleLabelUnhealthy))
		Expect(oldPrimary.Labels[utils.ClusterInstanceRoleLabelName]).To(Equal(specs.ClusterRoleLabelUnhealthy))
	})
})

// The instance Pods are updated in place with the metadata of their kind:
// objectMetadata.pgPods and the DEFAULT_* and MANDATORY_* settings for `pgPods`,
// `allPods` and `all`. A wrong
// up-to-date check would patch the Pods at every reconciliation, or never.
var _ = Describe("metadata of the instance Pods, reconciled in place", func() {
	var (
		previous *configuration.Data
		cluster  *apiv1.Cluster
		cli      ctrlclient.Client
	)

	reconcileAndGet := func() corev1.Pod {
		var pods corev1.PodList
		Expect(cli.List(context.Background(), &pods)).To(Succeed())
		Expect(ReconcileMetadata(context.Background(), cli, cluster, pods.Items)).To(Succeed())
		var pod corev1.Pod
		Expect(cli.Get(context.Background(), ctrlclient.ObjectKey{Name: "payments-1"}, &pod)).To(Succeed())
		return pod
	}

	BeforeEach(func() {
		previous = configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })

		cluster = &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "payments"},
			Status:     apiv1.ClusterStatus{CurrentPrimary: "payments-1", TargetPrimary: "payments-1"},
			Spec: apiv1.ClusterSpec{
				InheritedMetadata: &apiv1.EmbeddedObjectMetadata{
					Labels: map[string]string{"team": "payments"},
				},
				MetadataPropagation: []apiv1.MetadataPropagationRule{
					{
						Targets:     []apiv1.MetadataPropagationTarget{"pgPods"},
						Annotations: map[string]string{"vault.hashicorp.com/agent-inject": "true"},
					},
					{
						Targets:     []apiv1.MetadataPropagationTarget{"services"},
						Annotations: map[string]string{"external-dns.kubernetes.io/hostname": "db.example.com"},
					},
					{
						Targets: []apiv1.MetadataPropagationTarget{"persistentVolumeClaims"},
						Labels:  map[string]string{"velero.io/exclude-from-backup": "true"},
					},
				},
			},
		}
		cli = fake.NewClientBuilder().
			WithScheme(scheme.BuildWithAllKnownScheme()).
			WithObjects(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:   "payments-1",
				Labels: map[string]string{"third-party": "kept"},
			}}).
			Build()
	})

	It("adds the Pods' metadata, and nothing of the other kinds", func() {
		pod := reconcileAndGet()
		Expect(pod.Labels).To(HaveKeyWithValue("team", "payments"))
		Expect(pod.Labels).To(HaveKeyWithValue("third-party", "kept"))
		Expect(pod.Annotations).To(HaveKeyWithValue("vault.hashicorp.com/agent-inject", "true"))
		Expect(pod.Annotations).NotTo(HaveKey("external-dns.kubernetes.io/hostname"))
		Expect(pod.Labels).NotTo(HaveKey("velero.io/exclude-from-backup"))
	})

	It("does not patch the Pod again when it is up to date", func() {
		configuration.Current.DefaultMetadata = `[{"targets": ["all"], "annotations": {"example.com/mesh": "off"}}]`
		configuration.Current.MandatoryMetadata = `[{"targets": ["pgPods"], "labels": {"cost-centre": "dbaas"}}]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())

		first := reconcileAndGet()
		second := reconcileAndGet()
		Expect(second.ResourceVersion).To(Equal(first.ResourceVersion))
	})

	It("follows a change of the default and mandatory settings", func() {
		configuration.Current.DefaultMetadata = `[` +
			`{"targets": ["pgPods"], "annotations": {"vault.hashicorp.com/agent-inject": "false"}}` +
			`]`
		configuration.Current.MandatoryMetadata = `[` +
			`{"targets": ["pgPods"], "labels": {"cost-centre": "dbaas"}},` +
			`{"targets": ["pgPods"], "labels": {"team": "platform"}}` +
			`]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		pod := reconcileAndGet()
		Expect(pod.Labels).To(HaveKeyWithValue("cost-centre", "dbaas"))
		Expect(pod.Labels).To(HaveKeyWithValue("team", "platform"))
		// The Cluster's annotation wins over the default
		Expect(pod.Annotations).To(HaveKeyWithValue("vault.hashicorp.com/agent-inject", "true"))
		configuration.Current.MandatoryMetadata = `[{"targets": ["pgPods"], "labels": {"cost-centre": "finance"}}]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		pod = reconcileAndGet()
		Expect(pod.Labels).To(HaveKeyWithValue("cost-centre", "finance"))
		// As for inheritedMetadata, a key no longer set is not removed, and
		// the Cluster's value comes back where the mandatory one was
		Expect(pod.Labels).To(HaveKeyWithValue("team", "payments"))
	})

	It("follows a change of the Pods' own metadata on a Pod already reconciled", func() {
		reconcileAndGet()

		// Only the Pods' metadata changes: the rest of the Pod is up to date
		cluster.Spec.MetadataPropagation[0].Annotations["vault.hashicorp.com/agent-inject"] = "false"
		cluster.Spec.MetadataPropagation[0].Labels = map[string]string{"tier": "database"}
		pod := reconcileAndGet()
		Expect(pod.Annotations).To(HaveKeyWithValue("vault.hashicorp.com/agent-inject", "false"))
		Expect(pod.Labels).To(HaveKeyWithValue("tier", "database"))
	})

	It("ignores the settings for the other kinds of objects", func() {
		configuration.Current.MandatoryMetadata = `[{` +
			`"targets": ["services", "persistentVolumeClaims", "poolerPods", "majorUpgradePods"], ` +
			`"labels": {"cost-centre": "dbaas"}}]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		pod := reconcileAndGet()
		Expect(pod.Labels).NotTo(HaveKey("cost-centre"))
	})

	It("adds the settings for every kind of Pod, below those of the PostgreSQL Pods", func() {
		configuration.Current.MandatoryMetadata = `[` +
			`{"targets": ["allPods"], "labels": {"bla": "pods"}},` +
			`{"targets": ["pgPods"], "labels": {"bla": "1"}},` +
			`{"targets": ["allPods"], "labels": {"mesh": "pods"}}` +
			`]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		pod := reconcileAndGet()
		Expect(pod.Labels).To(HaveKeyWithValue("bla", "1"))
		Expect(pod.Labels).To(HaveKeyWithValue("mesh", "pods"))

		// Dropping the PostgreSQL Pods' entry brings the value of allPods
		configuration.Current.MandatoryMetadata = `[{"targets": ["allPods"], "labels": {"bla": "pods"}}]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		pod = reconcileAndGet()
		Expect(pod.Labels).To(HaveKeyWithValue("bla", "pods"))
	})
})
