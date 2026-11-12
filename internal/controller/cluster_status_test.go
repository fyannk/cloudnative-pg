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
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/certs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/reconciler/persistentvolumeclaim"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("cluster_status unit tests", func() {
	var env *testingEnvironment
	BeforeEach(func() {
		env = buildTestEnvironment()
	})

	It("should make sure setCertExpiration works correctly", func() {
		var certExpirationDate string
		ctx := context.Background()
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace)
		secretName := rand.String(10)

		By("creating the required secret", func() {
			secret, keyPair := generateFakeCASecret(env.client, secretName, namespace, "unittest.com")
			Expect(secret.Name).To(Equal(secretName))

			_, expDate, err := keyPair.IsExpiring()
			Expect(err).ToNot(HaveOccurred())

			certExpirationDate = expDate.String()
		})
		By("making sure that sets the status of the secret correctly", func() {
			cluster.Status.Certificates.Expirations = map[string]string{}
			err := env.clusterReconciler.setCertExpiration(ctx, cluster, secretName, namespace, certs.CACertKey)
			Expect(err).ToNot(HaveOccurred())
			Expect(cluster.Status.Certificates.Expirations[secretName]).To(Equal(certExpirationDate))
		})
	})

	It("makes sure that getPgbouncerIntegrationStatus returns the correct secret name without duplicates", func() {
		ctx := context.Background()
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace)
		pooler1 := *newFakePooler(env.client, cluster)
		pooler2 := *newFakePooler(env.client, cluster)
		Expect(pooler1.Name).ToNot(Equal(pooler2.Name))
		poolerList := apiv1.PoolerList{Items: []apiv1.Pooler{pooler1, pooler2}}

		intStatus, err := env.clusterReconciler.getPgbouncerIntegrationStatus(ctx, cluster, poolerList)
		Expect(err).ToNot(HaveOccurred())
		Expect(intStatus.Secrets).To(HaveLen(1))
	})

	It("makes sure getObjectResourceVersion returns the correct object version", func() {
		ctx := context.Background()
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace)
		pooler := newFakePooler(env.client, cluster)

		version, err := env.clusterReconciler.getObjectResourceVersion(ctx, cluster, pooler.Name, &apiv1.Pooler{})
		Expect(err).ToNot(HaveOccurred())
		Expect(version).To(Equal(pooler.ResourceVersion))
	})

	It("makes sure setPrimaryInstance works correctly", func() {
		const podName = "test-pod"
		ctx := context.Background()
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace)
		Expect(cluster.Status.TargetPrimaryTimestamp).To(BeEmpty())

		By("setting the primaryInstance and making sure the passed object is updated", func() {
			err := env.clusterReconciler.setPrimaryInstance(ctx, cluster, podName)
			Expect(err).ToNot(HaveOccurred())
			Expect(cluster.Status.TargetPrimaryTimestamp).ToNot(BeEmpty())
			Expect(cluster.Status.TargetPrimary).To(Equal(podName))
		})

		By("making sure the remote resource is updated", func() {
			remoteCluster := &apiv1.Cluster{}

			err := env.client.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, remoteCluster)
			Expect(err).ToNot(HaveOccurred())
			Expect(remoteCluster.Status.TargetPrimaryTimestamp).ToNot(BeEmpty())
			Expect(remoteCluster.Status.TargetPrimary).To(Equal(podName))
		})
	})

	It("makes sure RegisterPhase works correctly", func() {
		const phaseReason = "testing"
		ctx := context.Background()
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace)

		By("registering the phase and making sure the passed object is updated", func() {
			err := env.clusterReconciler.RegisterPhase(ctx, cluster, apiv1.PhaseSwitchover, phaseReason)
			Expect(err).ToNot(HaveOccurred())
			Expect(cluster.Status.Phase).To(Equal(apiv1.PhaseSwitchover))
			Expect(cluster.Status.PhaseReason).To(Equal(phaseReason))
		})

		By("making sure the remote resource is updated", func() {
			remoteCluster := &apiv1.Cluster{}
			err := env.client.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, remoteCluster)
			Expect(err).ToNot(HaveOccurred())
			Expect(remoteCluster.Status.Phase).To(Equal(apiv1.PhaseSwitchover))
			Expect(remoteCluster.Status.PhaseReason).To(Equal(phaseReason))
		})
	})

	It("makes sure that getManagedResources works correctly", func() {
		ctx := context.Background()
		crReconciler := &ClusterReconciler{
			Client: fakeClientWithIndexAdapter{
				Client: env.clusterReconciler.Client,
			},
			Scheme:         env.clusterReconciler.Scheme,
			Recorder:       env.clusterReconciler.Recorder,
			InstanceClient: env.clusterReconciler.InstanceClient,
		}

		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace)
		var jobs []batchv1.Job
		var pods []corev1.Pod
		var pvcs []corev1.PersistentVolumeClaim

		By("creating the required resources", func() {
			jobs = generateFakeInitDBJobs(crReconciler.Client, cluster)
			pods = generateFakeClusterPods(crReconciler.Client, cluster, true)
			pvcs = generateClusterPVC(crReconciler.Client, cluster, persistentvolumeclaim.StatusReady)
			name, isOwned := IsOwnedByCluster(&pods[0])
			Expect(isOwned).To(BeTrue())
			Expect(name).To(Equal(cluster.Name))
		})

		By("making sure that the required resources are found", func() {
			Eventually(func() (*managedResources, error) {
				return crReconciler.getManagedResources(ctx, cluster)
			}).Should(Satisfy(func(mr *managedResources) bool {
				return len(mr.instances.Items) == len(pods) &&
					len(mr.jobs.Items) == len(jobs) &&
					len(mr.pvcs.Items) == len(pvcs)
			}))
		})
	})

	It("produces a scale-subresource selector matching managed instance pods", func() {
		// updateResourceStatus publishes cluster.GetInstancesSelector() into
		// .status.selector so VPA/HPA can discover instance pods through the scale
		// subresource. We exercise the production code (GetInstancesSelector) and
		// verify both that it has the expected format and that it actually matches
		// the labels the operator applies to every instance pod.
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace)
		pods := generateFakeClusterPods(env.client, cluster, true)
		Expect(pods).ToNot(BeEmpty())

		selectorString := cluster.GetInstancesSelector()

		// GetInstancesSelector serializes through labels.SelectorFromSet, which
		// sorts requirements by key. The cluster label key sorts before the pod
		// role label key, so the expected string lists them in that order.
		expected := fmt.Sprintf("%s=%s,%s=%s",
			utils.ClusterLabelName, cluster.Name,
			utils.PodRoleLabelName, string(utils.PodRoleInstance))
		Expect(selectorString).To(Equal(expected))

		selector, err := labels.Parse(selectorString)
		Expect(err).ToNot(HaveOccurred())

		for i := range pods {
			Expect(selector.Matches(labels.Set(pods[i].Labels))).To(BeTrue(),
				"selector %q must match pod %s labels %v", selectorString, pods[i].Name, pods[i].Labels)
		}
	})
})

