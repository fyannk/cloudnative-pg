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

// Package volumeusage reads the space and the inodes of the filesystems of
// the volumes of an instance
package volumeusage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"
)

// Paths are the mount points of the volumes of an instance
type Paths struct {
	PgData      string
	WalVolume   string
	Tablespaces string
}

// DefaultPaths are the mount points used by the instance Pods
var DefaultPaths = Paths{
	PgData:      specs.PgDataVolumePath,
	WalVolume:   specs.PgWalVolumePath,
	Tablespaces: specs.PgTablespaceVolumePath,
}

// Statfs reads the filesystem of a path, as syscall.Statfs does
type Statfs func(path string, stat *syscall.Statfs_t) error

// volume is a volume of the instance and its mount point
type volume struct {
	name string
	path string
}

// listVolumes returns the volumes mounted in the instance: PGDATA, the WAL
// volume when there is one, and one volume per tablespace
func listVolumes(paths Paths) ([]volume, error) {
	volumes := []volume{{name: postgres.PgDataVolumeName, path: paths.PgData}}
	var errs []error

	switch _, err := os.Stat(paths.WalVolume); {
	case err == nil:
		volumes = append(volumes, volume{name: postgres.WalVolumeName, path: paths.WalVolume})
	case !errors.Is(err, os.ErrNotExist):
		errs = append(errs, err)
	}

	entries, err := os.ReadDir(paths.Tablespaces)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		volumes = append(volumes, volume{
			name: postgres.TablespaceVolumePrefix + entry.Name(),
			path: filepath.Join(paths.Tablespaces, entry.Name()),
		})
	}

	return volumes, errors.Join(errs...)
}

// Get reads the filesystem of each volume of the instance. It only reads the
// mount points, so it works while PostgreSQL is down. A volume that cannot be
// read is left out, the others are returned, and the error tells which one.
func Get(paths Paths, statfs Statfs) ([]postgres.VolumeUsage, error) {
	volumes, err := listVolumes(paths)
	errs := []error{err}

	usages := make([]postgres.VolumeUsage, 0, len(volumes))
	for _, volume := range volumes {
		var stat syscall.Statfs_t
		if err := statfs(volume.path, &stat); err != nil {
			errs = append(errs, fmt.Errorf("volume %s: %w", volume.name, err))
			continue
		}

		blockSize := uint64(stat.Bsize) //nolint:gosec // the block size is positive
		usage := postgres.VolumeUsage{
			Volume:         volume.name,
			TotalBytes:     stat.Blocks * blockSize,
			AvailableBytes: stat.Bavail * blockSize,
		}
		if stat.Blocks > stat.Bfree {
			usage.UsedBytes = (stat.Blocks - stat.Bfree) * blockSize
		}
		// statfs sets the fields a filesystem does not support to 0
		if stat.Files > 0 {
			usage.Inodes = stat.Files
			usage.InodesFree = stat.Ffree
		}
		usages = append(usages, usage)
	}

	return usages, errors.Join(errs...)
}

// GetWALVolumeName returns the name of the volume holding the WAL files: the
// WAL volume when there is one, PGDATA otherwise
func GetWALVolumeName(paths Paths) string {
	if _, err := os.Stat(paths.WalVolume); err == nil {
		return postgres.WalVolumeName
	}
	return postgres.PgDataVolumeName
}

// ErrNotAnswering is returned when the filesystem of a volume does not
// answer, such as a network mount whose server is gone
var ErrNotAnswering = errors.New("the filesystem of a volume does not answer")

// notAnswering is set while a read of the filesystems that ran out of time
// is still blocked: no other read is started until it returns, so that the
// blocked reads do not pile up
var notAnswering atomic.Bool

// GetWithTimeout is Get, giving up after the timeout: reading a filesystem
// cannot be interrupted, and must not hold a scrape of the metrics forever
func GetWithTimeout(paths Paths, statfs Statfs, timeout time.Duration) ([]postgres.VolumeUsage, error) {
	if notAnswering.Load() {
		return nil, ErrNotAnswering
	}

	type result struct {
		usages []postgres.VolumeUsage
		err    error
	}
	done := make(chan result, 1)
	go func() {
		usages, err := Get(paths, statfs)
		notAnswering.Store(false)
		done <- result{usages: usages, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case read := <-done:
		return read.usages, read.err
	case <-timer.C:
		notAnswering.Store(true)
		return nil, fmt.Errorf("%w after %s", ErrNotAnswering, timeout)
	}
}
