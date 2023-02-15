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
	al := aspects(req.ManagementInfo)
	topoLabels := allLabels(req.PodID, req.RackID, req.ManagementInfo)
	if err := c.createEntity(ctx, req.ID, topo.SwitchKind, al, topoLabels); err != nil {
		return err
	}
	return c.createRelation(ctx, req.RackID, req.ID, topo.CONTAINS)
}

// AddServerIPU adds a new server entity and an associated IPU entity, both with the requisite aspects into a rack
func (c *Controller) AddServerIPU(ctx context.Context, req *api.AddServerIPURequest) error {
	if c.getState() != Monitoring {
		return errors.NewUnavailable(controllerNotReady)
	}
	topoLabels := allLabels(req.PodID, req.RackID, req.ManagementInfo)
	if err := c.createEntity(ctx, req.ID, topo.ServerKind, nil, topoLabels); err != nil {
		return err
	}
	if err := c.createRelation(ctx, req.RackID, req.ID, topo.CONTAINS); err != nil {
		return err
	}

	ipuID := fmt.Sprintf("%s-IPU", req.ID)
	al := aspects(req.ManagementInfo)
	if err := c.createEntity(ctx, ipuID, topo.IPUKind, al, topoLabels); err != nil {
		return err
	}
	return c.createRelation(ctx, req.ID, ipuID, topo.CONTAINS)
}

// Produces a set of aspects for Stratum switch/IPU entity
func aspects(info *api.ManagementInfo) []proto.Message {
	list := make([]proto.Message, 0, 1)
	if len(info.GNMIEndpoint) > 0 || len(info.P4RTEndpoint) > 0 {
		list = append(list, &topo.StratumAgents{
			P4RTEndpoint: endpoint(info.P4RTEndpoint),
			GNMIEndpoint: endpoint(info.GNMIEndpoint),
			DeviceID:     info.DeviceID,
		},
			// TODO: Remove when these aspects are deprecated
			&topo.P4RuntimeServer{Endpoint: endpoint(info.P4RTEndpoint), DeviceID: info.DeviceID},
			&topo.GNMIServer{Endpoint: endpoint(info.GNMIEndpoint)},
		)
	}

	if len(info.ChassisConfigID) > 0 || len(info.PipelineConfigID) > 0 {
		list = append(list, &provisioner.DeviceConfig{
			ChassisConfigID:  provisioner.ConfigID(info.ChassisConfigID),
			PipelineConfigID: provisioner.ConfigID(info.PipelineConfigID),
		})
	}

	if len(info.LinkAgentEndpoint) > 0 || len(info.HostAgentEndpoint) > 0 || len(info.NatAgentEndpoint) > 0 {
		list = append(list, &topo.LocalAgents{
			LinkAgentEndpoint: endpoint(info.LinkAgentEndpoint),
			HostAgentEndpoint: endpoint(info.HostAgentEndpoint),
			NATAgentEndpoint:  endpoint(info.NatAgentEndpoint),
		})
	}
	return list
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
	labels := make(map[string]string, 0)
	if len(pod) > 0 {
		labels[topo.PodKind] = pod
	}
	if len(rack) > 0 {
		labels[topo.RackKind] = rack
	}
	return labels
}

func allLabels(pod string, rack string, info *api.ManagementInfo) map[string]string {
	labels := labels(pod, rack)
	if len(info.Realm) > 0 {
		labels["realm"] = info.Realm
	}
	if len(info.Role) > 0 {
		labels["role"] = info.Role
	}
	return labels
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
	if len(src) > 0 {
		relation := topo.NewRelation(topo.ID(src), topo.ID(tgt), topo.ID(kindID), nil)
		_, err := c.topoClient.Create(ctx, &topo.CreateRequest{Object: relation})
		return err
	}
	return nil
}