var _ = Describe("updateClusterStatusThatRequiresInstancesState tests", func() {
	var (
		env     *testingEnvironment
		cluster *apiv1.Cluster
	)

	BeforeEach(func() {
		env = buildTestEnvironment()
		cluster = newFakeCNPGCluster(env.client, newFakeNamespace(env.client))
	})

	It("should handle empty status list", func(ctx SpecContext) {
		statuses := postgres.PostgresqlStatusList{}

		err := env.clusterReconciler.updateClusterStatusThatRequiresInstancesState(ctx, cluster, statuses)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.Status.InstancesReportedState).To(BeEmpty())
		Expect(cluster.Status.SystemID).To(BeEmpty())

		condition := meta.FindStatusCondition(cluster.Status.Conditions, string(apiv1.ConditionConsistentSystemID))
		Expect(condition).ToNot(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("NotFound"))
		Expect(condition.Message).To(Equal("No instances are present in the cluster to report a system ID."))
	})

	It("should handle instances without SystemID", func(ctx SpecContext) {
		statuses := postgres.PostgresqlStatusList{
			Items: []postgres.PostgresqlStatus{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.1"},
					},
					IsPrimary:  true,
					TimeLineID: 123,
					SystemID:   "",
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.2"},
					},
					IsPrimary: false,
					SystemID:  "",
				},
			},
		}

		err := env.clusterReconciler.updateClusterStatusThatRequiresInstancesState(ctx, cluster, statuses)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.Status.InstancesReportedState).To(HaveLen(2))
		Expect(cluster.Status.TimelineID).To(Equal(123))
		Expect(cluster.Status.SystemID).To(BeEmpty())

		condition := meta.FindStatusCondition(cluster.Status.Conditions, string(apiv1.ConditionConsistentSystemID))
		Expect(condition).ToNot(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("NotFound"))
		Expect(condition.Message).To(Equal("Instances are present, but none have reported a system ID."))
	})

	It("should handle instances with a single SystemID", func(ctx SpecContext) {
		const systemID = "system123"
		statuses := postgres.PostgresqlStatusList{
			Items: []postgres.PostgresqlStatus{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.1"},
					},
					IsPrimary:  true,
					TimeLineID: 123,
					SystemID:   systemID,
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.2"},
					},
					IsPrimary: false,
					SystemID:  systemID,
				},
			},
		}

		err := env.clusterReconciler.updateClusterStatusThatRequiresInstancesState(ctx, cluster, statuses)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.Status.InstancesReportedState).To(HaveLen(2))
		Expect(cluster.Status.TimelineID).To(Equal(123))
		Expect(cluster.Status.SystemID).To(Equal(systemID))

		condition := meta.FindStatusCondition(cluster.Status.Conditions, string(apiv1.ConditionConsistentSystemID))
		Expect(condition).ToNot(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal("Unique"))
		Expect(condition.Message).To(Equal("A single, unique system ID was found across reporting instances."))
	})

	It("should handle instances with multiple SystemIDs", func(ctx SpecContext) {
		statuses := postgres.PostgresqlStatusList{
			Items: []postgres.PostgresqlStatus{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.1"},
					},
					IsPrimary:  true,
					TimeLineID: 123,
					SystemID:   "system1",
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.2"},
					},
					IsPrimary: false,
					SystemID:  "system2",
				},
			},
		}

		err := env.clusterReconciler.updateClusterStatusThatRequiresInstancesState(ctx, cluster, statuses)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.Status.InstancesReportedState).To(HaveLen(2))
		Expect(cluster.Status.TimelineID).To(Equal(123))
		Expect(cluster.Status.SystemID).To(BeEmpty())

		condition := meta.FindStatusCondition(cluster.Status.Conditions, string(apiv1.ConditionConsistentSystemID))
		Expect(condition).ToNot(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(condition.Reason).To(Equal("Mismatch"))
		Expect(condition.Message).To(ContainSubstring("Multiple differing system IDs reported by instances:"))
		Expect(condition.Message).To(ContainSubstring("system1"))
		Expect(condition.Message).To(ContainSubstring("system2"))
	})

	It("should update timeline ID from the primary instance", func(ctx SpecContext) {
		const timelineID = 999
		statuses := postgres.PostgresqlStatusList{
			Items: []postgres.PostgresqlStatus{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.1"},
					},
					IsPrimary:  true,
					TimeLineID: timelineID,
					SystemID:   "system1",
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.2"},
					},
					IsPrimary:  false,
					TimeLineID: 123,
					SystemID:   "system1",
				},
			},
		}

		err := env.clusterReconciler.updateClusterStatusThatRequiresInstancesState(ctx, cluster, statuses)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.Status.TimelineID).To(Equal(timelineID))
	})

	It("should correctly populate InstancesReportedState", func(ctx SpecContext) {
		statuses := postgres.PostgresqlStatusList{
			Items: []postgres.PostgresqlStatus{
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.1"},
					},
					IsPrimary:  true,
					TimeLineID: 123,
				},
				{
					Pod: &corev1.Pod{
						ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
						Status:     corev1.PodStatus{PodIP: "192.168.1.2"},
					},
					IsPrimary:  false,
					TimeLineID: 123,
				},
			},
		}

		err := env.clusterReconciler.updateClusterStatusThatRequiresInstancesState(ctx, cluster, statuses)
		Expect(err).ToNot(HaveOccurred())

		Expect(cluster.Status.InstancesReportedState).To(HaveLen(2))

		state1 := cluster.Status.InstancesReportedState["pod-1"]
		Expect(state1.IsPrimary).To(BeTrue())
		Expect(state1.TimeLineID).To(Equal(123))
		Expect(state1.IP).To(Equal("192.168.1.1"))

		state2 := cluster.Status.InstancesReportedState["pod-2"]
		Expect(state2.IsPrimary).To(BeFalse())
		Expect(state2.TimeLineID).To(Equal(123))
		Expect(state2.IP).To(Equal("192.168.1.2"))
	})
})

