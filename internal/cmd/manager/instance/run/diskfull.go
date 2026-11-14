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

package run

import (
	"context"
	"os"
	"slices"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	corev1 "k8s.io/api/core/v1"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres/volumeusage"
	pg "github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
)

// terminationLogPath is the termination log of the PostgreSQL container: the
// Pods of the instances keep the default terminationMessagePath
var terminationLogPath = corev1.TerminationMessagePathDefault

// volumesReadTimeout bounds the reading of the filesystems when the instance
// manager exits
const volumesReadTimeout = 5 * time.Second

// getVolumesWithoutSpace returns the volumes of the instance nothing can be
// written to anymore, see volumeusage.HasNoSpaceLeft. A volume that cannot be
// read is left out
func getVolumesWithoutSpace(ctx context.Context, paths volumeusage.Paths, statfs volumeusage.Statfs) []string {
	usages, err := volumeusage.GetWithTimeout(paths, statfs, volumesReadTimeout)
	if err != nil {
		log.FromContext(ctx).Info("Cannot read every volume of the instance while exiting", "error", err.Error())
	}

	var volumes []string
	for _, usage := range usages {
		if volumeusage.HasNoSpaceLeft(usage) {
			volumes = append(volumes, usage.Volume)
		}
	}
	return volumes
}

// writeDiskFullTerminationMessage writes the termination message naming the
// full volumes, which the kubelet reports in the status of the container for
// the operator to grow them. It writes nothing when no volume is full
func writeDiskFullTerminationMessage(ctx context.Context, volumes ...string) {
	slices.Sort(volumes)
	volumes = slices.Compact(volumes)
	if len(volumes) == 0 {
		return
	}

	message := pg.NewDiskFullTerminationMessage(volumes...).String()
	if err := os.WriteFile(terminationLogPath, []byte(message), 0o600); err != nil {
		log.FromContext(ctx).Error(err, "while writing the termination message", "message", message)
	}
}
