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
	"errors"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	k8client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/certs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// quotaRejection builds the error the API server returns when a ResourceQuota
// is full: a Forbidden status whose message is the text of the quota plugin
func quotaRejection(resource, name string) error {
	return apierrs.NewForbidden(schema.GroupResource{Resource: resource}, name,
		fmt.Errorf("exceeded quota: no-%s, requested: count/%s=1, used: count/%s=0, limited: count/%s=0",
			resource, resource, resource, resource))
}

// rejectCreatesOf wraps a client so that the creation of the given kind is
// rejected with the error built by reject
func rejectCreatesOf[T k8client.Object](cli k8client.WithWatch, reject func(name string) error) k8client.Client {
	return interceptor.NewClient(cli, interceptor.Funcs{
		Create: func(ctx context.Context, cl k8client.WithWatch, obj k8client.Object,
			opts ...k8client.CreateOption,
		) error {
			if _, matches := obj.(T); matches {
				return reject(obj.GetName())
			}
			return cl.Create(ctx, obj, opts...)
		},
	})
}

// recordedEvents returns the events recorded so far
func recordedEvents(recorder *record.FakeRecorder) []string {
	var events []string
	for {
		select {
		case e := <-recorder.Events:
			events = append(events, e)
		default:
			return events
		}
	}
}