// While the operator waits for an instance Pod that is not active, the
// cause is on the Pod (scheduling, a container that cannot start). The
// owner of the Cluster, who may not know where to look, gets it in the
// phase reason next to the generic sentence. The Pod here is scheduled but
// its container cannot start (an unschedulable Pod has its own handling).
var _ = Describe("waiting for an instance that reports why it is not active", func() {
	It("copies the Pod's cause into the phase reason", func(ctx SpecContext) {
		pendingPod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-example-1", Namespace: "default"},
			Status: corev1.PodStatus{
				Phase:      corev1.PodPending,
				Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}},
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "postgres",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason:  "ContainerCreating",
						Message: "MountVolume.SetUp failed for volume \"app-secret\": secret \"app\" not found",
					}},
				}},
			},
		}
		cluster := &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-example", Namespace: "default"},
			Spec:       apiv1.ClusterSpec{ImageName: "postgres:17.2", Instances: 1},
			Status:     apiv1.ClusterStatus{CurrentPrimary: "cluster-example-1", TargetPrimary: "cluster-example-1"},
		}
		r, _ := newUnrecoverableReconciler(interceptor.Funcs{}, cluster, &pendingPod)
		resources := &managedResources{instances: corev1.PodList{Items: []corev1.Pod{pendingPod}}}

		result, err := r.reconcileResources(ctx, cluster, resources, postgres.PostgresqlStatusList{})
		Expect(err).ToNot(HaveOccurred())
		Expect(result).To(Equal(ctrl.Result{RequeueAfter: time.Second}))
		Expect(cluster.Status.Phase).To(Equal(apiv1.PhaseWaitingForInstancesToBeActive))
		Expect(cluster.Status.PhaseReason).To(Equal("Some instances are not yet active. Please wait. " +
			"cluster-example-1: container postgres waiting (ContainerCreating: " +
			"MountVolume.SetUp failed for volume \"app-secret\": secret \"app\" not found)"))

		// A Pod that reports nothing yet keeps the generic sentence alone
		resources.instances.Items[0].Status.ContainerStatuses = nil
		_, err = r.reconcileResources(ctx, cluster, resources, postgres.PostgresqlStatusList{})
		Expect(err).ToNot(HaveOccurred())
		Expect(cluster.Status.PhaseReason).To(Equal("Some instances are not yet active. Please wait."))
	})
})

