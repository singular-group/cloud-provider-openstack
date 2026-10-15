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
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/csi-addons/spec/lib/go/identity"
	"github.com/csi-addons/spec/lib/go/reclaimspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

func startServer(t *testing.T, opts Options) *grpc.ClientConn {
	t.Helper()
	// Not t.TempDir(): it embeds the test name and can exceed the unix
	// socket length limit.
	dir, err := os.MkdirTemp("", "csia")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "csi-addons.sock")
	lis, err := net.Listen("unix", sock)
	require.NoError(t, err)
	s := NewServer(opts)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestServerNodeMode(t *testing.T) {
	conn := startServer(t, Options{DriverName: "cinder.csi.openstack.org", Version: "2.0.0@test", Mounter: mount.NewFakeMounter(nil)})
	ctx := context.Background()

	caps, err := identity.NewIdentityClient(conn).GetCapabilities(ctx, &identity.GetCapabilitiesRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"service:NODE_SERVICE", "reclaimspace:ONLINE"}, capStrings(caps.GetCapabilities()))

	// Registered: reaches the handler, which rejects the unmounted path.
	_, err = reclaimspace.NewReclaimSpaceNodeClient(conn).NodeReclaimSpace(ctx, &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", StagingTargetPath: t.TempDir()})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))

	_, err = reclaimspace.NewReclaimSpaceControllerClient(conn).ControllerReclaimSpace(ctx, &reclaimspace.ControllerReclaimSpaceRequest{VolumeId: "v"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestServerControllerMode(t *testing.T) {
	conn := startServer(t, Options{DriverName: "cinder.csi.openstack.org", Version: "2.0.0@test", Controller: true})
	ctx := context.Background()

	caps, err := identity.NewIdentityClient(conn).GetCapabilities(ctx, &identity.GetCapabilitiesRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"service:CONTROLLER_SERVICE", "reclaimspace:OFFLINE"}, capStrings(caps.GetCapabilities()))

	_, err = reclaimspace.NewReclaimSpaceControllerClient(conn).ControllerReclaimSpace(ctx, &reclaimspace.ControllerReclaimSpaceRequest{VolumeId: "v"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	// Node service not registered in controller mode.
	_, err = reclaimspace.NewReclaimSpaceNodeClient(conn).NodeReclaimSpace(ctx, &reclaimspace.NodeReclaimSpaceRequest{VolumeId: "v", StagingTargetPath: "/"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
