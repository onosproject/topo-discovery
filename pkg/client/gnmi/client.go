// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package gnmi

import (
	"context"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"io"

	"github.com/onosproject/onos-lib-go/pkg/errors"
	baseClient "github.com/openconfig/gnmi/client"
	gclient "github.com/openconfig/gnmi/client/gnmi"

	gpb "github.com/openconfig/gnmi/proto/gnmi"
)

var log = logging.GetLogger()

// Client gNMI client interface
type Client interface {
	io.Closer
	Capabilities(ctx context.Context, r *gpb.CapabilityRequest) (*gpb.CapabilityResponse, error)
	Get(ctx context.Context, r *gpb.GetRequest) (*gpb.GetResponse, error)
	Set(ctx context.Context, r *gpb.SetRequest) (*gpb.SetResponse, error)
	Subscribe(ctx context.Context, q baseClient.Query) error
	Poll() error
}

// client gnmi client
type client struct {
	client *gclient.Client
}

// Subscribe calls gNMI subscription bacc
// sed on a given query
func (c *client) Subscribe(ctx context.Context, q baseClient.Query) error {
	err := c.client.Subscribe(ctx, q)
	go c.run(ctx)
	return errors.FromGRPC(err)
}

// Poll issues a poll request using the backing client
func (c *client) Poll() error {
	return c.client.Poll()
}

func (c *client) run(ctx context.Context) {
	log.Infof("Subscription response monitor started")
	for {
		err := c.client.Recv()
		if err != nil {
			log.Infof("Subscription response monitor stopped due to %v", err)
			return
		}
	}
}

// Capabilities returns the capabilities of the target
func (c *client) Capabilities(ctx context.Context, req *gpb.CapabilityRequest) (*gpb.CapabilityResponse, error) {
	capResponse, err := c.client.Capabilities(ctx, req)
	return capResponse, errors.FromGRPC(err)
}

// Get calls gnmi Get RPC
func (c *client) Get(ctx context.Context, req *gpb.GetRequest) (*gpb.GetResponse, error) {
	getResponse, err := c.client.Get(ctx, req)
	return getResponse, errors.FromGRPC(err)
}

// Set calls gnmi Set RPC
func (c *client) Set(ctx context.Context, req *gpb.SetRequest) (*gpb.SetResponse, error) {
	setResponse, err := c.client.Set(ctx, req)
	return setResponse, errors.FromGRPC(err)
}

// Close closes the gnmi client
func (c *client) Close() error {
	return c.client.Close()
}

var _ Client = &client{}