var _ = Describe("topology availability without the Node API", func() {
	const zoneLabel = "topology.kubernetes.io/zone"

	BeforeEach(func() {
		previous := configuration.Current.APINodeEnabled
		previousSource := configuration.Current.NodeStatusSource
		configuration.Current.APINodeEnabled = false
		configuration.Current.NodeStatusSource = configuration.NodeStatusSourceNone
		DeferCleanup(func() {
			configuration.Current.APINodeEnabled = previous
			configuration.Current.NodeStatusSource = previousSource
		})
	})

	It("sets one condition and warning while preserving the node count from Pods", func() {
		recorder := record.NewFakeRecorder(10)
		reconciler := &ClusterReconciler{
			Recorder: recorder, nodeStatusSource: configuration.NodeStatusSourceNone,
		}
		cluster := &apiv1.Cluster{}
		pods := []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "pg-1"}, Spec: corev1.PodSpec{NodeName: "node-1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pg-2"}, Spec: corev1.PodSpec{NodeName: "node-2"}},
		}

		reconciler.updateTopologyAvailability(cluster, nil, pods, []string{zoneLabel})
		Expect(cluster.IsTopologyUnavailable()).To(BeTrue())
		Expect(cluster.Status.Topology.SuccessfullyExtracted).To(BeFalse())
		Expect(cluster.Status.Topology.NodesUsed).To(BeEquivalentTo(2))
		Expect(<-recorder.Events).To(ContainSubstring(zoneLabel))

		previous := cluster.Status.Conditions
		reconciler.updateTopologyAvailability(cluster, previous, pods, []string{zoneLabel})
		Expect(recorder.Events).ToNot(Receive())

		reconciler.updateTopologyAvailability(cluster, cluster.Status.Conditions, pods, nil)
		Expect(cluster.IsTopologyUnavailable()).To(BeFalse())
		Expect(cluster.Status.Topology.SuccessfullyExtracted).To(BeTrue())
		Expect(cluster.Status.Topology.NodesUsed).To(BeEquivalentTo(2))
	})
})