// When an API call of the operator is rejected, the reconciliation ends with
// an error that the manager logs as "Reconciler error" in the operator
// namespace. The owner of the object usually cannot read it: the Cluster,
// Pooler, Backup and ScheduledBackup reconcilers report the error as a
// Warning on the object, with the API server message, classified by cause.
var _ = Describe("reporting the error that ends a reconciliation", func() {
	DescribeTable("classifies the error by cause, skipping the ones retried at once",
		func(err error, reason string) {
			Expect(reconciliationErrorReason(err)).To(Equal(reason))
		},
		Entry("no error", nil, ""),
		Entry("loop control", fmt.Errorf("wrapped: %w", utils.ErrNextLoop), ""),
		Entry("loop termination", utils.ErrTerminateLoop, ""),
		Entry("optimistic lock conflict",
			apierrs.NewConflict(schema.GroupResource{Resource: "clusters"}, "c", errors.New("modified")), ""),
		Entry("cancelled context", context.Canceled, ""),
		Entry("terminal error, already reported by the admission guard",
			reconcile.TerminalError(errors.New("invalid")), ""),
		Entry("full quota", quotaRejection("pods", "c-1"), eventReasonQuotaExceeded),
		Entry("quota without a limit for the request",
			apierrs.NewForbidden(schema.GroupResource{Resource: "pods"}, "c-1",
				errors.New("failed quota: limits: must specify limits.memory")), eventReasonQuotaExceeded),
		Entry("admission webhook",
			apierrs.NewForbidden(schema.GroupResource{Resource: "pods"}, "c-1",
				errors.New("admission webhook \"policy.example.com\" denied the request: no root")),
			eventReasonAdmissionDenied),
		Entry("pod security",
			apierrs.NewForbidden(schema.GroupResource{Resource: "pods"}, "c-1",
				errors.New("violates PodSecurity \"restricted:latest\": runAsNonRoot != true")),
			eventReasonAdmissionDenied),
		Entry("RBAC",
			apierrs.NewForbidden(schema.GroupResource{Resource: "pods"}, "c-1",
				errors.New("User \"system:serviceaccount:cnpg-system:cnpg-manager\" cannot create resource")),
			eventReasonReconciliationFailed),
		Entry("invalid object",
			apierrs.NewInvalid(schema.GroupKind{Kind: "Pod"}, "c-1", nil), eventReasonReconciliationFailed),
		Entry("any other error", errors.New("unable to create Pod: boom"), eventReasonReconciliationFailed),
	)

	It("reports on the Cluster the creation rejected by a quota", func(ctx SpecContext) {
		env := buildTestEnvironment()
		namespace := newFakeNamespace(env.client)
		recorder := env.clusterReconciler.Recorder.(*record.FakeRecorder)
		env.clusterReconciler.Client = rejectCreatesOf[*corev1.Secret](env.client, func(name string) error {
			return quotaRejection("secrets", name)
		})
		pair, err := certs.GenerateSelfSignedClientCertificate("test-operator")
		Expect(err).ToNot(HaveOccurred())
		tlsCert, err := pair.TLSCertificate()
		Expect(err).ToNot(HaveOccurred())
		env.clusterReconciler.OperatorClientCert = &tlsCert
		// No user-provided CA: the operator creates its own, the first
		// Secret of the Cluster
		cluster := newFakeCNPGCluster(env.client, namespace, func(cluster *apiv1.Cluster) {
			cluster.Spec.Certificates = nil
			cluster.Status.Certificates = apiv1.CertificatesStatus{}
		})
		request := ctrl.Request{NamespacedName: k8client.ObjectKeyFromObject(cluster)}

		// The first passes only update the status: run until the one that
		// creates the CA Secret, which the quota rejects
		for range 10 {
			if _, err = env.clusterReconciler.Reconcile(ctx, request); err != nil {
				break
			}
		}
		Expect(err).To(HaveOccurred())
		Expect(recordedEvents(recorder)).To(ContainElement(SatisfyAll(
			HavePrefix("Warning QuotaExceeded"),
			ContainSubstring("exceeded quota: no-secrets"),
		)))
	})

	It("reports on the Pooler the Deployment rejected by a quota", func(ctx SpecContext) {
		env := buildTestEnvironment()
		namespace := newFakeNamespace(env.client)
		recorder := env.poolerReconciler.Recorder.(*record.FakeRecorder)
		env.poolerReconciler.Client = rejectCreatesOf[*appsv1.Deployment](env.client, func(name string) error {
			return quotaRejection("deployments.apps", name)
		})
		cluster := newFakeCNPGCluster(env.client, namespace)
		pooler := newFakePooler(env.client, cluster)
		for _, name := range []string{
			pooler.GetAuthQuerySecretName(),
			pooler.GetClientTLSSecretNameOrDefault(cluster),
			pooler.GetClientCASecretNameOrDefault(cluster),
			pooler.GetServerCASecretNameOrDefault(cluster),
		} {
			Expect(env.client.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			})).To(Succeed())
		}
		request := ctrl.Request{NamespacedName: k8client.ObjectKeyFromObject(pooler)}

		_, err := env.poolerReconciler.Reconcile(ctx, request)
		Expect(err).To(HaveOccurred())
		Expect(recordedEvents(recorder)).To(ContainElement(SatisfyAll(
			HavePrefix("Warning QuotaExceeded"),
			ContainSubstring("exceeded quota: no-deployments.apps"),
		)))
	})

	It("reports on the ScheduledBackup the Backup rejected by a quota", func(ctx SpecContext) {
		cli := newScheduledBackupTestClient()
		namespace := newFakeNamespace(cli)
		recorder := record.NewFakeRecorder(10)
		r := &ScheduledBackupReconciler{
			Client: rejectCreatesOf[*apiv1.Backup](cli, func(name string) error {
				return quotaRejection("backups.postgresql.cnpg.io", name)
			}),
			Recorder: recorder,
		}
		scheduledBackup := &apiv1.ScheduledBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "daily", Namespace: namespace},
			Spec: apiv1.ScheduledBackupSpec{
				Schedule:  "0 0 0 * * *",
				Immediate: ptr.To(true),
				Cluster:   apiv1.LocalObjectReference{Name: "cluster-x"},
			},
		}
		Expect(cli.Create(ctx, scheduledBackup)).To(Succeed())
		request := ctrl.Request{NamespacedName: k8client.ObjectKeyFromObject(scheduledBackup)}

		// The immediate Backup is rejected: the existing BackupCreation
		// warning stays, the cause is added next to it
		_, err := r.Reconcile(ctx, request)
		Expect(err).To(HaveOccurred())
		Expect(recordedEvents(recorder)).To(ContainElements(
			HavePrefix("Warning BackupCreation"),
			SatisfyAll(
				HavePrefix("Warning QuotaExceeded"),
				ContainSubstring("exceeded quota: no-backups.postgresql.cnpg.io"),
			),
		))
	})

	It("reports on the Backup a Secret it is not allowed to read", func(ctx SpecContext) {
		env := buildTestEnvironment()
		namespace := newFakeNamespace(env.client)
		recorder := env.backupReconciler.Recorder.(*record.FakeRecorder)
		env.backupReconciler.Client = interceptor.NewClient(env.client, interceptor.Funcs{
			Get: func(ctx context.Context, cl k8client.WithWatch, key types.NamespacedName,
				obj k8client.Object, opts ...k8client.GetOption,
			) error {
				if _, isSecret := obj.(*corev1.Secret); isSecret {
					return apierrs.NewForbidden(schema.GroupResource{Resource: "secrets"}, key.Name,
						errors.New("User \"system:serviceaccount:cnpg-system:cnpg-manager\" cannot get resource"))
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		})
		cluster := newFakeCNPGCluster(env.client, namespace, func(cluster *apiv1.Cluster) {
			cluster.Spec.Backup = &apiv1.BackupConfiguration{
				BarmanObjectStore: &apiv1.BarmanObjectStoreConfiguration{DestinationPath: "s3://backups/"},
			}
		})
		backup := &apiv1.Backup{
			ObjectMeta: metav1.ObjectMeta{Name: "manual", Namespace: namespace},
			Spec:       apiv1.BackupSpec{Cluster: apiv1.LocalObjectReference{Name: cluster.Name}},
		}
		Expect(env.client.Create(ctx, backup)).To(Succeed())
		request := ctrl.Request{NamespacedName: k8client.ObjectKeyFromObject(backup)}

		// The server CA Secret cannot be read: the pass ends there, and the
		// Backup says so
		_, err := env.backupReconciler.Reconcile(ctx, request)
		Expect(err).To(HaveOccurred())
		Expect(recordedEvents(recorder)).To(ContainElement(SatisfyAll(
			HavePrefix("Warning ReconciliationFailed"),
			ContainSubstring("cannot get resource"),
		)))
	})
})
