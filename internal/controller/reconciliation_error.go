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
	"strings"

	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

const (
	// eventReasonQuotaExceeded is the reason of the Warning recorded when the
	// API server rejected a request because of a ResourceQuota
	eventReasonQuotaExceeded = "QuotaExceeded"

	// eventReasonAdmissionDenied is the reason of the Warning recorded when an
	// admission webhook or Pod Security denied a request
	eventReasonAdmissionDenied = "AdmissionDenied"

	// eventReasonReconciliationFailed is the reason of the Warning recorded
	// for any other error ending a reconciliation
	eventReasonReconciliationFailed = "ReconciliationFailed"

	// eventMessageMaxLength is the maximum length of an Event message
	// accepted by the API server
	eventMessageMaxLength = 1024
)

// recordReconciliationError records a Warning Event on the reconciled object
// when an error ends the reconciliation, so that its owner sees the cause
// without access to the operator logs. Errors that the operator causes
// itself and retries at once (optimistic lock conflicts, loop control,
// cancelled contexts) and errors already reported on the object by the
// admission guard (terminal errors) are not recorded.
func recordReconciliationError(recorder record.EventRecorder, object runtime.Object, err error) {
	reason := reconciliationErrorReason(err)
	if reason == "" {
		return
	}

	message := err.Error()
	if len(message) > eventMessageMaxLength {
		message = message[:eventMessageMaxLength-3] + "..."
	}
	recorder.Event(object, "Warning", reason, message)
}

// reconciliationErrorReason classifies an error ending a reconciliation into
// an Event reason, or returns an empty string when it must not be reported.
// The API server sets no structured reason on a ResourceQuota rejection: it
// is a Forbidden error whose message starts with "exceeded quota" (the limit
// is reached) or "failed quota" (the request has no limit for a resource the
// quota constrains).
func reconciliationErrorReason(err error) string {
	if err == nil ||
		errors.Is(err, utils.ErrNextLoop) ||
		errors.Is(err, utils.ErrTerminateLoop) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		apierrs.IsConflict(err) {
		return ""
	}

	if errors.Is(err, reconcile.TerminalError(nil)) {
		return ""
	}

	if apierrs.IsForbidden(err) {
		message := err.Error()
		switch {
		case strings.Contains(message, "exceeded quota"), strings.Contains(message, "failed quota"):
			return eventReasonQuotaExceeded
		case strings.Contains(message, "denied the request"), strings.Contains(message, "violates PodSecurity"):
			return eventReasonAdmissionDenied
		}
	}

	return eventReasonReconciliationFailed
}
