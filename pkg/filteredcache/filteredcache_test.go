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
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parsing FILTERED_CACHE_MODE", func() {
	It("accepts the three modes and nothing else", func() {
		valid := map[string]Mode{"": ModeOff, "off": ModeOff, "strict": ModeStrict, "reload": ModeReload}
		for value, expected := range valid {
			mode, err := ParseMode(value)
			Expect(err).NotTo(HaveOccurred())
			Expect(mode).To(Equal(expected))
		}
		_, err := ParseMode("shadow")
		Expect(err).To(MatchError(ContainSubstring("FILTERED_CACHE_MODE")))
		Expect(ModeOff.Enabled()).To(BeFalse())
		Expect(ModeStrict.Enabled()).To(BeTrue())
	})
})

// byKind finds the ByObject entry of a kind, whatever pointer keys it
func byKind(opts cache.Options, obj client.Object) (cache.ByObject, bool) {
	for key, value := range opts.ByObject {
		if reflect.TypeOf(key) == reflect.TypeOf(obj) {
			return value, true
		}
	}
	return cache.ByObject{}, false
}

var _ = Describe("The cache options of a filtered mode", func() {
	// The options the operator builds today: the Helm exclusion on Secrets
	stock := func() cache.Options {
		return cache.Options{ByObject: map[client.Object]cache.ByObject{
			&corev1.Secret{}: {Field: fields.OneTermNotEqualSelector("type", "helm.sh/release.v1")},
		}}
	}

	It("leaves the options alone when the mode is off", func() {
		opts := Apply(stock(), Options{Mode: ModeOff, OperatorNamespace: "cnpg-system"})
		Expect(opts.ByObject).To(HaveLen(1))
	})

	It("restricts every managed kind to the labelled objects, keeping the operator namespace whole", func() {
		opts := Apply(stock(), Options{Mode: ModeStrict, OperatorNamespace: "cnpg-system", HavePodMonitor: true})
		Expect(opts.ByObject).To(HaveLen(13))

		// Secrets: the Helm exclusion stays, the label selector is added,
		// and the operator namespace is cached unfiltered
		secrets, found := byKind(opts, &corev1.Secret{})
		Expect(found).To(BeTrue())
		Expect(secrets.Field.String()).To(Equal("type!=helm.sh/release.v1"))
		Expect(secrets.Label.String()).To(Equal("cnpg.io/reconcile=true"))
		Expect(secrets.Namespaces).To(HaveKey(cache.AllNamespaces))
		Expect(secrets.Namespaces["cnpg-system"].LabelSelector.Empty()).To(BeTrue())

		// Pods are only ever created by the operator: filtered everywhere
		pods, found := byKind(opts, &corev1.Pod{})
		Expect(found).To(BeTrue())
		Expect(pods.Label.String()).To(Equal("cnpg.io/reconcile=true"))
		Expect(pods.Namespaces).To(BeEmpty())
	})

	It("filters per watched namespace without an exemption when the operator namespace is not watched", func() {
		opts := Apply(stock(), Options{
			Mode: ModeReload, OperatorNamespace: "cnpg-system", WatchedNamespaces: []string{"a", "b"},
		})
		secrets, _ := byKind(opts, &corev1.Secret{})
		Expect(secrets.Label.String()).To(Equal("cnpg.io/reconcile=true"))
		Expect(secrets.Namespaces).To(BeEmpty())
	})

	It("exempts no namespace when the operator namespace is unknown", func() {
		// An empty namespace key would mean every namespace, unfiltered
		opts := Apply(stock(), Options{Mode: ModeStrict})
		secrets, _ := byKind(opts, &corev1.Secret{})
		Expect(secrets.Label.String()).To(Equal("cnpg.io/reconcile=true"))
		Expect(secrets.Namespaces).To(BeEmpty())
	})

	It("keeps the operator namespace whole when WATCH_NAMESPACE lists it", func() {
		// Without the exemption, the plugin Services, which the operator
		// did not create, would never reach the plugin controller
		opts := Apply(stock(), Options{
			Mode: ModeStrict, OperatorNamespace: "cnpg-system", WatchedNamespaces: []string{"a", "cnpg-system"},
		})
		for _, obj := range []client.Object{&corev1.Secret{}, &corev1.ConfigMap{}, &corev1.Service{}} {
			byObject, found := byKind(opts, obj)
			Expect(found).To(BeTrue())
			Expect(byObject.Label.String()).To(Equal("cnpg.io/reconcile=true"))
			// Per-namespace settings replace the watched namespaces for the
			// kind: every watched namespace is listed, none is added
			Expect(byObject.Namespaces).To(HaveLen(2))
			Expect(byObject.Namespaces).NotTo(HaveKey(cache.AllNamespaces))
			Expect(byObject.Namespaces["a"].LabelSelector.String()).To(Equal("cnpg.io/reconcile=true"))
			Expect(byObject.Namespaces["cnpg-system"].LabelSelector.Empty()).To(BeTrue())
		}

		// The Helm exclusion stays on Secrets
		secrets, _ := byKind(opts, &corev1.Secret{})
		Expect(secrets.Field.String()).To(Equal("type!=helm.sh/release.v1"))

		// The operator does not read foreign Pods in its namespace
		pods, _ := byKind(opts, &corev1.Pod{})
		Expect(pods.Namespaces).To(BeEmpty())
	})
})

// countingReader counts the live reads
type countingReader struct {
	client.Reader
	gets int
}

func (r *countingReader) Get(
	ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
) error {
	r.gets++
	return r.Reader.Get(ctx, key, obj, opts...)
}

