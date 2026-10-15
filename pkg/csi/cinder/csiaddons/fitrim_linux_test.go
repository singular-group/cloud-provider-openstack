/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package csiaddons

import (
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/unix"
)

// _IOWR('X', 121, struct fstrim_range) with the generic Linux ioctl encoding.
func TestFitrimIoctlValue(t *testing.T) {
	const iocWrite, iocRead = 1, 2
	size := uint64(unsafe.Sizeof(fstrimRange{}))
	want := uint64(iocRead|iocWrite)<<30 | size<<16 | uint64('X')<<8 | 121
	assert.Equal(t, uint64(24), size)
	assert.Equal(t, want, uint64(fitrimIoctl))
}

func TestFitrimMissingPath(t *testing.T) {
	_, err := fitrim(filepath.Join(t.TempDir(), "gone"))
	assert.Error(t, err)
}

// A plain directory shares its parent's device. Getting the sentinel rather
// than the kernel's EPERM/ENOTTY/EOPNOTSUPP shows the ioctl was never issued.
func TestFitrimNotMountRoot(t *testing.T) {
	_, err := fitrim(t.TempDir())
	assert.ErrorIs(t, err, errNotMountPoint)
}

// /proc is a mount root, so the ioctl is issued; procfs rejects it with
// ENOTTY without touching any block device.
func TestFitrimMountRoot(t *testing.T) {
	_, err := fitrim("/proc")
	assert.ErrorIs(t, err, unix.ENOTTY)
}