var _ = Describe("topology from echoed node labels", func() {
	const zoneLabel = "topology.kubernetes.io/zone"

	It("reads node labels copied onto the instance Pods", func() {
		reader := utils.NodeStatusReader{LabelPrefix: "node.example.com/"}
		pods := []corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pg-1",
					Labels: map[string]string{"node.example.com/" + zoneLabel: "zone-a"},
				},
				Spec: corev1.PodSpec{NodeName: "node-1"},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "pg-2",
					Labels: map[string]string{"node.example.com/" + zoneLabel: "zone-b"},
				},
				Spec: corev1.PodSpec{NodeName: "node-2"},
			},
		}

		topology := getPodsTopologyWithNodeLabels(context.Background(), pods, []string{zoneLabel}, reader.NodeLabels)
		Expect(topology.SuccessfullyExtracted).To(BeTrue())
		Expect(topology.Instances["pg-1"][zoneLabel]).To(Equal("zone-a"))
		Expect(topology.Instances["pg-2"][zoneLabel]).To(Equal("zone-b"))
		Expect(topology.NodesUsed).To(BeEquivalentTo(2))
	})

	It("leaves the topology unavailable when no node state is read", func() {
		reconciler := &ClusterReconciler{nodeStatusSource: configuration.NodeStatusSourceNone}
		cluster := &apiv1.Cluster{}
		pods := []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "pg-1"}, Spec: corev1.PodSpec{NodeName: "node-1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "pg-2"}, Spec: corev1.PodSpec{NodeName: "node-2"}},
		}

		reconciler.updateTopologyAvailability(cluster, nil, pods, []string{zoneLabel})
		Expect(cluster.IsTopologyUnavailable()).To(BeTrue())
		Expect(cluster.Status.Topology.NodesUsed).To(BeEquivalentTo(2))
		Expect(cluster.Status.Conditions[0].Message).To(ContainSubstring("NODE_STATUS_SOURCE=none"))
	})
})

