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
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/csi-addons/spec/lib/go/reclaimspace"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

func TestNodeReclaimSpace(t *testing.T) {
	mounted := t.TempDir()
	unmounted := t.TempDir()
	broken := t.TempDir()
	missing := filepath.Join(t.TempDir(), "gone")
	blockCap := &csi.VolumeCapability{AccessType: &csi.VolumeCapability_Block{Block: &csi.VolumeCapability_BlockVolume{}}}
	mountCap := &csi.VolumeCapability{AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{}}}

	tests := []struct {
		name       string
		req        *reclaimspace.NodeReclaimSpaceRequest
		trimErr    error
		wantCode   codes.Code
		wantTrimOn string // "" = trim must not be called
	}{
		{"missing volume id", &reclaimspace.NodeReclaimSpaceRequest{StagingTargetPath: mounted}, nil, codes.InvalidArgument, ""},
		{"block volume", &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", StagingTargetPath: mounted, VolumeCapability: blockCap}, nil, codes.Unimplemented, ""},
		{"no paths", &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v"}, nil, codes.InvalidArgument, ""},
		{"staging path missing", &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", StagingTargetPath: missing}, nil, codes.NotFound, ""},
		// An existing but unmounted path would FITRIM the node's root filesystem.
		{"staging path not mounted", &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", StagingTargetPath: unmounted}, nil, codes.FailedPrecondition, ""},
		{"mount check error", &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", StagingTargetPath: broken}, nil, codes.Internal, ""},
		{"staging path mounted", &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", StagingTargetPath: mounted, VolumeCapability: mountCap}, nil, codes.OK, mounted},
		{"falls back to volume path", &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", VolumePath: mounted}, nil, codes.OK, mounted},
		{"trim fails", &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", StagingTargetPath: mounted}, errors.New("operation not supported"), codes.Internal, mounted},
		// fitrim's own check on the open directory, after the mount check.
		{"trim refuses non mount root", &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", StagingTargetPath: mounted}, errNotMountPoint, codes.FailedPrecondition, mounted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := mount.NewFakeMounter([]mount.MountPoint{{Path: mounted}})
			fake.MountCheckErrors = map[string]error{broken: errors.New("stat failed")}
			var trimmedOn string
			s := &reclaimSpaceNodeServer{
				mounter: fake,
				trim: func(path string) (uint64, error) {
					trimmedOn = path
					return 4096, tt.trimErr
				},
			}
			_, err := s.NodeReclaimSpace(context.Background(), tt.req)
			assert.Equal(t, tt.wantCode, status.Code(err), "error: %v", err)
			assert.Equal(t, tt.wantTrimOn, trimmedOn)
		})
	}
}

func TestNodeReclaimSpaceInFlight(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	entered := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	trims := map[string]int{}
	s := &reclaimSpaceNodeServer{
		mounter: mount.NewFakeMounter([]mount.MountPoint{{Path: first}, {Path: second}}),
		trim: func(path string) (uint64, error) {
			mu.Lock()
			trims[path]++
			n := trims[path]
			mu.Unlock()
			// Only the first trim of the first volume blocks, so a missing
			// guard fails the assertions below instead of hanging the test.
			if path == first && n == 1 {
				close(entered)
				<-release
			}
			return 4096, nil
		},
	}
	reclaim := func(id, path string) error {
		_, err := s.NodeReclaimSpace(context.Background(), &reclaimspace.NodeReclaimSpaceRequest{VolumeId: id, StagingTargetPath: path})
		return err
	}

	errA := make(chan error, 1)
	go func() { errA <- reclaim("vol-1", first) }()
	<-entered

	err := reclaim("vol-1", first)
	assert.Equal(t, codes.Aborted, status.Code(err), "error: %v", err)
	assert.NoError(t, reclaim("vol-2", second))

	close(release)
	assert.NoError(t, <-errA)
	// The guard is released once the first call returns.
	assert.NoError(t, reclaim("vol-1", first))

	assert.Equal(t, map[string]int{first: 2, second: 1}, trims)
}

func TestControllerReclaimSpaceUnimplemented(t *testing.T) {
	_, err := (&reclaimSpaceControllerServer{}).ControllerReclaimSpace(context.Background(), &reclaimspace.ControllerReclaimSpaceRequest{VolumeId: "v"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
