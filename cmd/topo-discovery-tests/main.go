// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package main launches the integration tests
package main

import (
	"github.com/onosproject/helmit/pkg/registry"
	"github.com/onosproject/helmit/pkg/test"
	"github.com/onosproject/topo-discovery/test/basic"
)

func main() {
	registry.RegisterTestSuite("basic", &basic.TestSuite{})
	test.Main()
}
