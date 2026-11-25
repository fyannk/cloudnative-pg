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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	testclock "k8s.io/utils/clock/testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The client limits the rate of the events per component and object, 25 in a
// burst: the events added by this fork must not use up the budget of the ones
// of the upstream operator
var _ = Describe("event recorder", func() {
	event := func(component, reason, message string) *corev1.Event {
		return &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{Name: message, Namespace: "default"},
			Source:         corev1.EventSource{Component: component},
			InvolvedObject: corev1.ObjectReference{Kind: "Backup", Name: "backup", Namespace: "default", UID: "1"},
			Type:           corev1.EventTypeWarning,
			Reason:         reason,
			Message:        message,
			Count:          1,
			FirstTimestamp: metav1.Now(),
			LastTimestamp:  metav1.Now(),
		}
	}

	DescribeTable("the budget of the other events, after 30 diagnostics",
		func(component string, skipped bool) {
			correlator := record.NewEventCorrelator(testclock.NewFakePassiveClock(time.Now()))
			for i := range 30 {
				_, err := correlator.EventCorrelate(event(component, "VolumeSnapshotError", fmt.Sprintf("retry %d", i)))
				Expect(err).ToNot(HaveOccurred())
			}
			result, err := correlator.EventCorrelate(event("cloudnative-pg-backup", "Error", "failed"))
			Expect(err).ToNot(HaveOccurred())
			Expect(result.Skip).To(Equal(skipped))
		},
		Entry("is used up by the same component", "cloudnative-pg-backup", true),
		Entry("is left alone by a component of their own",
			"cloudnative-pg-backup"+diagnosticsComponentSuffix, false),
	)

	It("sends the diagnostics, and nothing else, to their own component", func() {
		stock, diagnostics := record.NewFakeRecorder(10), record.NewFakeRecorder(10)
		recorder := &eventRecorder{EventRecorder: stock, diagnostics: diagnostics}

		recorder.Event(nil, "Warning", "VolumeSnapshotError", "one")
		recorder.Eventf(nil, "Warning", "BackupDelayed", "%s", "two")
		recorder.AnnotatedEventf(nil, nil, "Warning", "ReconciliationFailed", "%s", "three")
		recorder.Event(nil, "Warning", "Error", "four")
		recorder.Eventf(nil, "Warning", "BackupPending", "%s", "five")

		Expect(recordedEvents(diagnostics)).To(HaveLen(3))
		Expect(recordedEvents(stock)).To(HaveExactElements("Warning Error four", "Warning BackupPending five"))
	})

	It("tells when a state begins", func() {
		var tracker stateTracker
		Expect(tracker.begins("a", "x")).To(BeTrue())
		Expect(tracker.begins("a", "x")).To(BeFalse())
		Expect(tracker.begins("b", "x")).To(BeTrue())
		Expect(tracker.begins("a", "y")).To(BeTrue())
		Expect(tracker.begins("a", "")).To(BeFalse())
		Expect(tracker.begins("a", "y")).To(BeTrue())
	})
})
