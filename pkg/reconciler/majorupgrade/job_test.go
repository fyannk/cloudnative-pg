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

package majorupgrade

import (
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	schemeBuilder "github.com/cloudnative-pg/cloudnative-pg/internal/scheme"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Major upgrade Job generation", func() {
	oldImageInfo := &apiv1.ImageInfo{
		Image:        "postgres:16",
		MajorVersion: 16,
	}
	newImageName := "postgres:17"

	cluster := apiv1.Cluster{
		Spec: apiv1.ClusterSpec{
			ImageName: newImageName,
			Bootstrap: &apiv1.BootstrapConfiguration{
				InitDB: &apiv1.BootstrapInitDB{},
			},
		},
		Status: apiv1.ClusterStatus{
			Image:           newImageName,
			PGDataImageInfo: oldImageInfo.DeepCopy(),
		},
	}

	It("creates major upgrade jobs", func() {
		majorUpgradeJob := createMajorUpgradeJobDefinition(&cluster, 1, nil)
		Expect(majorUpgradeJob).ToNot(BeNil())
		Expect(majorUpgradeJob.Spec.Template.Spec.Containers[0].Image).To(Equal(newImageName))
	})

	It("is able to discover which target image was used", func() {
		majorUpgradeJob := createMajorUpgradeJobDefinition(&cluster, 1, nil)
		Expect(majorUpgradeJob).ToNot(BeNil())

		imgName, found := getTargetImageFromMajorUpgradeJob(majorUpgradeJob)
		Expect(found).To(BeTrue())
		Expect(imgName).To(Equal(newImageName))
	})

	DescribeTable(
		"Tells major upgrade jobs apart from jobs of other types",
		func(job *batchv1.Job, isMajorUpgrade bool) {
			Expect(isMajorUpgradeJob(job)).To(Equal(isMajorUpgrade))
		},
		Entry("initdb jobs are not major upgrades", func() *batchv1.Job {
			cmd := specs.BuildPrimaryBootstrapCommandViaInitdb(cluster)
			return specs.CreatePrimaryJob(cluster, 1, cmd.Role, cmd.Command, nil)
		}(), false),
		Entry("major-upgrade jobs are major upgrades", createMajorUpgradeJobDefinition(&cluster, 1, nil), true),
	)

	It("always disables the automount and projects the token for non-bootstrap containers", func() {
		majorUpgradeJob := createMajorUpgradeJobDefinition(&cluster, 1, nil)
		podSpec := majorUpgradeJob.Spec.Template.Spec
		Expect(podSpec.AutomountServiceAccountToken).To(HaveValue(BeFalse()))

		Expect(podSpec.InitContainers).To(HaveLen(2))
		Expect(podSpec.Containers).To(HaveLen(1))

		// The bootstrap init container only copies the manager binary and does
		// not need Kubernetes API access.
		bootstrapContainer := podSpec.InitContainers[0]
		Expect(bootstrapContainer.Name).To(Equal(specs.BootstrapControllerContainerName))
		Expect(bootstrapContainer.VolumeMounts).NotTo(ContainElement(
			HaveField("MountPath", "/var/run/secrets/kubernetes.io/serviceaccount"),
		))

		// All other containers need to reach the Kubernetes API.
		nonBootstrapContainers := append(podSpec.InitContainers[1:], podSpec.Containers...)
		for _, container := range nonBootstrapContainers {
			Expect(container.VolumeMounts).To(ContainElement(
				HaveField("MountPath", "/var/run/secrets/kubernetes.io/serviceaccount"),
			), "container %s should mount the projected token", container.Name)
		}
	})
})

