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

package v1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The label keys of the topology feed both the TopologyUnavailable condition
// and the webhook warning: the failure domain keys of the synchronous
// replicas, when set, replace the node labels of syncReplicaElectionConstraint
var _ = DescribeTable("The label keys of the topology of the instances",
	func(election []string, synchronous *SynchronousReplicaConfiguration, pods, nodes []string) {
		cluster := &Cluster{Spec: ClusterSpec{PostgresConfiguration: PostgresConfiguration{
			SyncReplicaElectionConstraint: SyncReplicaElectionConstraints{NodeLabelsAntiAffinity: election},
			Synchronous:                   synchronous,
		}}}
		podKeys, nodeKeys := cluster.GetTopologyLabelKeys()
		Expect(podKeys).To(Equal(pods))
		Expect(nodeKeys).To(Equal(nodes))
	},
	Entry("none", nil, nil, nil, nil),
	Entry("the election constraint", []string{"kubernetes.io/hostname"}, nil,
		nil, []string{"kubernetes.io/hostname"}),
	Entry("a synchronous block without failure domain keys keeps the election constraint",
		[]string{"topology.kubernetes.io/zone"}, &SynchronousReplicaConfiguration{Number: 1},
		nil, []string{"topology.kubernetes.io/zone"}),
	Entry("node failure domain keys replace the election constraint",
		[]string{"kubernetes.io/hostname"}, &SynchronousReplicaConfiguration{NodeFailureDomainKeys: []string{"rack"}},
		nil, []string{"rack"}),
	Entry("pod failure domain keys alone replace it too, with no node label",
		[]string{"kubernetes.io/hostname"}, &SynchronousReplicaConfiguration{PodFailureDomainKeys: []string{"zone"}},
		[]string{"zone"}, nil),
	Entry("both kinds of failure domain keys",
		nil, &SynchronousReplicaConfiguration{
			PodFailureDomainKeys: []string{"zone"}, NodeFailureDomainKeys: []string{"rack"},
		},
		[]string{"zone"}, []string{"rack"}),
)
