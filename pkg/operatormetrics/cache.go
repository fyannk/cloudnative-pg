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
	"strconv"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

var (
	cachedObjectsDesc = prometheus.NewDesc(
		"cnpg_operator_cached_objects",
		"Number of objects held in the operator cache, per kind, split on whether the operator manages "+
			"them (cnpg.io/reconcile=true)",
		[]string{"kind", "managed"},
		nil,
	)

	metadataCachedObjectsDesc = prometheus.NewDesc(
		"cnpg_operator_metadata_cached_objects",
		"Number of user objects carrying cnpg.io/reload whose metadata only is watched, per kind",
		[]string{"kind"},
		nil,
	)
)

// MetadataCounter counts the objects held as metadata only, per kind
type MetadataCounter interface {
	CachedObjects(ctx context.Context) (map[string]int, error)
}

// CacheCollector counts, when scraped, the objects the operator cache holds
// for the kinds the operator creates, without copying them
type CacheCollector struct {
	reader   client.Reader
	scheme   *runtime.Scheme
	objects  []client.Object
	metadata MetadataCounter
}

// NewCacheCollector creates a collector counting, in the given reader, the
// objects of the given kinds, and the metadata counted by metadata when not nil
func NewCacheCollector(
	reader client.Reader, scheme *runtime.Scheme, objects []client.Object, metadata MetadataCounter,
) *CacheCollector {
	return &CacheCollector{reader: reader, scheme: scheme, objects: objects, metadata: metadata}
}

// RegisterCache registers a cache collector on the metrics registry of controller-runtime
func RegisterCache(
	reader client.Reader, scheme *runtime.Scheme, objects []client.Object, metadata MetadataCounter,
) error {
	return metrics.Registry.Register(NewCacheCollector(reader, scheme, objects, metadata))
}

// Describe implements prometheus.Collector
func (c *CacheCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- cachedObjectsDesc
	ch <- metadataCachedObjectsDesc
}

// Collect implements prometheus.Collector. A kind that cannot be listed is
// left out of the scrape, and logged.
func (c *CacheCollector) Collect(ch chan<- prometheus.Metric) {
	ctx := context.Background()
	contextLogger := log.FromContext(ctx)

	for _, object := range c.objects {
		gvk, err := apiutil.GVKForObject(object, c.scheme)
		if err != nil {
			contextLogger.Error(err, "while counting the cached objects, kind left out")
			continue
		}
		newList, err := c.scheme.New(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
		if err != nil {
			contextLogger.Error(err, "while counting the cached objects, kind left out", "kind", gvk.Kind)
			continue
		}
		list := newList.(client.ObjectList)
		// The objects are only counted: the cached objects are read in place
		if err := c.reader.List(ctx, list, client.UnsafeDisableDeepCopy); err != nil {
			contextLogger.Error(err, "while counting the cached objects, kind left out", "kind", gvk.Kind)
			continue
		}

		counts := map[bool]int{true: 0, false: 0}
		_ = meta.EachListItem(list, func(obj runtime.Object) error {
			managed := obj.(client.Object).GetLabels()[utils.ReconcileLabelName] == "true"
			counts[managed]++
			return nil
		})
		for managed, count := range counts {
			ch <- prometheus.MustNewConstMetric(cachedObjectsDesc, prometheus.GaugeValue, float64(count),
				gvk.Kind, strconv.FormatBool(managed))
		}
	}

	if c.metadata == nil {
		return
	}
	counts, err := c.metadata.CachedObjects(ctx)
	if err != nil {
		contextLogger.Error(err, "while counting the metadata cached objects, left out")
		return
	}
	for kind, count := range counts {
		ch <- prometheus.MustNewConstMetric(metadataCachedObjectsDesc, prometheus.GaugeValue, float64(count), kind)
	}
}
