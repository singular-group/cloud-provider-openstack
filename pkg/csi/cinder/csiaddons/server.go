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
	"net"

	"github.com/csi-addons/spec/lib/go/identity"
	"github.com/csi-addons/spec/lib/go/reclaimspace"
	"google.golang.org/grpc"
)

// Options selects which CSI-Addons services the server exposes.
type Options struct {
	DriverName string
	Version    string
	// Controller advertises CONTROLLER_SERVICE and ReclaimSpace OFFLINE.
	Controller bool
	// Mounter, when set, enables the node ReclaimSpace service (ONLINE).
	Mounter MountChecker
}

// Server serves the CSI-Addons gRPC services.
type Server struct {
	grpc *grpc.Server
}

// NewServer registers Identity plus the ReclaimSpace services opts enables.
func NewServer(opts Options, grpcOpts ...grpc.ServerOption) *Server {
	s := grpc.NewServer(grpcOpts...)
	identity.RegisterIdentityServer(s, &identityServer{
		name:       opts.DriverName,
		version:    opts.Version,
		controller: opts.Controller,
		node:       opts.Mounter != nil,
	})
	if opts.Controller {
		reclaimspace.RegisterReclaimSpaceControllerServer(s, &reclaimSpaceControllerServer{})
	}
	if opts.Mounter != nil {
		reclaimspace.RegisterReclaimSpaceNodeServer(s, &reclaimSpaceNodeServer{mounter: opts.Mounter, trim: fitrim})
	}
	return &Server{grpc: s}
}

// Serve blocks until Stop is called or lis fails.
func (s *Server) Serve(lis net.Listener) error {
	return s.grpc.Serve(lis)
}

// Stop stops the server gracefully.
func (s *Server) Stop() {
	s.grpc.GracefulStop()
}
