// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package northbound implements the northbound API of the topo discovery
package northbound

import (
	"context"
	api "github.com/onosproject/onos-api/go/onos/discovery"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-lib-go/pkg/northbound"
	"github.com/onosproject/topo-discovery/pkg/controller"
	"google.golang.org/grpc"
)

var log = logging.GetLogger("northbound")

// Service implements the topolody discovery NB gRPC
type Service struct {
	northbound.Service
	controller *controller.Controller
}

// NewService allocates a Service struct with the given parameters
func NewService(controller *controller.Controller) Service {
	return Service{
		controller: controller,
	}
}

// Register registers the server with grpc
func (s Service) Register(r *grpc.Server) {
	server := &Server{
		controller: s.controller,
	}
	api.RegisterDiscoveryServiceServer(r, server)
	log.Debug("Topology Discovery API services registered")
}

// Server implements the grpc topology discovery service
type Server struct {
	controller *controller.Controller
}

// AddPod adds a new POD entity with the requisite aspects
func (s Server) AddPod(ctx context.Context, request *api.AddPodRequest) (*api.AddPodResponse, error) {
	//TODO implement me
	panic("implement me")
}

// AddRack adds a new rack entity with the requisite aspects as part of a POD
func (s Server) AddRack(ctx context.Context, request *api.AddRackRequest) (*api.AddRackResponse, error) {
	//TODO implement me
	panic("implement me")
}

// AddSwitch adds a new switch entity with the requisite aspects into a rack
func (s Server) AddSwitch(ctx context.Context, request *api.AddSwitchRequest) (*api.AddSwitchResponse, error) {
	//TODO implement me
	panic("implement me")
}

// AddServerIPU adds a new server entity and an associated IPU entity, both with the requisite aspects into a rack
func (s Server) AddServerIPU(ctx context.Context, request *api.AddServerIPURequest) (*api.AddServerIPUResponse, error) {
	//TODO implement me
	panic("implement me")
}
