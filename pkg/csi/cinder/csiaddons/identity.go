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

// Package csiaddons serves the CSI-Addons services (github.com/csi-addons/spec)
// for the Cinder CSI driver.
package csiaddons

import (
	"context"

	"github.com/csi-addons/spec/lib/go/identity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type identityServer struct {
	identity.UnimplementedIdentityServer
	name       string
	version    string
	controller bool
	node       bool
}

func (s *identityServer) GetIdentity(_ context.Context, _ *identity.GetIdentityRequest) (*identity.GetIdentityResponse, error) {
	if s.name == "" || s.version == "" {
		return nil, status.Error(codes.Unavailable, "driver name or version not configured")
	}
	return &identity.GetIdentityResponse{Name: s.name, VendorVersion: s.version}, nil
}

// GetCapabilities advertises ReclaimSpace OFFLINE for the controller service:
// the CSI-Addons controller requires a leader advertising it before it runs
// the node-side ONLINE operation.
func (s *identityServer) GetCapabilities(_ context.Context, _ *identity.GetCapabilitiesRequest) (*identity.GetCapabilitiesResponse, error) {
	var caps []*identity.Capability
	if s.controller {
		caps = append(caps,
			serviceCap(identity.Capability_Service_CONTROLLER_SERVICE),
			reclaimSpaceCap(identity.Capability_ReclaimSpace_OFFLINE))
	}
	if s.node {
		caps = append(caps,
			serviceCap(identity.Capability_Service_NODE_SERVICE),
			reclaimSpaceCap(identity.Capability_ReclaimSpace_ONLINE))
	}
	return &identity.GetCapabilitiesResponse{Capabilities: caps}, nil
}

func (s *identityServer) Probe(_ context.Context, _ *identity.ProbeRequest) (*identity.ProbeResponse, error) {
	return &identity.ProbeResponse{Ready: wrapperspb.Bool(true)}, nil
}

func serviceCap(t identity.Capability_Service_Type) *identity.Capability {
	return &identity.Capability{Type: &identity.Capability_Service_{
		Service: &identity.Capability_Service{Type: t},
	}}
}

func reclaimSpaceCap(t identity.Capability_ReclaimSpace_Type) *identity.Capability {
	return &identity.Capability{Type: &identity.Capability_ReclaimSpace_{
		ReclaimSpace: &identity.Capability_ReclaimSpace{Type: t},
	}}
}
