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
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
)

// diagnosticsComponentSuffix names the event component of the diagnostics
const diagnosticsComponentSuffix = "-diagnostics"

// diagnosticsReasons are the reasons of the events this fork adds to the ones
// of the upstream operator
var diagnosticsReasons = map[string]struct{}{
	eventReasonQuotaExceeded:                   {},
	eventReasonAdmissionDenied:                 {},
	eventReasonReconciliationFailed:            {},
	"BackupDelayed":                            {},
	"BackupWaiting":                            {},
	"DefaultMonitoringQueriesError":            {},
	"InstanceCreated":                          {},
	"InstanceCreationFailed":                   {},
	"MajorUpgradeFailed":                       {},
	"NodeStatusUnavailable":                    {},
	"ScheduleFieldCount":                       {},
	"VolumeAutoGrow":                           {},
	"VolumeSnapshotError":                      {},
	string(apiv1.ConditionTopologyUnavailable): {},
	string(apiv1.ConditionReasonStorageClassForbidsExpansion): {},
	string(apiv1.ConditionReasonExpansionForbidden):           {},
	string(apiv1.ConditionReasonVolumeExpansionAccepted):      {},
	string(apiv1.ConditionReasonFileSystemResizePending):      {},
	string(apiv1.ConditionReasonVolumeResizeCompleted):        {},
	string(apiv1.ConditionReasonMaxSizeReached):               {},
	string(apiv1.ConditionReasonWALArchivingFailing):          {},
	string(apiv1.ConditionReasonVolumeAutoGrowResumed):        {},
}

// eventRecorder sends the diagnostics through a component of their own. The
// client limits the rate of the events per component and object: without it,
// the diagnostics would use up the budget of the other events.
type eventRecorder struct {
	record.EventRecorder
	diagnostics record.EventRecorder
}

// NewEventRecorder returns the event recorder of a controller
func NewEventRecorder(mgr ctrl.Manager, component string) record.EventRecorder {
	return &eventRecorder{
		EventRecorder: mgr.GetEventRecorderFor(component),                              //nolint:staticcheck
		diagnostics:   mgr.GetEventRecorderFor(component + diagnosticsComponentSuffix), //nolint:staticcheck
	}
}

func (r *eventRecorder) recorderFor(reason string) record.EventRecorder {
	if _, ok := diagnosticsReasons[reason]; ok {
		return r.diagnostics
	}
	return r.EventRecorder
}

// Event implements record.EventRecorder
func (r *eventRecorder) Event(object runtime.Object, eventType, reason, message string) {
	r.recorderFor(reason).Event(object, eventType, reason, message)
}

// Eventf implements record.EventRecorder
func (r *eventRecorder) Eventf(object runtime.Object, eventType, reason, messageFmt string, args ...interface{}) {
	r.recorderFor(reason).Eventf(object, eventType, reason, messageFmt, args...)
}

// AnnotatedEventf implements record.EventRecorder
func (r *eventRecorder) AnnotatedEventf(
	object runtime.Object,
	annotations map[string]string,
	eventType, reason, messageFmt string,
	args ...interface{},
) {
	r.recorderFor(reason).AnnotatedEventf(object, annotations, eventType, reason, messageFmt, args...)
}

// stateTracker remembers the last state seen of an object, for the events to
// be recorded when the state begins and not at each reconciliation
type stateTracker struct {
	states sync.Map
}

// begins tells whether the state differs from the previous one of the key.
// An empty state forgets the key.
func (t *stateTracker) begins(key, state string) bool {
	if state == "" {
		t.states.Delete(key)
		return false
	}
	previous, loaded := t.states.Swap(key, state)
	return !loaded || previous != state
}
