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

package operatormetrics

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// failingReader fails to list one kind, as a cache does when an informer
// cannot be started, and serves the others
type failingReader struct {
	client.Reader
	failing client.ObjectList
}

func (r failingReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if fmt.Sprintf("%T", list) == fmt.Sprintf("%T", r.failing) {
		return errors.New("informer not started")
	}
	return r.Reader.List(ctx, list, opts...)
}

// gather scrapes a collector once and returns its series as
// name{label="value",...} -> value
func gather(c prometheus.Collector) map[string]float64 {
	registry := prometheus.NewPedanticRegistry()
	Expect(registry.Register(c)).To(Succeed())
	families, err := registry.Gather()
	Expect(err).ToNot(HaveOccurred())

	series := make(map[string]float64)
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels := make([]string, 0, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				labels = append(labels, fmt.Sprintf("%s=%q", label.GetName(), label.GetValue()))
			}
			sort.Strings(labels)
			series[fmt.Sprintf("%s{%s}", family.GetName(), joinLabels(labels))] = metric.GetGauge().GetValue()
		}
	}
	return series
}

func joinLabels(labels []string) string {
	result := ""
	for i, label := range labels {
		if i > 0 {
			result += ","
		}
		result += label
	}
	return result
}

var _ = Describe("The operator metrics collector", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(apiv1.AddToScheme(scheme)).To(Succeed())
	})

	meta := func(namespace, name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{Namespace: namespace, Name: name}
	}

	// One scrape over an inventory spread across two namespaces: the counts
	// are split per namespace, and per Cluster for the kinds referencing one;
	// kinds with no object have no series; the build info is a constant 1.
	It("counts every CloudNativePG kind per namespace and per Cluster, and reports the build", func() {
		objects := []client.Object{
			&apiv1.Cluster{ObjectMeta: meta("tenant-a", "pg-1")},
			&apiv1.Cluster{ObjectMeta: meta("tenant-a", "pg-2")},
			&apiv1.Cluster{ObjectMeta: meta("tenant-b", "pg-3")},
			&apiv1.Backup{
				ObjectMeta: meta("tenant-a", "b1"),
				Spec:       apiv1.BackupSpec{Cluster: apiv1.LocalObjectReference{Name: "pg-1"}},
			},
			&apiv1.Backup{
				ObjectMeta: meta("tenant-a", "b2"),
				Spec:       apiv1.BackupSpec{Cluster: apiv1.LocalObjectReference{Name: "pg-1"}},
			},
			&apiv1.Backup{
				ObjectMeta: meta("tenant-a", "b3"),
				Spec:       apiv1.BackupSpec{Cluster: apiv1.LocalObjectReference{Name: "pg-2"}},
			},
			&apiv1.ScheduledBackup{
				ObjectMeta: meta("tenant-b", "s1"),
				Spec:       apiv1.ScheduledBackupSpec{Cluster: apiv1.LocalObjectReference{Name: "pg-3"}},
			},
			&apiv1.Pooler{
				ObjectMeta: meta("tenant-a", "p1"),
				Spec:       apiv1.PoolerSpec{Cluster: apiv1.LocalObjectReference{Name: "pg-1"}},
			},
			&apiv1.Database{
				ObjectMeta: meta("tenant-a", "d1"),
				Spec:       apiv1.DatabaseSpec{ClusterRef: corev1.LocalObjectReference{Name: "pg-2"}},
			},
			&apiv1.Publication{
				ObjectMeta: meta("tenant-a", "pub1"),
				Spec:       apiv1.PublicationSpec{ClusterRef: corev1.LocalObjectReference{Name: "pg-2"}},
			},
			&apiv1.Subscription{
				ObjectMeta: meta("tenant-b", "sub1"),
				Spec:       apiv1.SubscriptionSpec{ClusterRef: corev1.LocalObjectReference{Name: "pg-3"}},
			},
			&apiv1.DatabaseRole{
				ObjectMeta: meta("tenant-a", "r1"),
				Spec:       apiv1.DatabaseRoleSpec{ClusterRef: corev1.LocalObjectReference{Name: "pg-1"}},
			},
			&apiv1.FailoverQuorum{ObjectMeta: meta("tenant-a", "pg-1")},
			&apiv1.ImageCatalog{ObjectMeta: meta("tenant-a", "catalog")},
			&apiv1.ClusterImageCatalog{ObjectMeta: metav1.ObjectMeta{Name: "global"}},
		}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

		Expect(gather(NewCollector(reader))).To(Equal(map[string]float64{
			`cnpg_operator_build_info{commit="` + versions.Info.Commit + `",date="` + versions.Info.Date +
				`",version="` + versions.Info.Version + `"}`: 1,
			`cnpg_operator_managed_objects{cluster="",kind="Cluster",namespace="tenant-a"}`:             2,
			`cnpg_operator_managed_objects{cluster="",kind="Cluster",namespace="tenant-b"}`:             1,
			`cnpg_operator_managed_objects{cluster="pg-1",kind="Backup",namespace="tenant-a"}`:          2,
			`cnpg_operator_managed_objects{cluster="pg-2",kind="Backup",namespace="tenant-a"}`:          1,
			`cnpg_operator_managed_objects{cluster="pg-3",kind="ScheduledBackup",namespace="tenant-b"}`: 1,
			`cnpg_operator_managed_objects{cluster="pg-1",kind="Pooler",namespace="tenant-a"}`:          1,
			`cnpg_operator_managed_objects{cluster="pg-2",kind="Database",namespace="tenant-a"}`:        1,
			`cnpg_operator_managed_objects{cluster="pg-2",kind="Publication",namespace="tenant-a"}`:     1,
			`cnpg_operator_managed_objects{cluster="pg-3",kind="Subscription",namespace="tenant-b"}`:    1,
			`cnpg_operator_managed_objects{cluster="pg-1",kind="DatabaseRole",namespace="tenant-a"}`:    1,
			`cnpg_operator_managed_objects{cluster="pg-1",kind="FailoverQuorum",namespace="tenant-a"}`:  1,
			`cnpg_operator_managed_objects{cluster="",kind="ImageCatalog",namespace="tenant-a"}`:        1,
			`cnpg_operator_managed_objects{cluster="",kind="ClusterImageCatalog",namespace=""}`:         1,
		}))
	})

	// A kind the cache cannot list is left out of the scrape; the other
	// kinds and the build info are still reported.
	It("leaves out a kind that cannot be listed and reports the others", func() {
		reader := failingReader{
			Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
				&apiv1.Cluster{ObjectMeta: meta("tenant-a", "pg-1")},
				&apiv1.Pooler{
					ObjectMeta: meta("tenant-a", "p1"),
					Spec:       apiv1.PoolerSpec{Cluster: apiv1.LocalObjectReference{Name: "pg-1"}},
				},
			).Build(),
			failing: &apiv1.PoolerList{},
		}

		series := gather(NewCollector(reader))
		Expect(series).To(HaveLen(2))
		Expect(series).To(HaveKeyWithValue(
			`cnpg_operator_managed_objects{cluster="",kind="Cluster",namespace="tenant-a"}`, 1.0))
		Expect(series).To(HaveKey(HavePrefix("cnpg_operator_build_info{")))
	})
})
