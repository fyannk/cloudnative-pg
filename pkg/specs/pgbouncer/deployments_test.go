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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	config "github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	pgBouncerConfig "github.com/cloudnative-pg/cloudnative-pg/pkg/management/pgbouncer/config"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Deployment", func() {
	var (
		pooler  *apiv1.Pooler
		cluster *apiv1.Cluster
	)

	BeforeEach(func() {
		pooler = &apiv1.Pooler{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pooler",
				Namespace: "test-namespace",
			},
			Spec: apiv1.PoolerSpec{
				Cluster:   apiv1.LocalObjectReference{Name: "test-cluster"},
				Type:      apiv1.PoolerTypeRW,
				Instances: ptr.To(int32(1)),
				Template:  &apiv1.PodTemplateSpec{},
				PgBouncer: &apiv1.PgBouncerSpec{
					PoolMode:  apiv1.PgBouncerPoolModeSession,
					AuthQuery: "test",
				},
				DeploymentStrategy: &appsv1.DeploymentStrategy{
					Type: appsv1.RollingUpdateDeploymentStrategyType,
				},
				Monitoring: &apiv1.PoolerMonitoringConfiguration{
					EnablePodMonitor: true, //nolint:staticcheck
				},
			},
			Status: apiv1.PoolerStatus{
				Image: config.Current.PgbouncerImageName,
			},
		}

		cluster = &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-cluster",
				Namespace: "test-namespace",
			},
		}
	})

	It("creates a Deployment correctly", func() {
		deployment, err := Deployment(pooler, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())

		expectedHash, err := computeTemplateHash(pooler, config.Current.OperatorImageName)
		Expect(err).ShouldNot(HaveOccurred())

		// Check the computed hash
		Expect(deployment.ObjectMeta.Annotations[utils.PoolerSpecHashAnnotationName]).Should(Equal(expectedHash))

		// Check the metadata
		Expect(deployment.ObjectMeta.Name).To(Equal(pooler.Name))
		Expect(deployment.ObjectMeta.Namespace).To(Equal(pooler.Namespace))
		Expect(deployment.Labels).To(BeEquivalentTo(map[string]string{
			utils.ClusterLabelName:                cluster.Name,
			utils.PgbouncerNameLabel:              pooler.Name,
			utils.PodRoleLabelName:                string(utils.PodRolePooler),
			utils.KubernetesAppLabelName:          utils.AppName,
			utils.KubernetesAppInstanceLabelName:  cluster.Name,
			utils.KubernetesAppComponentLabelName: utils.PoolerComponentName,
			utils.KubernetesAppManagedByLabelName: utils.ManagerName,
		}))
		// Check the DeploymentSpec
		Expect(deployment.Spec.Replicas).To(Equal(pooler.Spec.Instances))
		Expect(deployment.Spec.Selector.MatchLabels[utils.PgbouncerNameLabel]).To(Equal(pooler.Name))

		// Check the PodTemplateSpec
		podTemplate := deployment.Spec.Template
		Expect(podTemplate.ObjectMeta.Annotations).To(Equal(pooler.Spec.Template.ObjectMeta.Annotations))
		Expect(podTemplate.Labels).To(BeEquivalentTo(map[string]string{
			utils.ClusterLabelName:                cluster.Name,
			utils.PgbouncerNameLabel:              pooler.Name,
			utils.PodRoleLabelName:                string(utils.PodRolePooler),
			utils.KubernetesAppLabelName:          utils.AppName,
			utils.KubernetesAppInstanceLabelName:  cluster.Name,
			utils.KubernetesAppComponentLabelName: utils.PoolerComponentName,
			utils.KubernetesAppManagedByLabelName: utils.ManagerName,
		}))

		// Check the containers
		Expect(podTemplate.Spec.Containers).ToNot(BeEmpty())
		Expect(podTemplate.Spec.Containers[0].Name).To(Equal("pgbouncer"))
		Expect(podTemplate.Spec.Containers[0].Image).To(Equal(config.Current.PgbouncerImageName))
	})

	It("sets the correct number of replicas", func() {
		pooler.Spec.Instances = ptr.To(int32(3))
		deployment, err := Deployment(pooler, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())
		Expect(deployment.Spec.Replicas).To(Equal(pooler.Spec.Instances))
	})

	It("sets the correct deployment strategy", func() {
		pooler.Spec.DeploymentStrategy = &appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}
		deployment, err := Deployment(pooler, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())
		Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RecreateDeploymentStrategyType))
	})

	It("creates correct volume mounts", func() {
		deployment, err := Deployment(pooler, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())
		Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts).ToNot(BeEmpty())
		Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("scratch-data"))
		Expect(deployment.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath).To(Equal(postgres.ScratchDataDirectory))
	})

	It("creates correct init containers", func() {
		deployment, err := Deployment(pooler, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())

		Expect(deployment.Spec.Template.Spec.InitContainers).ToNot(BeEmpty())
		Expect(deployment.Spec.Template.Spec.InitContainers[0].Name).To(Equal(specs.BootstrapControllerContainerName))
	})

	It("sets the correct service account name when not specified", func() {
		deployment, err := Deployment(pooler, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())
		Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal(pooler.Name))
	})

	It("sets the custom service account name when specified", func() {
		customPooler := pooler.DeepCopy()
		customPooler.Spec.ServiceAccountName = "custom-service-account"
		deployment, err := Deployment(customPooler, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())
		Expect(deployment.Spec.Template.Spec.ServiceAccountName).To(Equal("custom-service-account"))
	})

	It("sets the correct readiness probe", func() {
		deployment, err := Deployment(pooler, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())
		Expect(deployment.Spec.Template.Spec.Containers[0].ReadinessProbe.TimeoutSeconds).To(Equal(int32(5)))
		Expect(deployment.Spec.Template.Spec.Containers[0].ReadinessProbe.TCPSocket.Port).
			To(Equal(intstr.FromInt32(pgBouncerConfig.PgBouncerPort)))
	})

	It("should correctly set pod resources to the bootstrap init container", func() {
		pooler := &apiv1.Pooler{
			Spec: apiv1.PoolerSpec{
				Template: &apiv1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					},
				},
			},
		}

		dep, err := Deployment(pooler, cluster)
		Expect(err).ToNot(HaveOccurred())
		// check that the init container has the correct resources
		Expect(dep.Spec.Template.Spec.InitContainers).To(HaveLen(1))
		initResources := dep.Spec.Template.Spec.InitContainers[0].Resources
		Expect(initResources.Requests).To(HaveKeyWithValue(corev1.ResourceCPU, resource.MustParse("100m")))
		Expect(initResources.Requests).To(HaveKeyWithValue(corev1.ResourceMemory, resource.MustParse("128Mi")))
		Expect(initResources.Limits).To(HaveKeyWithValue(corev1.ResourceCPU, resource.MustParse("200m")))
		Expect(initResources.Limits).To(HaveKeyWithValue(corev1.ResourceMemory, resource.MustParse("256Mi")))
	})

	It("retains user-defined bootstrap-controller resources", func() {
		pooler.Spec.Template = &apiv1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{
					Name: specs.BootstrapControllerContainerName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2m"),
							corev1.ResourceMemory: resource.MustParse("30Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				}},
			},
		}

		deployment, err := Deployment(pooler, cluster)
		Expect(err).ShouldNot(HaveOccurred())
		var found *corev1.Container
		for i := range deployment.Spec.Template.Spec.InitContainers {
			if deployment.Spec.Template.Spec.InitContainers[i].Name == specs.BootstrapControllerContainerName {
				found = &deployment.Spec.Template.Spec.InitContainers[i]
				break
			}
		}
		Expect(found).ToNot(BeNil())
		Expect(found.Resources.Requests.Cpu().String()).To(Equal("2m"))
		Expect(found.Resources.Requests.Memory().String()).To(Equal("30Mi"))
		Expect(found.Resources.Limits.Cpu().String()).To(Equal("1"))
		Expect(found.Resources.Limits.Memory().String()).To(Equal("100Mi"))
	})
})