var _ = Describe("topology resolution by source of node state", func() {
	const (
		zoneLabel = "topology.kubernetes.io/zone"
		prefix    = "node.example.com/"
	)
	labelNames := []string{zoneLabel}
	reader := utils.NodeStatusReader{LabelPrefix: prefix}

	pod := func(name, nodeName string, labels map[string]string) corev1.Pod {
		return corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
			Spec:       corev1.PodSpec{NodeName: nodeName},
		}
	}
	nodes := map[string]corev1.Node{
		"node-1": {ObjectMeta: metav1.ObjectMeta{Name: "node-1", Labels: map[string]string{zoneLabel: "az1"}}},
	}

	It("fails the extraction when a copy is missing, as an unknown node does", func() {
		pods := []corev1.Pod{
			pod("pg-1", "node-1", map[string]string{prefix + zoneLabel: "az1"}),
			pod("pg-2", "node-2", nil),
		}
		topology := getPodsTopologyWithNodeLabels(context.Background(), pods, labelNames, reader.NodeLabels)
		Expect(topology.SuccessfullyExtracted).To(BeFalse())
	})

	It("fails the extraction without node state when node labels are needed", func() {
		pods := []corev1.Pod{pod("pg-1", "node-1", nil)}
		topology := getPodsTopologyWithNodeLabels(context.Background(), pods, labelNames, noNodeLabels)
		Expect(topology.SuccessfullyExtracted).To(BeFalse())
	})

	DescribeTable("the resolver of the node labels follows the source of node state",
		func(source string, expected bool) {
			r := &ClusterReconciler{nodeStatusSource: source, nodeStatus: reader}
			p := pod("pg-1", "node-1", map[string]string{prefix + zoneLabel: "az1"})
			_, ok := r.nodeLabelsResolver(nodes)(&p, labelNames)
			Expect(ok).To(Equal(expected))
		},
		Entry("api", configuration.NodeStatusSourceAPI, true),
		Entry("pods", configuration.NodeStatusSourcePods, true),
		Entry("none", configuration.NodeStatusSourceNone, false),
	)

	It("the resolver of the API source reads the Nodes only", func() {
		r := &ClusterReconciler{nodeStatusSource: configuration.NodeStatusSourceAPI, nodeStatus: reader}
		p := pod("pg-1", "node-1", map[string]string{prefix + zoneLabel: "az1"})
		_, ok := r.nodeLabelsResolver(map[string]corev1.Node{})(&p, labelNames)
		Expect(ok).To(BeFalse())
	})

	It("the resolver of the pods source reads the Pod only", func() {
		r := &ClusterReconciler{nodeStatusSource: configuration.NodeStatusSourcePods, nodeStatus: reader}
		p := pod("pg-1", "node-1", nil)
		_, ok := r.nodeLabelsResolver(nodes)(&p, labelNames)
		Expect(ok).To(BeFalse())
	})

	DescribeTable("updateTopologyAvailability sets nothing when node state is read",
		func(source string) {
			recorder := record.NewFakeRecorder(10)
			r := &ClusterReconciler{Recorder: recorder, nodeStatusSource: source}
			cluster := &apiv1.Cluster{}
			cluster.Status.Conditions = []metav1.Condition{{
				Type: string(apiv1.ConditionTopologyUnavailable), Status: metav1.ConditionTrue,
			}}
			pods := []corev1.Pod{pod("pg-1", "node-1", nil)}

			r.updateTopologyAvailability(cluster, cluster.Status.Conditions, pods, labelNames)
			Expect(meta.FindStatusCondition(cluster.Status.Conditions,
				string(apiv1.ConditionTopologyUnavailable))).To(BeNil())
			Expect(recorder.Events).ToNot(Receive())
		},
		Entry("api", configuration.NodeStatusSourceAPI),
		Entry("pods", configuration.NodeStatusSourcePods),
	)
})
