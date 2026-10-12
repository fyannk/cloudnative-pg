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

// Package operatormetrics exposes what the operator manages and its build as
// Prometheus metrics, on the metrics endpoint of the manager
package operatormetrics

import (
	"context"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"
)

var (
	managedObjectsDesc = prometheus.NewDesc(
		"cnpg_operator_managed_objects",
		"Number of objects of each CloudNativePG kind the operator manages, per namespace and, "+
			"for the kinds that reference one, per Cluster",
		[]string{"kind", "namespace", "cluster"},
		nil,
	)

	buildInfoDesc = prometheus.NewDesc(
		"cnpg_operator_build_info",
		"Build of the running operator",
		[]string{"version", "commit", "date"},
		nil,
	)
)

// managedKind is a CloudNativePG kind counted by the collector
type managedKind struct {
	name    string
	newList func() client.ObjectList
	// cluster gives the Cluster an object belongs to, for the kinds that
	// reference one
	cluster func(client.Object) string
}

// managedKinds lists every CloudNativePG kind, in the order of the output
var managedKinds = []managedKind{
	{
		name:    "Cluster",
		newList: func() client.ObjectList { return &apiv1.ClusterList{} },
	},
	{
		name:    "Backup",
		newList: func() client.ObjectList { return &apiv1.BackupList{} },
		cluster: func(o client.Object) string { return o.(*apiv1.Backup).Spec.Cluster.Name },
	},
	{
		name:    "ScheduledBackup",
		newList: func() client.ObjectList { return &apiv1.ScheduledBackupList{} },
		cluster: func(o client.Object) string { return o.(*apiv1.ScheduledBackup).Spec.Cluster.Name },
	},
	{
		name:    "Pooler",
		newList: func() client.ObjectList { return &apiv1.PoolerList{} },
		cluster: func(o client.Object) string { return o.(*apiv1.Pooler).Spec.Cluster.Name },
	},
	{
		name:    "Database",
		newList: func() client.ObjectList { return &apiv1.DatabaseList{} },
		cluster: func(o client.Object) string { return o.(*apiv1.Database).Spec.ClusterRef.Name },
	},
	{
		name:    "Publication",
		newList: func() client.ObjectList { return &apiv1.PublicationList{} },
		cluster: func(o client.Object) string { return o.(*apiv1.Publication).Spec.ClusterRef.Name },
	},
	{
		name:    "Subscription",
		newList: func() client.ObjectList { return &apiv1.SubscriptionList{} },
		cluster: func(o client.Object) string { return o.(*apiv1.Subscription).Spec.ClusterRef.Name },
	},
	{
		name:    "DatabaseRole",
		newList: func() client.ObjectList { return &apiv1.DatabaseRoleList{} },
		cluster: func(o client.Object) string { return o.(*apiv1.DatabaseRole).Spec.ClusterRef.Name },
	},
	{
		name:    "FailoverQuorum",
		newList: func() client.ObjectList { return &apiv1.FailoverQuorumList{} },
		// A failover quorum is named after its Cluster
		cluster: func(o client.Object) string { return o.GetName() },
	},
	{
		name:    "ImageCatalog",
		newList: func() client.ObjectList { return &apiv1.ImageCatalogList{} },
	},
	{
		name:    "ClusterImageCatalog",
		newList: func() client.ObjectList { return &apiv1.ClusterImageCatalogList{} },
	},
}

// Collector counts, when scraped, the CloudNativePG objects held by a reader,
// the cache of the manager in the operator: every replica reports its own
// cache, with no refresh to schedule and no stale value to keep
type Collector struct {
	reader client.Reader
}

// NewCollector creates a collector counting the objects of the given reader
func NewCollector(reader client.Reader) *Collector {
	return &Collector{reader: reader}
}

// Register registers a collector on the metrics registry of controller-runtime
func Register(reader client.Reader) error {
	return metrics.Registry.Register(NewCollector(reader))
}

// Describe implements prometheus.Collector
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- managedObjectsDesc
	ch <- buildInfoDesc
}

// scope is the label set of a count: a namespace, and a Cluster name for the
// kinds that reference one
type scope struct {
	namespace string
	cluster   string
}

// Collect implements prometheus.Collector. A kind that cannot be listed is
// left out of the scrape, and logged.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(buildInfoDesc, prometheus.GaugeValue, 1,
		versions.Info.Version, versions.Info.Commit, versions.Info.Date)

	ctx := context.Background()
	for _, kind := range managedKinds {
		// The objects are only counted: the cached objects are read in place
		list := kind.newList()
		if err := c.reader.List(ctx, list, client.UnsafeDisableDeepCopy); err != nil {
			log.FromContext(ctx).Error(err, "while counting the objects the operator manages, kind left out",
				"kind", kind.name)
			continue
		}

		counts := make(map[scope]int)
		_ = meta.EachListItem(list, func(obj runtime.Object) error {
			object := obj.(client.Object)
			key := scope{namespace: object.GetNamespace()}
			if kind.cluster != nil {
				key.cluster = kind.cluster(object)
			}
			counts[key]++
			return nil
		})
		for key, count := range counts {
			ch <- prometheus.MustNewConstMetric(managedObjectsDesc, prometheus.GaugeValue, float64(count),
				kind.name, key.namespace, key.cluster)
		}
	}
}
