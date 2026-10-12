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
	"context"
	"fmt"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	toolscache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// ReloadWatch is a metadata-only cache of the Secrets and ConfigMaps that
// carry cnpg.io/reload=true without the cache label: the user objects the
// operator must notice changes of, held as a few hundred bytes each. The
// controllers watch it with the predicates and mappers they use on the
// full objects, through a typed stub carrying the metadata.
type ReloadWatch struct {
	cache  cache.Cache
	scheme *runtime.Scheme
}

// ReloadWatchConfig is what the metadata cache shares with the manager
type ReloadWatchConfig struct {
	HTTPClient *http.Client
	Scheme     *runtime.Scheme
	Mapper     meta.RESTMapper
}

// reloadSelector selects the user objects to watch
func reloadSelector() (labels.Selector, error) {
	reload, err := labels.NewRequirement(utils.WatchedLabelName, selection.Equals, []string{"true"})
	if err != nil {
		return nil, err
	}
	unmanaged, err := labels.NewRequirement(utils.ReconcileLabelName, selection.NotEquals, []string{"true"})
	if err != nil {
		return nil, err
	}
	return labels.NewSelector().Add(*reload, *unmanaged), nil
}

// watchedKinds are the kinds the reload label applies to
func watchedKinds() []schema.GroupVersionKind {
	return []schema.GroupVersionKind{
		corev1.SchemeGroupVersion.WithKind("Secret"),
		corev1.SchemeGroupVersion.WithKind("ConfigMap"),
	}
}

// NewReloadWatch builds the metadata cache for the reload mode, or returns
// nil in the other modes
func NewReloadWatch(config *rest.Config, cfg ReloadWatchConfig, o Options) (*ReloadWatch, error) {
	if o.Mode != ModeReload {
		return nil, nil
	}
	selector, err := reloadSelector()
	if err != nil {
		return nil, err
	}

	byObject := map[client.Object]cache.ByObject{}
	for _, gvk := range watchedKinds() {
		byObject[partial(gvk)] = cache.ByObject{Label: selector, Transform: stripMetadata}
	}
	options := cache.Options{
		HTTPClient:                  cfg.HTTPClient,
		Scheme:                      cfg.Scheme,
		Mapper:                      cfg.Mapper,
		ByObject:                    byObject,
		ReaderFailOnMissingInformer: true,
	}
	if len(o.WatchedNamespaces) > 0 {
		options.DefaultNamespaces = map[string]cache.Config{}
		for _, namespace := range o.WatchedNamespaces {
			options.DefaultNamespaces[namespace] = cache.Config{}
		}
	}

	c, err := cache.New(config, options)
	if err != nil {
		return nil, fmt.Errorf("while creating the reload metadata cache: %w", err)
	}
	return &ReloadWatch{cache: c, scheme: cfg.Scheme}, nil
}

// Start registers the informers and runs the cache until the context ends
func (w *ReloadWatch) Start(ctx context.Context) error {
	for _, gvk := range watchedKinds() {
		if _, err := w.cache.GetInformer(ctx, partial(gvk), cache.BlockUntilSynced(false)); err != nil {
			return err
		}
	}
	return w.cache.Start(ctx)
}

// NeedLeaderElection makes the cache run on every replica, like the manager cache
func (w *ReloadWatch) NeedLeaderElection() bool {
	return false
}

// Source returns a watch on the metadata of the given kind, Secret or
// ConfigMap, feeding the map function and the predicates written for the
// full objects
func (w *ReloadWatch) Source(
	obj client.Object, mapFunc handler.MapFunc, predicates ...predicate.Predicate,
) (source.Source, error) {
	gvk, err := apiutil.GVKForObject(obj, w.scheme)
	if err != nil {
		return nil, err
	}
	adapter := handler.TypedEnqueueRequestsFromMapFunc(
		func(ctx context.Context, object *metav1.PartialObjectMetadata) []reconcile.Request {
			return mapFunc(ctx, stub(object))
		})
	return source.Kind(w.cache, partial(gvk), adapter, stubPredicate(predicates)), nil
}

// partial is an empty PartialObjectMetadata of the given kind
func partial(gvk schema.GroupVersionKind) *metav1.PartialObjectMetadata {
	object := &metav1.PartialObjectMetadata{}
	object.SetGroupVersionKind(gvk)
	return object
}

// stub returns an empty typed object carrying the metadata, so that the
// predicates and mappers written for the full object accept it
func stub(object *metav1.PartialObjectMetadata) client.Object {
	switch object.GroupVersionKind().Kind {
	case "Secret":
		return &corev1.Secret{TypeMeta: object.TypeMeta, ObjectMeta: object.ObjectMeta}
	case "ConfigMap":
		return &corev1.ConfigMap{TypeMeta: object.TypeMeta, ObjectMeta: object.ObjectMeta}
	default:
		return object
	}
}

// stubPredicate applies predicates written for full objects to metadata events
func stubPredicate(predicates []predicate.Predicate) predicate.TypedPredicate[*metav1.PartialObjectMetadata] {
	return predicate.TypedFuncs[*metav1.PartialObjectMetadata]{
		CreateFunc: func(e event.TypedCreateEvent[*metav1.PartialObjectMetadata]) bool {
			typed := event.CreateEvent{Object: stub(e.Object), IsInInitialList: e.IsInInitialList}
			for _, p := range predicates {
				if !p.Create(typed) {
					return false
				}
			}
			return true
		},
		UpdateFunc: func(e event.TypedUpdateEvent[*metav1.PartialObjectMetadata]) bool {
			typed := event.UpdateEvent{ObjectOld: stub(e.ObjectOld), ObjectNew: stub(e.ObjectNew)}
			for _, p := range predicates {
				if !p.Update(typed) {
					return false
				}
			}
			return true
		},
		DeleteFunc: func(e event.TypedDeleteEvent[*metav1.PartialObjectMetadata]) bool {
			typed := event.DeleteEvent{Object: stub(e.Object), DeleteStateUnknown: e.DeleteStateUnknown}
			for _, p := range predicates {
				if !p.Delete(typed) {
					return false
				}
			}
			return true
		},
		GenericFunc: func(e event.TypedGenericEvent[*metav1.PartialObjectMetadata]) bool {
			typed := event.GenericEvent{Object: stub(e.Object)}
			for _, p := range predicates {
				if !p.Generic(typed) {
					return false
				}
			}
			return true
		},
	}
}

// stripMetadata keeps what the predicates and mappers look at: the
// identity, the labels and the owners. Annotations and managedFields go.
func stripMetadata(obj interface{}) (interface{}, error) {
	object, ok := obj.(*metav1.PartialObjectMetadata)
	if !ok {
		return obj, nil
	}
	object.Annotations = nil
	object.ManagedFields = nil
	object.Finalizers = nil
	return object, nil
}

var _ toolscache.TransformFunc = stripMetadata