func mustComputeTemplateHash(pooler *apiv1.Pooler) string {
	poolerHash, err := computeTemplateHash(pooler, config.Current.OperatorImageName)
	Expect(err).ToNot(HaveOccurred())
	return poolerHash
}

// Every object of a Pooler gets the default and mandatory settings of its kind: the
// Pods those of `poolerPods`, the Service those of `services`, every object
// those of `all`. The Deployment is updated only when its hash changes, so
// the hash must change exactly when the Pods' metadata does, and stay the
// one of the previous versions without settings: otherwise upgrading the
// operator would roll out every Pooler, or a new setting would reach none.
var _ = Describe("The default and mandatory metadata on the objects of a Pooler", func() {
	var (
		previous *config.Data
		pooler   *apiv1.Pooler
		cluster  *apiv1.Cluster
	)

	settings := func(defaultLabels, mandatoryLabels, defaultAnnotations, mandatoryAnnotations []string) {
		config.Current.DefaultMetadata = rulesYAML(defaultLabels, defaultAnnotations)
		config.Current.MandatoryMetadata = rulesYAML(mandatoryLabels, mandatoryAnnotations)
		Expect(config.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
	}

	deployment := func() *appsv1.Deployment {
		result, err := Deployment(pooler, cluster)
		Expect(err).ToNot(HaveOccurred())
		return result
	}

	BeforeEach(func() {
		previous = config.Current
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
					Labels:      map[string]string{"team": "payments"},
					Annotations: map[string]string{"example.com/mesh": "on"},
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

	Context("without settings", func() {
		It("builds the Deployment as before, with the hash of the previous versions", func() {
			result := deployment()
			Expect(result.Annotations).To(Equal(map[string]string{
				utils.PoolerSpecHashAnnotationName: mustComputeTemplateHash(pooler),
			}))
			Expect(result.Labels).NotTo(HaveKey("team"))
			Expect(result.Spec.Template.Labels).To(HaveKeyWithValue("team", "payments"))
			Expect(result.Spec.Template.Annotations).To(Equal(map[string]string{"example.com/mesh": "on"}))
		})

		It("builds the other objects without any added key", func() {
			service, err := Service(pooler, cluster)
			Expect(err).ToNot(HaveOccurred())
			Expect(service.Annotations).To(HaveLen(2)) // the Pooler's and the spec hash
			Expect(ServiceAccount(pooler).Labels).To(BeEmpty())
			Expect(ServiceAccount(pooler).Annotations).To(BeEmpty())
			Expect(Role(pooler).Labels).To(BeEmpty())
			roleBinding := RoleBinding(pooler, "pooler-payments")
			Expect(roleBinding.Annotations).To(BeEmpty())
			Expect(NewPoolerPodMonitorManager(pooler, cluster).BuildPodMonitor().Annotations).To(BeEmpty())
		})
	})

	Context("with settings for other kinds of objects only", func() {
		It("keeps the hash and the Pods' metadata: no Pooler rolls out", func() {
			settings(nil, []string{"cost-centre=dbaas@pgPods+services+persistentVolumeClaims"},
				[]string{"example.com/note=x@pgPods"}, nil)
			result := deployment()
			Expect(result.Annotations[utils.PoolerSpecHashAnnotationName]).To(Equal(mustComputeTemplateHash(pooler)))
			Expect(result.Spec.Template.Labels).NotTo(HaveKey("cost-centre"))
			Expect(result.Labels).NotTo(HaveKey("cost-centre"))
		})

		It("keeps them for the PostgreSQL and major upgrade Pods only", func() {
			settings(nil, []string{"bla=1@pgPods", "bla=3@majorUpgradePods"}, nil, nil)
			result := deployment()
			Expect(result.Annotations[utils.PoolerSpecHashAnnotationName]).To(Equal(mustComputeTemplateHash(pooler)))
			Expect(result.Spec.Template.Labels).NotTo(HaveKey("bla"))
		})
	})

	Context("with settings naming the kinds of resources they apply to", func() {
		parse := func(defaults, mandatory string) {
			config.Current.DefaultMetadata = defaults
			config.Current.MandatoryMetadata = mandatory
			Expect(config.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		}

		It("leaves every object and the hash alone for the rules of other resources", func() {
			parse(`[{"resources": ["Cluster"], "targets": ["allPods"], "labels": {"mesh": "on"}}]`,
				`[{"resources": ["Cluster", "Backup"], "targets": ["all"], "labels": {"cost-centre": "dbaas"}},
				  {"resources": ["ScheduledBackup"], "targets": ["all"], "annotations": {"note": "x"}}]`)
			result := deployment()
			Expect(result.Annotations).To(Equal(map[string]string{
				utils.PoolerSpecHashAnnotationName: mustComputeTemplateHash(pooler),
			}))
			Expect(result.Labels).NotTo(HaveKey("cost-centre"))
			Expect(result.Spec.Template.Labels).NotTo(HaveKey("cost-centre"))
			Expect(result.Spec.Template.Labels).NotTo(HaveKey("mesh"))
			service, err := Service(pooler, cluster)
			Expect(err).ToNot(HaveOccurred())
			Expect(service.Labels).NotTo(HaveKey("cost-centre"))
			Expect(ServiceAccount(pooler).Labels).To(BeEmpty())
			Expect(Role(pooler).Labels).To(BeEmpty())
			Expect(RoleBinding(pooler, "pooler-payments").Labels).NotTo(HaveKey("cost-centre"))
			Expect(NewPoolerPodMonitorManager(pooler, cluster).BuildPodMonitor().Labels).NotTo(HaveKey("cost-centre"))
		})

		It("puts the rules naming the Pooler on its objects only", func() {
			parse("", `[{"resources": ["Cluster"], "targets": ["services"], "labels": {"lb-pool": "databases"}},
				{"resources": ["Pooler"], "targets": ["services"], "labels": {"lb-pool": "poolers"}},
				{"resources": ["Pooler"], "targets": ["all"], "labels": {"component": "pooler"}}]`)
			service, err := Service(pooler, cluster)
			Expect(err).ToNot(HaveOccurred())
			Expect(service.Labels).To(HaveKeyWithValue("lb-pool", "poolers"))
			Expect(service.Labels).To(HaveKeyWithValue("component", "pooler"))
			result := deployment()
			Expect(result.Labels).To(HaveKeyWithValue("component", "pooler"))
			Expect(result.Labels).NotTo(HaveKey("lb-pool"))
			Expect(result.Spec.Template.Labels).To(HaveKeyWithValue("component", "pooler"))
			Expect(result.Annotations[utils.PoolerSpecHashAnnotationName]).NotTo(Equal(mustComputeTemplateHash(pooler)))
			Expect(ServiceAccount(pooler).Labels).To(Equal(map[string]string{"component": "pooler"}))
			Expect(Role(pooler).Labels).To(Equal(map[string]string{"component": "pooler"}))
			Expect(RoleBinding(pooler, "pooler-payments").Labels).To(HaveKeyWithValue("component", "pooler"))
		})
	})

	Context("with settings for every kind of Pod", func() {
		It("puts them on the Pooler Pods, below their own entries, and rolls the Pooler out", func() {
			settings([]string{"mesh=pods@allPods"}, []string{"bla=pods@allPods", "bla=2@poolerPods"}, nil, nil)
			result := deployment()
			Expect(result.Spec.Template.Labels).To(HaveKeyWithValue("mesh", "pods"))
			Expect(result.Spec.Template.Labels).To(HaveKeyWithValue("bla", "2"))
			Expect(result.Labels).NotTo(HaveKey("mesh"))
			Expect(result.Labels).NotTo(HaveKey("bla"))
			Expect(result.Annotations[utils.PoolerSpecHashAnnotationName]).NotTo(Equal(mustComputeTemplateHash(pooler)))
		})

		It("gives the Pooler Pods the value of allPods when they have none of their own", func() {
			settings(nil, []string{"bla=pods@allPods", "bla=1@pgPods"}, nil, nil)
			Expect(deployment().Spec.Template.Labels).To(HaveKeyWithValue("bla", "pods"))
		})
	})

	Context("with settings for the Pooler Pods", func() {
		BeforeEach(func() {
			settings(
				[]string{"team=unassigned@poolerPods", "env=prod@all"},
				[]string{"cost-centre=dbaas@poolerPods"},
				[]string{"example.com/mesh=off@poolerPods"},
				[]string{"example.com/owner=platform@all"},
			)
		})

		It("puts them on the Pods, below the Pooler's keys for the defaults", func() {
			template := deployment().Spec.Template
			Expect(template.Labels).To(HaveKeyWithValue("team", "payments"))
			Expect(template.Labels).To(HaveKeyWithValue("env", "prod"))
			Expect(template.Labels).To(HaveKeyWithValue("cost-centre", "dbaas"))
			Expect(template.Labels).To(HaveKeyWithValue(utils.PgbouncerNameLabel, "pooler-payments"))
			Expect(template.Labels).To(HaveKeyWithValue(utils.PodRoleLabelName, string(utils.PodRolePooler)))
			Expect(template.Annotations).To(HaveKeyWithValue("example.com/mesh", "on"))
			Expect(template.Annotations).To(HaveKeyWithValue("example.com/owner", "platform"))
		})

		It("puts only the entries of all on the Deployment itself, and never in its selector", func() {
			result := deployment()
			Expect(result.Labels).To(HaveKeyWithValue("env", "prod"))
			Expect(result.Labels).NotTo(HaveKey("cost-centre"))
			Expect(result.Labels).NotTo(HaveKey("team"))
			Expect(result.Annotations).To(HaveKeyWithValue("example.com/owner", "platform"))
			Expect(result.Spec.Selector.MatchLabels).To(Equal(map[string]string{utils.PgbouncerNameLabel: "pooler-payments"}))
		})

		It("changes the hash, the same at every reconciliation", func() {
			first := deployment().Annotations[utils.PoolerSpecHashAnnotationName]
			Expect(first).NotTo(Equal(mustComputeTemplateHash(pooler)))
			for range 20 {
				Expect(deployment().Annotations[utils.PoolerSpecHashAnnotationName]).To(Equal(first))
			}
		})

		DescribeTable("changes the hash again when the Pods' metadata changes",
			func(change func()) {
				before := deployment().Annotations[utils.PoolerSpecHashAnnotationName]
				change()
				Expect(deployment().Annotations[utils.PoolerSpecHashAnnotationName]).NotTo(Equal(before))
			},
			Entry("a new mandatory value", func() {
				settings([]string{"team=unassigned@poolerPods", "env=prod@all"},
					[]string{"cost-centre=finance@poolerPods"},
					[]string{"example.com/mesh=off@poolerPods"}, []string{"example.com/owner=platform@all"})
			}),
			Entry("a new key through all", func() {
				settings([]string{"team=unassigned@poolerPods", "env=prod@all", "zone=eu@all"},
					[]string{"cost-centre=dbaas@poolerPods"},
					[]string{"example.com/mesh=off@poolerPods"}, []string{"example.com/owner=platform@all"})
			}),
			Entry("a change of the Pooler's template", func() {
				pooler.Spec.Template.ObjectMeta.Labels["team"] = "billing"
			}),
		)

		It("does not change the hash for a setting that does not reach the Pods", func() {
			before := deployment().Annotations[utils.PoolerSpecHashAnnotationName]
			settings([]string{"team=unassigned@poolerPods", "env=prod@all"},
				[]string{"cost-centre=dbaas@poolerPods", "tier=data@services+pgPods"},
				[]string{"example.com/mesh=off@poolerPods"}, []string{"example.com/owner=platform@all"})
			Expect(deployment().Annotations[utils.PoolerSpecHashAnnotationName]).To(Equal(before))
		})
	})

	Context("with settings for the Services and for every object", func() {
		BeforeEach(func() {
			settings(
				[]string{"env=prod@all"},
				[]string{"cost-centre=dbaas@all", "tier=data@poolerPods"},
				[]string{"example.com/owner=default@services"},
				[]string{"example.com/lb=internal@services"},
			)
		})

		It("puts the Services' entries on the Service, the Pooler's keys winning over the defaults", func() {
			service, err := Service(pooler, cluster)
			Expect(err).ToNot(HaveOccurred())
			Expect(service.Labels).To(HaveKeyWithValue("env", "prod"))
			Expect(service.Labels).To(HaveKeyWithValue("cost-centre", "dbaas"))
			Expect(service.Labels).NotTo(HaveKey("tier"))
			Expect(service.Annotations).To(HaveKeyWithValue("example.com/owner", "team-a"))
			Expect(service.Annotations).To(HaveKeyWithValue("example.com/lb", "internal"))
			Expect(service.Spec.Selector).To(Equal(map[string]string{utils.PgbouncerNameLabel: "pooler-payments"}))
		})

		It("puts the entries of all on the ServiceAccount, Role, RoleBinding and PodMonitor", func() {
			expected := map[string]string{"env": "prod", "cost-centre": "dbaas"}
			Expect(ServiceAccount(pooler).Labels).To(Equal(expected))
			Expect(Role(pooler).Labels).To(Equal(expected))
			roleBinding := RoleBinding(pooler, "pooler-payments")
			Expect(roleBinding.Labels).To(HaveKeyWithValue("env", "prod"))
			Expect(roleBinding.Labels).To(HaveKeyWithValue("cost-centre", "dbaas"))
			podMonitor := NewPoolerPodMonitorManager(pooler, cluster).BuildPodMonitor()
			Expect(podMonitor.Labels).To(HaveKeyWithValue("cost-centre", "dbaas"))
			Expect(podMonitor.Labels).To(HaveKeyWithValue(utils.PgbouncerNameLabel, "pooler-payments"))
			Expect(podMonitor.Labels).NotTo(HaveKey("tier"))
		})
	})
})