// The Pods of the major upgrade Job get the metadata of `majorUpgradePods`
// and `allPods`, the Job that of every object only. Neither gets the metadata
// of the PostgreSQL Pods
var _ = Describe("The metadata of the major upgrade Job", func() {
	var cluster *apiv1.Cluster

	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })

		cluster = &apiv1.Cluster{
			TypeMeta:   metav1.TypeMeta{Kind: apiv1.ClusterKind, APIVersion: apiv1.SchemeGroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "payments", UID: "uid"},
			Spec: apiv1.ClusterSpec{
				ImageName:         "postgres:17",
				Bootstrap:         &apiv1.BootstrapConfiguration{InitDB: &apiv1.BootstrapInitDB{}},
				InheritedMetadata: &apiv1.EmbeddedObjectMetadata{Labels: map[string]string{"env": "prod"}},
			},
			Status: apiv1.ClusterStatus{
				Image:           "postgres:17",
				PGDataImageInfo: &apiv1.ImageInfo{Image: "postgres:16", MajorVersion: 16},
			},
		}
	})

	createJob := func(ctx SpecContext) batchv1.Job {
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		cli := fake.NewClientBuilder().WithScheme(schemeBuilder.BuildWithAllKnownScheme()).Build()
		_, err := createMajorUpgradeJob(ctx, cli, cluster, 1, nil)
		Expect(err).ToNot(HaveOccurred())

		var jobs batchv1.JobList
		Expect(cli.List(ctx, &jobs, client.InNamespace("payments"))).To(Succeed())
		Expect(jobs.Items).To(HaveLen(1))
		return jobs.Items[0]
	}

	It("gives the Job and its Pods the Cluster's metadata, as in stock, with no settings", func(ctx SpecContext) {
		job := createJob(ctx)
		for _, meta := range []metav1.ObjectMeta{job.ObjectMeta, job.Spec.Template.ObjectMeta} {
			Expect(meta.Labels).To(HaveKeyWithValue("env", "prod"))
			Expect(meta.Labels).To(HaveKeyWithValue("cnpg.io/cluster", "payments"))
		}
		Expect(job.Spec.Template.Labels).To(HaveKeyWithValue("cnpg.io/instanceRole", "primary"))
	})

	It("gives its Pods the metadata of majorUpgradePods and allPods, not that of pgPods", func(ctx SpecContext) {
		configuration.Current.DefaultMetadata = `[{"targets": ["pgPods"], "annotations": {"example.com/mesh": "off"}}]`
		configuration.Current.MandatoryMetadata = `[` +
			`{"targets": ["all"], "labels": {"bla": "0"}},` +
			`{"targets": ["pgPods"], "labels": {"bla": "1"}},` +
			`{"targets": ["poolerPods"], "labels": {"bla": "2"}},` +
			`{"targets": ["majorUpgradePods"], "labels": {"bla": "3"}},` +
			`{"targets": ["allPods"], "labels": {"cost-centre": "pods"}}` +
			`]`
		cluster.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
			{
				Targets:     []apiv1.MetadataPropagationTarget{"pgPods"},
				Annotations: map[string]string{"vault.hashicorp.com/agent-inject": "true"},
			},
			{
				Targets:     []apiv1.MetadataPropagationTarget{"majorUpgradePods"},
				Annotations: map[string]string{"sidecar.istio.io/inject": "false"},
			},
		}

		job := createJob(ctx)
		pod := job.Spec.Template.ObjectMeta
		Expect(pod.Labels).To(HaveKeyWithValue("env", "prod"))
		Expect(pod.Labels).To(HaveKeyWithValue("bla", "3"))
		Expect(pod.Labels).To(HaveKeyWithValue("cost-centre", "pods"))
		Expect(pod.Annotations).To(HaveKeyWithValue("sidecar.istio.io/inject", "false"))
		Expect(pod.Annotations).NotTo(HaveKey("vault.hashicorp.com/agent-inject"))
		Expect(pod.Annotations).NotTo(HaveKey("example.com/mesh"))

		Expect(job.Labels).To(HaveKeyWithValue("bla", "0"))
		Expect(job.Labels).NotTo(HaveKey("cost-centre"))
		Expect(job.Annotations).NotTo(HaveKey("sidecar.istio.io/inject"))
	})

	It("keeps the labels the operator sets on its Pods over the settings", func(ctx SpecContext) {
		cluster.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
			{
				Targets: []apiv1.MetadataPropagationTarget{"majorUpgradePods"},
				Labels:  map[string]string{"cnpg.io/instanceRole": "replica", "cnpg.io/cluster": "other"},
			},
		}
		job := createJob(ctx)
		Expect(job.Spec.Template.Labels).To(HaveKeyWithValue("cnpg.io/instanceRole", "primary"))
		Expect(job.Spec.Template.Labels).To(HaveKeyWithValue("cnpg.io/cluster", "payments"))
	})
})
