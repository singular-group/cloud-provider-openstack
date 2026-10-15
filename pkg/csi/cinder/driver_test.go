/*
Copyright 2017 The Kubernetes Authors.

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

package cinder

import (
	"context"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/csi-addons/spec/lib/go/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/cloud-provider-openstack/pkg/csi/cinder/openstack"
	"k8s.io/cloud-provider-openstack/pkg/util/metadata"
	"k8s.io/cloud-provider-openstack/pkg/util/mount"
)

var (
	vendorVersion = "2.0.0"
)

func NewFakeDriver() *Driver {

	driver := NewDriver(&DriverOpts{Endpoint: FakeEndpoint, ClusterID: FakeCluster})

	return driver
}

func TestValidateControllerServiceRequest(t *testing.T) {
	d := NewFakeDriver()

	// Valid requests which require no capabilities
	err := d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_UNKNOWN)
	assert.NoError(t, err)

	// Test controller service publish/unpublish is supported
	err = d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME)
	assert.NoError(t, err)

	// Test controller service create/delete is supported
	err = d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME)
	assert.NoError(t, err)

	// Test controller service list volumes is supported
	err = d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_LIST_VOLUMES)
	assert.NoError(t, err)

	// Test controller service create/delete snapshot is supported
	err = d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT)
	assert.NoError(t, err)

	// Test controller service list snapshot is supported
	err = d.ValidateControllerServiceRequest(csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS)
	assert.NoError(t, err)
}

// serveCSIAddons serves d's CSI-Addons server on sock and returns a client.
func serveCSIAddons(t *testing.T, d *Driver, sock string) *grpc.ClientConn {
	t.Helper()
	s, lis, err := d.newCSIAddonsServer()
	require.NoError(t, err)
	go func() { _ = s.Serve(lis) }()
	// Registered before the conn's cleanup: t.Cleanup is LIFO, so the client
	// conn closes first and GracefulStop does not wait on it.
	t.Cleanup(s.Stop)

	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return conn
}

func csiAddonsCapabilities(t *testing.T, conn *grpc.ClientConn) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := identity.NewIdentityClient(conn).GetCapabilities(ctx, &identity.GetCapabilitiesRequest{})
	require.NoError(t, err)
	var caps []string
	for _, c := range resp.GetCapabilities() {
		if svc := c.GetService(); svc != nil {
			caps = append(caps, "service:"+svc.GetType().String())
		}
		if rs := c.GetReclaimSpace(); rs != nil {
			caps = append(caps, "reclaimspace:"+rs.GetType().String())
		}
	}
	return caps
}

func TestNewCSIAddonsServerNodeMode(t *testing.T) {
	sock := csiAddonsSocket(t)
	d := NewDriver(&DriverOpts{Endpoint: FakeEndpoint, ClusterID: FakeCluster, CSIAddonsEndpoint: "unix://" + sock})
	d.SetupNodeService(new(mount.MountMock), new(metadata.MetadataMock), openstack.BlockStorageOpts{}, map[string]string{})
	conn := serveCSIAddons(t, d, sock)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := identity.NewIdentityClient(conn).GetIdentity(ctx, &identity.GetIdentityRequest{})
	require.NoError(t, err)
	assert.Equal(t, driverName, id.GetName())
	assert.Equal(t, d.fqVersion, id.GetVendorVersion())

	assert.Equal(t, []string{"service:NODE_SERVICE", "reclaimspace:ONLINE"}, csiAddonsCapabilities(t, conn))
}

func TestNewCSIAddonsServerControllerMode(t *testing.T) {
	sock := csiAddonsSocket(t)
	d := NewDriver(&DriverOpts{Endpoint: FakeEndpoint, ClusterID: FakeCluster, CSIAddonsEndpoint: "unix://" + sock})
	d.SetupControllerService(map[string]openstack.IOpenStack{"": new(openstack.OpenStackMock)})
	conn := serveCSIAddons(t, d, sock)

	assert.Equal(t, []string{"service:CONTROLLER_SERVICE", "reclaimspace:OFFLINE"}, csiAddonsCapabilities(t, conn))
}
