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
	"math"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fitrimIoctl is FITRIM, _IOWR('X', 121, struct fstrim_range), which
// golang.org/x/sys/unix does not define. Same value on every Linux arch.
const fitrimIoctl = 0xC0185879

// fstrimRange mirrors struct fstrim_range from <linux/fs.h>.
type fstrimRange struct {
	start  uint64
	length uint64
	minLen uint64
}

// fitrim discards every unused block of the filesystem mounted at path and
// returns the byte count the kernel reports as trimmed.
func fitrim(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	rc, err := f.SyscallConn()
	if err != nil {
		return 0, err
	}
	var trimmed uint64
	var trimErr error
	if err := rc.Control(func(fd uintptr) { trimmed, trimErr = fitrimMountRoot(int(fd)) }); err != nil {
		return 0, err
	}
	return trimmed, trimErr
}

// fitrimMountRoot issues FITRIM on fd only if fd is the root of a mount. The
// open fd pins that mount, so the check cannot race with an unmount.
func fitrimMountRoot(fd int) (uint64, error) {
	var dir, parent unix.Stat_t
	if err := unix.Fstat(fd, &dir); err != nil {
		return 0, err
	}
	// ".." relative to fd is the parent of the directory actually opened,
	// not of a symlink the path may have gone through.
	if err := unix.Fstatat(fd, "..", &parent, 0); err != nil {
		return 0, err
	}
	if dir.Dev == parent.Dev {
		return 0, errNotMountPoint
	}

	r := fstrimRange{length: math.MaxUint64}
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), fitrimIoctl, uintptr(unsafe.Pointer(&r))); errno != 0 {
		return 0, errno
	}
	return r.length, nil
}
