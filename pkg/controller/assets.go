// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package controller implements the topology discovery controller
package controller

import (
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	api "github.com/onosproject/onos-api/go/onos/discovery"
	"github.com/onosproject/onos-api/go/onos/provisioner"
	topo "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"strconv"
	"strings"
)

const (
	controllerNotReady = "Controller not ready yet"
)

// AddPod adds a new POD entity with the requisite aspects
func (c *Controller) AddPod(ctx context.Context, req *api.AddPodRequest) error {
	if c.getState() != Monitoring {
		return errors.NewUnavailable(controllerNotReady)
	}
	return c.createEntity(ctx, req.ID, topo.PodKind, nil, map[string]string{topo.PodKind: req.ID})
}

// AddRack adds a new rack entity with the requisite aspects as part of a POD
func (c *Controller) AddRack(ctx context.Context, req *api.AddRackRequest) error {
	if c.getState() != Monitoring {
		return errors.NewUnavailable(controllerNotReady)
	}
	if err := c.createEntity(ctx, req.ID, topo.RackKind, nil, labels(req.PodID, req.ID)); err != nil {
		return err
	}
	return c.createRelation(ctx, req.PodID, req.ID, topo.CONTAINS)
}

// AddSwitch adds a new switch entity with the requisite aspects into a rack
func (c *Controller) AddSwitch(ctx context.Context, req *api.AddSwitchRequest) error {
	if c.getState() != Monitoring {
		return errors.NewUnavailable(controllerNotReady)
	}
	al := aspects(req.ChassisConfigID, req.PipelineConfigID, req.GNMIEndpoint, req.P4Endpoint)
	if err := c.createEntity(ctx, req.ID, topo.SwitchKind, al, labels(req.PodID, req.RackID)); err != nil {
		return err
	}
	return c.createRelation(ctx, req.RackID, req.ID, topo.CONTAINS)
}

// AddServerIPU adds a new server entity and an associated IPU entity, both with the requisite aspects into a rack
func (c *Controller) AddServerIPU(ctx context.Context, req *api.AddServerIPURequest) error {
	if c.getState() != Monitoring {
		return errors.NewUnavailable(controllerNotReady)
	}
	if err := c.createEntity(ctx, req.ID, topo.ServerKind, nil, labels(req.PodID, req.RackID)); err != nil {
		return err
	}
	if err := c.createRelation(ctx, req.RackID, req.ID, topo.CONTAINS); err != nil {
		return err
	}

	ipuID := fmt.Sprintf("%s-IPU", req.ID)
	al := aspects(req.ChassisConfigID, req.PipelineConfigID, req.GNMIEndpoint, req.P4Endpoint)
	if err := c.createEntity(ctx, ipuID, topo.IPUKind, al, labels(req.PodID, req.RackID)); err != nil {
		return err
	}
	return c.createRelation(ctx, req.ID, ipuID, topo.CONTAINS)
}

// Produces a set of aspects for Stratum switch/IPU entity
func aspects(chassisConfigID string, pipelineConfigID string, gnmiEndpoint string, p4Endpoint string) []proto.Message {
	return []proto.Message{
		&provisioner.DeviceConfig{
			ChassisConfigID:  provisioner.ConfigID(chassisConfigID),
			PipelineConfigID: provisioner.ConfigID(pipelineConfigID),
		},
		&topo.GNMIServer{Endpoint: endpoint(gnmiEndpoint)},
		&topo.P4RuntimeServer{Endpoint: endpoint(p4Endpoint)},
	}
}

// Produces an endpoint from a host:port string
func endpoint(ep string) *topo.Endpoint {
	fields := strings.Split(ep, ":")
	if len(fields) < 2 {
		return nil
	}
	port, err := strconv.ParseInt(fields[1], 10, 32)
	if err != nil {
		return nil
	}
	return &topo.Endpoint{Address: fields[0], Port: uint32(port)}
}

// Produces a map of pod and rack labels
func labels(pod string, rack string) map[string]string {
	return map[string]string{topo.PodKind: pod, topo.RackKind: rack}
}

func (c *Controller) createEntity(ctx context.Context, id string, kindID string, aspects []proto.Message, labels map[string]string) error {
	object, err := topo.NewEntity(topo.ID(id), topo.ID(kindID)).WithAspects(aspects...)
	if err != nil {
		return err
	}
	object.Labels = labels
	_, err = c.topoClient.Create(ctx, &topo.CreateRequest{Object: object})
	return err
}

func (c *Controller) createRelation(ctx context.Context, src string, tgt string, kindID string) error {
	relation := topo.NewRelation(topo.ID(src), topo.ID(tgt), topo.ID(kindID), nil)
	_, err := c.topoClient.Create(ctx, &topo.CreateRequest{Object: relation})
	return err
}
