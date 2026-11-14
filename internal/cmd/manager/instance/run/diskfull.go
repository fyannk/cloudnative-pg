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

	"github.com/cloudnative-pg/machinery/pkg/log"
	corev1 "k8s.io/api/core/v1"

	pg "github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
)

// terminationLogPath is the termination log of the PostgreSQL container: the
// Pods of the instances keep the default terminationMessagePath
var terminationLogPath = corev1.TerminationMessagePathDefault

// writeDiskFullTerminationMessage writes the termination message naming the
// volume holding the WAL files, full, which the kubelet reports in the status
// of the container for the operator to grow it
func writeDiskFullTerminationMessage(ctx context.Context, volume string) {
	message := pg.NewDiskFullTerminationMessage(volume).String()
	if err := os.WriteFile(terminationLogPath, []byte(message), 0o600); err != nil {
		log.FromContext(ctx).Error(err, "while writing the termination message", "message", message)
	}
}
