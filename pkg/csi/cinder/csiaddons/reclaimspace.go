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
	"os"
	"sync"

	"github.com/csi-addons/spec/lib/go/reclaimspace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

// errNotMountPoint is returned by fitrim when the path is not a mount root.
var errNotMountPoint = errors.New("not a mount point")

// MountChecker is the subset of k8s.io/mount-utils.Interface used to validate
// the trim target. Unlike mount.IMount.IsLikelyNotMountPointAttach it never
// creates the path.
type MountChecker interface {
	IsLikelyNotMountPoint(file string) (bool, error)
}

type reclaimSpaceNodeServer struct {
	reclaimspace.UnimplementedReclaimSpaceNodeServer
	mounter MountChecker
	trim    func(path string) (uint64, error)
	// inFlight holds the IDs of volumes being trimmed.
	inFlight sync.Map
}

func (s *reclaimSpaceNodeServer) NodeReclaimSpace(_ context.Context, req *reclaimspace.NodeReclaimSpaceRequest) (*reclaimspace.NodeReclaimSpaceResponse, error) {
	id := req.GetVolumeId()
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID missing in request")
	}
	if req.GetVolumeCapability().GetBlock() != nil {
		return nil, status.Error(codes.Unimplemented, "reclaim space is not supported for block volumes")
	}
	path := req.GetStagingTargetPath()
	if path == "" {
		path = req.GetVolumePath()
	}
	if path == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path and volume path missing in request")
	}

	// FITRIM outlives the caller's per-request timeout, so the caller's
	// retries would otherwise overlap on the backend.
	if _, busy := s.inFlight.LoadOrStore(id, struct{}{}); busy {
		return nil, status.Errorf(codes.Aborted, "reclaim space already in progress for volume %s", id)
	}
	defer s.inFlight.Delete(id)

	notMnt, err := s.mounter.IsLikelyNotMountPoint(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "path %s does not exist", path)
		}
		return nil, status.Errorf(codes.Internal, "failed to check mount point %s: %v", path, err)
	}
	// Trimming an unmounted path would discard free blocks of the node's own
	// filesystem instead of the volume's.
	if notMnt {
		return nil, status.Errorf(codes.FailedPrecondition, "path %s is not a mount point", path)
	}

	trimmed, err := s.trim(path)
	if errors.Is(err, errNotMountPoint) {
		return nil, status.Errorf(codes.FailedPrecondition, "path %s is not a mount point", path)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "FITRIM on %s failed: %v", path, err)
	}
	klog.Infof("NodeReclaimSpace: volume %s: %d bytes trimmed on %s", id, trimmed, path)
	return &reclaimspace.NodeReclaimSpaceResponse{}, nil
}

// reclaimSpaceControllerServer only exists so the controller service can
// advertise ReclaimSpace OFFLINE; Cinder has no controller-side reclaim.
type reclaimSpaceControllerServer struct {
	reclaimspace.UnimplementedReclaimSpaceControllerServer
}
