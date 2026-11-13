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
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	corev1 "k8s.io/api/core/v1"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres/volumeusage"
	pg "github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
)

// terminationLogPath is the termination log of the PostgreSQL container: the
// Pods of the instances keep the default terminationMessagePath
var terminationLogPath = corev1.TerminationMessagePathDefault

// errDiskFull is returned when PostgreSQL stopped because a volume is full
type errDiskFull struct {
	volumes []string
	err     error
}

func (e *errDiskFull) Error() string {
	return fmt.Sprintf("no free disk space on the volume(s) %s: %s", strings.Join(e.volumes, ", "), e.err)
}

func (e *errDiskFull) Unwrap() error {
	return e.err
}

// getFullVolumes returns the full volumes of an instance whose PostgreSQL
// stopped, when it logged a disk full error, see logpipe.IsDiskFullLogged.
// It returns nil when PostgreSQL logged no such error, or when no volume is
// full: the error was about another filesystem, or the space was freed since.
func getFullVolumes(
	ctx context.Context,
	diskFullLogged bool,
	paths volumeusage.Paths,
	statfs volumeusage.Statfs,
) []string {
	if !diskFullLogged {
		return nil
	}

	contextLogger := log.FromContext(ctx)
	usages, err := volumeusage.Get(paths, statfs)
	if err != nil {
		contextLogger.Error(err, "while reading the usage of the volumes, after a disk full error of PostgreSQL")
	}
	volumes := volumeusage.GetFullVolumes(usages)
	if len(volumes) == 0 {
		contextLogger.Info("PostgreSQL logged a disk full error, but no volume of the instance is full",
			"usages", usages)
	}
	return volumes
}

// getMissingDiskSpaceVolumes returns the full volumes the instance manager
// exits for, and whether it exits for a lack of disk space
func getMissingDiskSpaceVolumes(err error) ([]string, bool) {
	var diskFull *errDiskFull
	switch {
	case errors.As(err, &diskFull):
		return diskFull.volumes, true
	case errors.Is(err, postgres.ErrNoFreeWALSpace):
		return []string{volumeusage.GetWALVolumeName(volumeusage.DefaultPaths)}, true
	default:
		return nil, false
	}
}

// writeDiskFullTerminationMessage writes the termination message naming the
// full volumes, which the kubelet reports in the status of the container for
// the operator to grow them
func writeDiskFullTerminationMessage(ctx context.Context, volumes []string) {
	message := pg.NewDiskFullTerminationMessage(volumes...).String()
	if err := os.WriteFile(terminationLogPath, []byte(message), 0o600); err != nil {
		log.FromContext(ctx).Error(err, "while writing the termination message", "message", message)
	}
}
