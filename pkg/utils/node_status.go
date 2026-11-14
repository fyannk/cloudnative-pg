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

package utils

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// Keys a node bridge echoes onto the instance Pods, relative to the prefix
// configured by NODE_STATUS_POD_KEY_PREFIX
const (
	// NodeStatusUnschedulableKey is the label or annotation set to "true"
	// while the Node of the Pod is cordoned (spec.unschedulable)
	NodeStatusUnschedulableKey = "unschedulable"

	// NodeStatusTaintsKey is the annotation holding, as a JSON array of
	// taints, the taints of the Node of the Pod that the bridge copies
	NodeStatusTaintsKey = "taints"

	// NodeStatusEvictionRequestedKey is the annotation holding the time, in
	// RFC 3339, of the first eviction request received for the Pod
	NodeStatusEvictionRequestedKey = "eviction-requested"
)

// NodeStatusReader reads, on an instance Pod, the node state a node bridge
// echoed onto it: the cordon state and the taints of its Node, the labels of
// its Node, and the eviction requests received for the Pod
type NodeStatusReader struct {
	// KeyPrefix is the prefix of the unschedulable, taints and
	// eviction-requested keys
	KeyPrefix string

	// LabelPrefix is the prefix under which the node labels are copied onto
	// the Pod: the node label <key> is read from the Pod label <prefix><key>
	LabelPrefix string

	// EvictionMaxAge is the age after which an eviction request is ignored;
	// zero means no limit
	EvictionMaxAge time.Duration
}

// IsUnschedulable reports whether the Node of the Pod is cordoned, from the
// unschedulable key, written as a label or as an annotation
func (r NodeStatusReader) IsUnschedulable(pod *corev1.Pod) bool {
	key := r.KeyPrefix + NodeStatusUnschedulableKey
	return pod.Labels[key] == "true" || pod.Annotations[key] == "true"
}

// Taints returns the taints of the Node of the Pod that the bridge copied;
// nil without the annotation, an error when it does not hold a JSON array
// of taints
func (r NodeStatusReader) Taints(pod *corev1.Pod) ([]corev1.Taint, error) {
	value, ok := pod.Annotations[r.KeyPrefix+NodeStatusTaintsKey]
	if !ok {
		return nil, nil
	}

	var taints []corev1.Taint
	if err := json.Unmarshal([]byte(value), &taints); err != nil {
		return nil, fmt.Errorf("while decoding the taints echoed onto the pod %q: %w", pod.Name, err)
	}
	return taints, nil
}

// IsOnDrainingNode reports whether the Node of the Pod is cordoned or carries
// a taint whose key is in drainTaints, with the same rule as the one applied
// to a Node read from the API. Taints that cannot be decoded count for nothing
func (r NodeStatusReader) IsOnDrainingNode(pod *corev1.Pod, drainTaints []string) bool {
	if r.IsUnschedulable(pod) {
		return true
	}

	taints, _ := r.Taints(pod)
	return slices.ContainsFunc(taints, func(taint corev1.Taint) bool {
		return slices.Contains(drainTaints, taint.Key)
	})
}

// EvictionRequestedAt returns the time of the first eviction request received
// for the Pod; false without the annotation or when it is not an RFC 3339 time
func (r NodeStatusReader) EvictionRequestedAt(pod *corev1.Pod) (time.Time, bool) {
	value, ok := pod.Annotations[r.KeyPrefix+NodeStatusEvictionRequestedKey]
	if !ok {
		return time.Time{}, false
	}

	requestedAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return requestedAt, true
}

// IsEvictionRequested reports whether an eviction request was received for
// the Pod and is not older than EvictionMaxAge as of now
func (r NodeStatusReader) IsEvictionRequested(pod *corev1.Pod, now time.Time) bool {
	requestedAt, ok := r.EvictionRequestedAt(pod)
	if !ok {
		return false
	}

	return r.EvictionMaxAge == 0 || now.Sub(requestedAt) <= r.EvictionMaxAge
}

// NodeLabels returns the values of the given node labels copied onto the
// Pod. The second value is false when one of them is missing: either the
// Node lacks it or the bridge has not copied it yet, and the two cannot be
// told apart
func (r NodeStatusReader) NodeLabels(pod *corev1.Pod, keys []string) (map[string]string, bool) {
	labels := make(map[string]string, len(keys))
	for _, key := range keys {
		value, ok := pod.Labels[r.LabelPrefix+key]
		if !ok {
			return nil, false
		}
		labels[key] = value
	}
	return labels, true
}
