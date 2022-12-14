// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package northbound implements the northbound API of the topo discovery
package northbound

import (
	"context"
	api "github.com/onosproject/onos-api/go/onos/discovery"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-lib-go/pkg/northbound"
	"github.com/onosproject/topo-discovery/pkg/controller"
	"google.golang.org/grpc"
)

var log = logging.GetLogger("northbound")

// Service implements the topology discovery NB gRPC
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
func (s *Server) AddPod(ctx context.Context, request *api.AddPodRequest) (*api.AddPodResponse, error) {
	log.Infof("Adding new pod %s", request.ID)
	if err := s.controller.AddPod(ctx, request); err != nil {
		log.Warnf("Failed adding new pod %s: %v", request.ID, err)
		return nil, errors.Status(err).Err()
	}
	return &api.AddPodResponse{}, nil
}

// AddRack adds a new rack entity with the requisite aspects as part of a POD
func (s *Server) AddRack(ctx context.Context, request *api.AddRackRequest) (*api.AddRackResponse, error) {
	log.Infof("Adding new rack %s", request.ID)
	if err := s.controller.AddRack(ctx, request); err != nil {
		log.Warnf("Failed adding new rack %s: %v", request.ID, err)
		return nil, errors.Status(err).Err()
	}
	return &api.AddRackResponse{}, nil
}

// AddSwitch adds a new switch entity with the requisite aspects into a rack
func (s *Server) AddSwitch(ctx context.Context, request *api.AddSwitchRequest) (*api.AddSwitchResponse, error) {
	log.Infof("Adding new switch %s", request.ID)
	if err := s.controller.AddSwitch(ctx, request); err != nil {
		log.Warnf("Failed adding new switch %s: %v", request.ID, err)
		return nil, errors.Status(err).Err()
	}
	return &api.AddSwitchResponse{}, nil
}

// AddServerIPU adds a new server entity and an associated IPU entity, both with the requisite aspects into a rack
func (s *Server) AddServerIPU(ctx context.Context, request *api.AddServerIPURequest) (*api.AddServerIPUResponse, error) {
	log.Infof("Adding new server IPU %s", request.ID)
	if err := s.controller.AddServerIPU(ctx, request); err != nil {
		log.Warnf("Failed adding new server IPU %s: %v", request.ID, err)
		return nil, errors.Status(err).Err()
	}
	return &api.AddServerIPUResponse{}, nil
}
