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

package filteredcache

import (
	"reflect"
	"slices"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// Options describe the installation the filter applies to
type Options struct {
	// Mode is the value of FILTERED_CACHE_MODE
	Mode Mode

	// OperatorNamespace is the namespace of the operator, where the plugin
	// Services and Secrets, the pull secret and the default monitoring
	// queries live: those are read by the operator but not created by it,
	// so they are cached unfiltered there, when the operator watches it
	OperatorNamespace string

	// WatchedNamespaces is the list of WATCH_NAMESPACE, empty when the
	// operator watches the whole cluster
	WatchedNamespaces []string

	// HavePodMonitor and HaveVolumeSnapshot tell whether those APIs exist:
	// a kind absent from the API server cannot be configured in the cache
	HavePodMonitor     bool
	HaveVolumeSnapshot bool
}

// ManagedSelector selects the objects the operator manages
func ManagedSelector() labels.Selector {
	return labels.SelectorFromSet(labels.Set{utils.ReconcileLabelName: "true"})
}

// UnmanagedSelector selects the objects the operator does not manage
func UnmanagedSelector() labels.Selector {
	requirement, err := labels.NewRequirement(utils.ReconcileLabelName, selection.NotEquals, []string{"true"})
	if err != nil {
		// a constant key and a constant value cannot fail to parse
		panic(err)
	}
	return labels.NewSelector().Add(*requirement)
}

// FilteredObjects lists the kinds whose informers are restricted to the
// managed objects: every kind the operator creates and caches
func FilteredObjects(o Options) []client.Object {
	objects := []client.Object{
		&corev1.Secret{},
		&corev1.ConfigMap{},
		&corev1.Pod{},
		&corev1.PersistentVolumeClaim{},
		&corev1.Service{},
		&corev1.ServiceAccount{},
		&batchv1.Job{},
		&coordinationv1.Lease{},
		&policyv1.PodDisruptionBudget{},
		&appsv1.Deployment{},
		&rbacv1.Role{},
		&rbacv1.RoleBinding{},
	}
	if o.HavePodMonitor {
		objects = append(objects, &monitoringv1.PodMonitor{})
	}
	if o.HaveVolumeSnapshot {
		objects = append(objects, &volumesnapshotv1.VolumeSnapshot{})
	}
	return objects
}

// unfilteredInOperatorNamespace tells whether a kind holds, in the operator
// namespace, objects the operator reads without having created them
func unfilteredInOperatorNamespace(obj client.Object) bool {
	switch obj.(type) {
	case *corev1.Secret, *corev1.ConfigMap, *corev1.Service:
		return true
	default:
		return false
	}
}

// Apply restricts the informers of the filtered kinds to the managed
// objects, keeping the other settings of the cache options
func Apply(opts cache.Options, o Options) cache.Options {
	if !o.Mode.Enabled() {
		return opts
	}
	if opts.ByObject == nil {
		opts.ByObject = map[client.Object]cache.ByObject{}
	}

	selector := ManagedSelector()
	for _, obj := range FilteredObjects(o) {
		key, byObject := lookup(opts.ByObject, obj)
		byObject.Label = selector
		if namespace := unfilteredNamespace(o); namespace != "" && unfilteredInOperatorNamespace(obj) {
			byObject.Namespaces = exemptNamespace(o, namespace, selector)
		}
		opts.ByObject[key] = byObject
	}
	return opts
}

// exemptNamespace returns the per-namespace settings of a kind cached
// unfiltered in the given namespace and filtered elsewhere. A kind with
// per-namespace settings is only cached in those namespaces, so with
// WATCH_NAMESPACE every watched namespace is listed.
func exemptNamespace(o Options, namespace string, selector labels.Selector) map[string]cache.Config {
	namespaces := map[string]cache.Config{}
	if len(o.WatchedNamespaces) == 0 {
		namespaces[cache.AllNamespaces] = cache.Config{LabelSelector: selector}
	}
	for _, watched := range o.WatchedNamespaces {
		namespaces[watched] = cache.Config{LabelSelector: selector}
	}
	namespaces[namespace] = cache.Config{LabelSelector: labels.Everything()}
	return namespaces
}

// lookup finds the entry of the same kind in a ByObject map, whose keys are
// pointers, or returns the object itself as a new key
func lookup(byObject map[client.Object]cache.ByObject, obj client.Object) (client.Object, cache.ByObject) {
	for key, value := range byObject {
		if reflect.TypeOf(key) == reflect.TypeOf(obj) {
			return key, value
		}
	}
	return obj, cache.ByObject{}
}

// FilteredKinds returns the GroupVersionKinds of the filtered objects
func FilteredKinds(o Options, scheme *runtime.Scheme) (map[schema.GroupVersionKind]struct{}, error) {
	kinds := map[schema.GroupVersionKind]struct{}{}
	if !o.Mode.Enabled() {
		return kinds, nil
	}
	for _, obj := range FilteredObjects(o) {
		gvk, err := apiutil.GVKForObject(obj, scheme)
		if err != nil {
			return nil, err
		}
		kinds[gvk] = struct{}{}
	}
	return kinds, nil
}

// unfilteredNamespace returns the namespace whose Secrets, ConfigMaps and
// Services are cached unfiltered, or "" when there is none: the operator
// namespace, when the operator watches it. Otherwise the operator namespace
// objects are not cached, and are read from the API server.
func unfilteredNamespace(o Options) string {
	if len(o.WatchedNamespaces) > 0 && !slices.Contains(o.WatchedNamespaces, o.OperatorNamespace) {
		return ""
	}
	return o.OperatorNamespace
}
