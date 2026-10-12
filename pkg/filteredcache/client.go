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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// readThroughClient reads from the cache, and from the API server the
// objects of the filtered kinds the cache does not hold: a cache miss on a
// filtered kind can be an object the operator does not manage
type readThroughClient struct {
	client.Client
	reader              client.Reader
	filtered            map[schema.GroupVersionKind]struct{}
	unfilteredNamespace string
	unfilteredKinds     map[schema.GroupVersionKind]struct{}
}

// NewClient wraps the manager client. In a filtered mode a Get that misses
// the cache for a filtered kind is retried on the API server, so that user
// objects without the cache label are still found. Writes are untouched.
func NewClient(c client.Client, reader client.Reader, scheme *runtime.Scheme, o Options) (client.Client, error) {
	if !o.Mode.Enabled() {
		return c, nil
	}
	filtered, err := FilteredKinds(o, scheme)
	if err != nil {
		return nil, err
	}
	unfilteredKinds := map[schema.GroupVersionKind]struct{}{}
	for _, obj := range FilteredObjects(o) {
		if !unfilteredInOperatorNamespace(obj) {
			continue
		}
		gvk, err := apiutil.GVKForObject(obj, scheme)
		if err != nil {
			return nil, err
		}
		unfilteredKinds[gvk] = struct{}{}
	}
	return &readThroughClient{
		Client:              c,
		reader:              reader,
		filtered:            filtered,
		unfilteredNamespace: unfilteredNamespace(o),
		unfilteredKinds:     unfilteredKinds,
	}, nil
}

// Get reads from the cache, then from the API server when the cache does
// not hold an object of a filtered kind
func (c *readThroughClient) Get(
	ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
) error {
	err := c.Client.Get(ctx, key, obj, opts...)
	if !apierrors.IsNotFound(err) || !c.readThrough(key, obj) {
		return err
	}
	return c.reader.Get(ctx, key, obj, opts...)
}

// readThrough tells whether a cache miss on this object is final
func (c *readThroughClient) readThrough(key client.ObjectKey, obj client.Object) bool {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return false
	}
	if _, filtered := c.filtered[gvk]; !filtered {
		return false
	}
	// The kinds cached unfiltered in the operator namespace are complete there
	if key.Namespace == c.unfilteredNamespace && c.unfilteredNamespace != "" {
		if _, unfiltered := c.unfilteredKinds[gvk]; unfiltered {
			return false
		}
	}
	return true
}
