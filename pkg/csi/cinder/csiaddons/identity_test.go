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
	"testing"

	"github.com/csi-addons/spec/lib/go/identity"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func capStrings(caps []*identity.Capability) []string {
	var out []string
	for _, c := range caps {
		switch t := c.GetType().(type) {
		case *identity.Capability_Service_:
			out = append(out, "service:"+t.Service.GetType().String())
		case *identity.Capability_ReclaimSpace_:
			out = append(out, "reclaimspace:"+t.ReclaimSpace.GetType().String())
		}
	}
	return out
}

func TestGetCapabilities(t *testing.T) {
	tests := []struct {
		name             string
		controller, node bool
		want             []string
	}{
		{"controller only", true, false, []string{"service:CONTROLLER_SERVICE", "reclaimspace:OFFLINE"}},
		{"node only", false, true, []string{"service:NODE_SERVICE", "reclaimspace:ONLINE"}},
		{"both", true, true, []string{"service:CONTROLLER_SERVICE", "reclaimspace:OFFLINE", "service:NODE_SERVICE", "reclaimspace:ONLINE"}},
		{"neither", false, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &identityServer{name: "cinder.csi.openstack.org", version: "2.0.0@test", controller: tt.controller, node: tt.node}
			resp, err := s.GetCapabilities(context.Background(), &identity.GetCapabilitiesRequest{})
			assert.NoError(t, err)
			assert.Equal(t, tt.want, capStrings(resp.GetCapabilities()))
		})
	}
}

func TestGetIdentity(t *testing.T) {
	s := &identityServer{name: "cinder.csi.openstack.org", version: "2.0.0@test"}
	resp, err := s.GetIdentity(context.Background(), &identity.GetIdentityRequest{})
	assert.NoError(t, err)
	assert.Equal(t, "cinder.csi.openstack.org", resp.GetName())
	assert.Equal(t, "2.0.0@test", resp.GetVendorVersion())

	_, err = (&identityServer{}).GetIdentity(context.Background(), &identity.GetIdentityRequest{})
	assert.Equal(t, codes.Unavailable, status.Code(err))
}

func TestProbe(t *testing.T) {
	resp, err := (&identityServer{}).Probe(context.Background(), &identity.ProbeRequest{})
	assert.NoError(t, err)
	assert.True(t, resp.GetReady().GetValue())
}