var _ = Describe("The read-through client", func() {
	var (
		ctx     = context.Background()
		cached  client.Client
		reader  *countingReader
		wrapped client.Client
	)

	BeforeEach(func() {
		testScheme := runtime.NewScheme()
		Expect(scheme.AddToScheme(testScheme)).To(Succeed())

		managed := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: "cluster-app", Namespace: "tenant", Labels: map[string]string{utils.ReconcileLabelName: "true"},
		}}
		user := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-tls", Namespace: "tenant"}}
		plugin := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "plugin-tls", Namespace: "cnpg-system"}}

		// What the filtered cache holds, and what the API server holds
		cached = fake.NewClientBuilder().WithScheme(testScheme).WithObjects(managed).Build()
		reader = &countingReader{Reader: fake.NewClientBuilder().WithScheme(testScheme).
			WithObjects(managed, user, plugin).Build()}

		var err error
		wrapped, err = NewClient(cached, reader, testScheme,
			Options{Mode: ModeStrict, OperatorNamespace: "cnpg-system"})
		Expect(err).NotTo(HaveOccurred())
	})

	It("serves the managed objects from the cache and the user objects from the API server", func() {
		var secret corev1.Secret

		// A managed object: cache hit, no live read
		Expect(wrapped.Get(ctx, types.NamespacedName{Namespace: "tenant", Name: "cluster-app"}, &secret)).
			To(Succeed())
		Expect(reader.gets).To(BeZero())

		// A user object the cache does not hold: one live read
		Expect(wrapped.Get(ctx, types.NamespacedName{Namespace: "tenant", Name: "my-tls"}, &secret)).To(Succeed())
		Expect(reader.gets).To(Equal(1))

		// An object that does not exist at all: one live read, then NotFound
		err := wrapped.Get(ctx, types.NamespacedName{Namespace: "tenant", Name: "nope"}, &secret)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		Expect(reader.gets).To(Equal(2))

		// A kind that is not filtered: the cache answer is final
		var node corev1.Node
		err = wrapped.Get(ctx, types.NamespacedName{Name: "nope"}, &node)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		Expect(reader.gets).To(Equal(2))

		// The operator namespace Secrets are cached unfiltered: a miss is final there too
		err = wrapped.Get(ctx, types.NamespacedName{Namespace: "cnpg-system", Name: "plugin-tls"}, &secret)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		Expect(reader.gets).To(Equal(2))
	})

	It("reads the operator namespace like the cache options cache it", func() {
		var secret corev1.Secret
		key := types.NamespacedName{Namespace: "cnpg-system", Name: "plugin-tls"}

		// WATCH_NAMESPACE lists the operator namespace: cached whole, a miss is final
		watching, err := NewClient(cached, reader, scheme.Scheme, Options{
			Mode: ModeStrict, OperatorNamespace: "cnpg-system", WatchedNamespaces: []string{"tenant", "cnpg-system"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(apierrors.IsNotFound(watching.Get(ctx, key, &secret))).To(BeTrue())
		Expect(reader.gets).To(BeZero())

		// WATCH_NAMESPACE does not list it: not cached, read from the API server
		notWatching, err := NewClient(cached, reader, scheme.Scheme, Options{
			Mode: ModeStrict, OperatorNamespace: "cnpg-system", WatchedNamespaces: []string{"tenant"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(notWatching.Get(ctx, key, &secret)).To(Succeed())
		Expect(reader.gets).To(Equal(1))
	})

	It("is the plain client when the mode is off", func() {
		plain, err := NewClient(cached, reader, scheme.Scheme, Options{Mode: ModeOff})
		Expect(err).NotTo(HaveOccurred())
		Expect(plain).To(BeIdenticalTo(cached))
	})
})

var _ = Describe("The reload watch", func() {
	It("feeds the predicates and mappers written for full objects with a typed stub", func() {
		// A predicate and a mapper as the controllers write them
		reloadOnly := predicate.NewPredicateFuncs(func(object client.Object) bool {
			_, isSecret := object.(*corev1.Secret)
			return isSecret && object.GetLabels()[utils.WatchedLabelName] == "true"
		})
		mapper := func(_ context.Context, object client.Object) []reconcile.Request {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: object.(*corev1.Secret).Name}}}
		}

		secret := partial(corev1.SchemeGroupVersion.WithKind("Secret"))
		secret.Name = "app-creds"
		secret.Labels = map[string]string{utils.WatchedLabelName: "true"}

		Expect(stubPredicate([]predicate.Predicate{reloadOnly}).Update(
			event.TypedUpdateEvent[*metav1.PartialObjectMetadata]{ObjectOld: secret, ObjectNew: secret})).To(BeTrue())
		Expect(mapper(context.Background(), stub(secret))[0].Name).To(Equal("app-creds"))

		// Without the label, the stock predicate says no, as on the full object
		unlabelled := partial(corev1.SchemeGroupVersion.WithKind("Secret"))
		Expect(stubPredicate([]predicate.Predicate{reloadOnly}).Create(
			event.TypedCreateEvent[*metav1.PartialObjectMetadata]{Object: unlabelled})).To(BeFalse())

		// Only the identity, the labels and the owners survive the transform
		secret.Annotations = map[string]string{"a": "b"}
		secret.ManagedFields = []metav1.ManagedFieldsEntry{{Manager: "kubectl"}}
		stripped, err := stripMetadata(secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(stripped.(*metav1.PartialObjectMetadata).Annotations).To(BeNil())
		Expect(stripped.(*metav1.PartialObjectMetadata).ManagedFields).To(BeNil())
		Expect(stripped.(*metav1.PartialObjectMetadata).Labels).To(HaveKey(utils.WatchedLabelName))
	})
})
