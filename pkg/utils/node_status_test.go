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
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NodeStatusReader", func() {
	const prefix = "bridge.example.com/"
	reader := NodeStatusReader{KeyPrefix: prefix, LabelPrefix: "node.example.com/", EvictionMaxAge: 10 * time.Minute}
	drainTaints := []string{"node.kubernetes.io/unschedulable", "karpenter.sh/disrupted"}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	pod := func(labels, annotations map[string]string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pg-1", Labels: labels, Annotations: annotations}}
	}

	DescribeTable("reads the cordon state from the unschedulable key",
		func(labels, annotations map[string]string, expected bool) {
			Expect(reader.IsUnschedulable(pod(labels, annotations))).To(Equal(expected))
		},
		Entry("no key", nil, nil, false),
		Entry("label true", map[string]string{prefix + "unschedulable": "true"}, nil, true),
		Entry("annotation true", nil, map[string]string{prefix + "unschedulable": "true"}, true),
		Entry("label with another value", map[string]string{prefix + "unschedulable": "false"}, nil, false),
		Entry("another prefix", map[string]string{"other.example.com/unschedulable": "true"}, nil, false),
	)

	DescribeTable("decodes the taints annotation",
		func(annotations map[string]string, expected []corev1.Taint, fails bool) {
			taints, err := reader.Taints(pod(nil, annotations))
			if fails {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).ToNot(HaveOccurred())
			Expect(taints).To(HaveLen(len(expected)))
			for i := range expected {
				Expect(taints[i].Key).To(Equal(expected[i].Key))
				Expect(taints[i].Value).To(Equal(expected[i].Value))
				Expect(taints[i].Effect).To(Equal(expected[i].Effect))
				if expected[i].TimeAdded == nil {
					Expect(taints[i].TimeAdded).To(BeNil())
				} else {
					Expect(taints[i].TimeAdded.Time).To(BeTemporally("==", expected[i].TimeAdded.Time))
				}
			}
		},
		Entry("no annotation", nil, nil, false),
		Entry("one taint",
			map[string]string{prefix + "taints": `[{"key":"karpenter.sh/disrupted","effect":"NoSchedule"}]`},
			[]corev1.Taint{{Key: "karpenter.sh/disrupted", Effect: corev1.TaintEffectNoSchedule}}, false),
		Entry("two taints with values and times",
			map[string]string{prefix + "taints": `[{"key":"a","value":"1","effect":"NoExecute",` +
				`"timeAdded":"2026-10-20T11:00:00Z"},{"key":"b","effect":"PreferNoSchedule"}]`},
			[]corev1.Taint{
				{
					Key: "a", Value: "1", Effect: corev1.TaintEffectNoExecute,
					TimeAdded: &metav1.Time{Time: time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC)},
				},
				{Key: "b", Effect: corev1.TaintEffectPreferNoSchedule},
			}, false),
		Entry("empty array", map[string]string{prefix + "taints": `[]`}, nil, false),
		Entry("not JSON", map[string]string{prefix + "taints": `karpenter.sh/disrupted`}, nil, true),
		Entry("not an array", map[string]string{prefix + "taints": `{"key":"a"}`}, nil, true),
	)

	DescribeTable("applies the drain predicate of the Node API to the echoed state",
		func(labels, annotations map[string]string, expected bool) {
			Expect(reader.IsOnDrainingNode(pod(labels, annotations), drainTaints)).To(Equal(expected))
		},
		Entry("nothing echoed", nil, nil, false),
		Entry("cordoned", map[string]string{prefix + "unschedulable": "true"}, nil, true),
		Entry("a drain taint",
			nil, map[string]string{prefix + "taints": `[{"key":"karpenter.sh/disrupted","effect":"NoSchedule"}]`}, true),
		Entry("a drain taint among others",
			nil, map[string]string{prefix + "taints": `[{"key":"other","effect":"NoSchedule"},` +
				`{"key":"node.kubernetes.io/unschedulable","effect":"NoSchedule"}]`}, true),
		Entry("a taint that is not a drain taint",
			nil, map[string]string{prefix + "taints": `[{"key":"other","effect":"NoSchedule"}]`}, false),
		Entry("taints that cannot be decoded count for nothing",
			nil, map[string]string{prefix + "taints": `nope`}, false),
		Entry("cordoned with undecodable taints",
			map[string]string{prefix + "unschedulable": "true"}, map[string]string{prefix + "taints": `nope`}, true),
	)

	DescribeTable("reads the time of the eviction request",
		func(annotations map[string]string, expected time.Time, ok bool) {
			requestedAt, found := reader.EvictionRequestedAt(pod(nil, annotations))
			Expect(found).To(Equal(ok))
			if ok {
				Expect(requestedAt).To(BeTemporally("==", expected))
			}
		},
		Entry("no annotation", nil, time.Time{}, false),
		Entry("RFC 3339 in UTC", map[string]string{prefix + "eviction-requested": "2026-10-20T11:59:00Z"},
			time.Date(2026, 10, 20, 11, 59, 0, 0, time.UTC), true),
		Entry("RFC 3339 with an offset", map[string]string{prefix + "eviction-requested": "2026-10-20T13:59:00+02:00"},
			time.Date(2026, 10, 20, 11, 59, 0, 0, time.UTC), true),
		Entry("not a time", map[string]string{prefix + "eviction-requested": "yesterday"}, time.Time{}, false),
	)

	DescribeTable("acts on an eviction request within its maximum age",
		func(reader NodeStatusReader, value string, expected bool) {
			annotations := map[string]string{}
			if value != "" {
				annotations[prefix+"eviction-requested"] = value
			}
			Expect(reader.IsEvictionRequested(pod(nil, annotations), now)).To(Equal(expected))
		},
		Entry("no request", reader, "", false),
		Entry("one minute old", reader, "2026-10-20T11:59:00Z", true),
		Entry("exactly the maximum age", reader, "2026-10-20T11:50:00Z", true),
		Entry("one second too old", reader, "2026-10-20T11:49:59Z", false),
		Entry("in the future, from a clock ahead of ours", reader, "2026-10-20T12:05:00Z", true),
		Entry("no maximum age", NodeStatusReader{KeyPrefix: prefix}, "2020-01-01T00:00:00Z", true),
		Entry("not a time", reader, "yesterday", false),
	)

	DescribeTable("resolves the copied node labels",
		func(labels map[string]string, keys []string, expected map[string]string, ok bool) {
			resolved, found := reader.NodeLabels(pod(labels, nil), keys)
			Expect(found).To(Equal(ok))
			Expect(resolved).To(Equal(expected))
		},
		Entry("no key asked", map[string]string{}, nil, map[string]string{}, true),
		Entry("one key copied", map[string]string{"node.example.com/zone": "az1"}, []string{"zone"},
			map[string]string{"zone": "az1"}, true),
		Entry("two keys copied, one empty",
			map[string]string{"node.example.com/zone": "az1", "node.example.com/rack": ""}, []string{"zone", "rack"},
			map[string]string{"zone": "az1", "rack": ""}, true),
		Entry("one key missing", map[string]string{"node.example.com/zone": "az1"}, []string{"zone", "rack"}, nil, false),
		Entry("the key without the prefix does not count", map[string]string{"zone": "az1"}, []string{"zone"}, nil, false),
	)

	It("reads the labels under the node keys themselves without a label prefix", func() {
		resolved, ok := NodeStatusReader{}.NodeLabels(pod(map[string]string{"topology.kubernetes.io/zone": "az1"}, nil),
			[]string{"topology.kubernetes.io/zone"})
		Expect(ok).To(BeTrue())
		Expect(resolved).To(HaveKeyWithValue("topology.kubernetes.io/zone", "az1"))
	})
})
