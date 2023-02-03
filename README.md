![Build](https://github.com/onosproject/topo-discovery/workflows/build/badge.svg)
![Coverage](https://img.shields.io/badge/Coverage-0.0%25-red)

<!--
SPDX-FileCopyrightText: 2022 Intel Corporation

SPDX-License-Identifier: Apache-2.0
-->

# topo-discovery
Subsystem for discovering network topology using P4Runtime and gNMI. The component stitches together 
the topology connectivity graph by interacting with the following device agents:

* Stratum Agent – for port information
* Link Local Agent – for ingress link information
* Host Local Agent – for attached hosts (bare-metal and VMs) information

The design assumes that all major assets will have entities pre-configured in the `onos-topo`.
This includes switches, bare-metal servers and embedded IPUs including any aspects required for 
making connections to the above agents. (See Topology Bootstrap Service section below)

The discovery engine operation is driven by the contents of the `onos-topo` subsystem, and conversely,
all its persistent state is maintained using the `onos-topo` subsystem.

The operation of the engine will be triggered by changes in `onos-topo` subsystem, but also by periodic
sweep over relevant root entities.

## Realms
Multiple instances of this component can operate, dividing responsibility for stitching together
different parts of the topology. This partitioning is accomplished using labels applied to the
topology entities. For example, a pod-specific topology discovery instance can be deployed with
realm label `pod` and realm value `pod-07`.

## Workers
The subsystem will use a bank of reconciler workers to perform the following reconciliation activities:
* reconcile device ports
* reconcile ingress links
* reconcile hosts

## Port Reconciliation
Root entities for port reconciliation are entities of Switch and IPU kind and with `onos.topo.StratumAgents` aspect.

The following is a rough outline of the reconciliation process:
* connect to server
* get ports via gNMI
* get device port entities from topology
* for each gNMI port
  * create/update port object with port aspect if needed
  * create switch -> port `has` relation if needed
* remove any ports if needed

## Ingress Link Reconciliation
Root entities for link reconciliation are of Switch and IPU kind and with `onos.topo.LocalAgents` aspect

The following is a rough outline of the ingress link reconciliation process:
* connect to server
* get links via gNMI
* get ingress link entities from topology
* for each gNMI link
  * create/update link entity if needed
  * create port -> link `originates`/`terminates` relations if needed
* mark inactive any links if needed

## Topology Bootstrap Service
The main goal of this API is to simplify initial creation of the core onos-topo entities and relations 
(tagged with appropriate kinds, realm labels and required aspects) that represent the major network assets:

* Pods
* Racks in Pods
* Switches in Racks (stratum, link and host agent endpoints)
* Servers in Racks (management address)
* IPUs in Servers (stratum, link and host agent endpoints)

This facility de-facto raises the level of abstraction for the topology API, while making it specific to the
SD-Fabric use-case and for seeding the provisioning/discovery activities. As such, it will focus purely on
initial creation of the above entities. The underlying `onos-topo` API will of course remain topology agnostic.

Rather than creating a separate service component, the proposal is to have the main topology discovery offer 
this API service as it will be the principal consumer of the information – specifically the stratum agent, 
link agent and host agent end-points.

The service would also include a command-line facility for ingesting a simple YAML file to create the entire
pod deployment at once - primarily for easy creation of a simulated environment in a single go.

This facility would also allow for injection of IPU/leaf switch links as a provisional feature.

