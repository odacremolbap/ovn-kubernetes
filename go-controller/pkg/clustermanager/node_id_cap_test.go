// SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
// SPDX-License-Identifier: Apache-2.0

package clustermanager

import (
	"context"
	"fmt"
	"net"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/config"
	ipgenerator "github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/generator/ip"
	"github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/types"
	"github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/util"
)

// TestNodeIDsAboveFiveThousand allocates IDs to more than 5000 nodes and checks that
// every ID is unique and in range, that each transit switch port address is the
// transit subnet base plus the node ID, and that the allocator is full at maxNodeIDs.
func TestNodeIDsAboveFiveThousand(t *testing.T) {
	const numNodes = 6500
	if numNodes >= maxNodeIDs-1 {
		t.Fatalf("numNodes %d must leave free IDs below maxNodeIDs %d", numNodes, maxNodeIDs)
	}

	t.Cleanup(func() { _ = config.PrepareTestConfig() })
	if err := config.PrepareTestConfig(); err != nil {
		t.Fatalf("failed to prepare the test config: %v", err)
	}
	// The default cluster subnet is IPv4 only; set it anyway to make the test's
	// assumption explicit.
	config.IPv4Mode = true
	config.IPv6Mode = false

	// The primary L2 UDN transit router port key is 10+ID (transitRouterPortFirstTunnelKey
	// in package ovn).
	if maxNodeIDs+10 > types.MaxLogicalPortTunnelKey {
		t.Fatalf("maxNodeIDs %d plus 10 is above the largest logical port tunnel key %d",
			maxNodeIDs, types.MaxLogicalPortTunnelKey)
	}
	joinIPGenerator, err := ipgenerator.NewIPGenerator(config.Gateway.V4JoinSubnet)
	if err != nil {
		t.Fatalf("failed to create the join subnet IP generator: %v", err)
	}
	if _, err := joinIPGenerator.GenerateIP(maxNodeIDs); err != nil {
		t.Fatalf("node ID %d does not fit the join subnet %s: %v", maxNodeIDs, config.Gateway.V4JoinSubnet, err)
	}
	transitIPGenerator, err := ipgenerator.NewIPGenerator(config.ClusterManager.V4TransitSubnet)
	if err != nil {
		t.Fatalf("failed to create the transit subnet IP generator: %v", err)
	}
	if _, _, err := transitIPGenerator.GenerateIPPair(maxNodeIDs); err != nil {
		t.Fatalf("node ID %d does not fit a transit router /31 in %s: %v",
			maxNodeIDs, config.ClusterManager.V4TransitSubnet, err)
	}

	nodes := make([]runtime.Object, 0, numNodes)
	for i := 0; i < numNodes; i++ {
		nodes = append(nodes, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("node-%d", i)}})
	}
	kubeClient := fake.NewSimpleClientset(nodes...)
	nac, err := newNodeAllocationController(&util.OVNClusterManagerClientset{KubeClient: kubeClient}, nil)
	if err != nil {
		t.Fatalf("failed to create the node allocation controller: %v", err)
	}
	_, transitSubnet, err := net.ParseCIDR(config.ClusterManager.V4TransitSubnet)
	if err != nil {
		t.Fatalf("failed to parse the transit subnet: %v", err)
	}

	seen := make(map[int]string, numNodes)
	for _, obj := range nodes {
		node := obj.(*corev1.Node)
		if err := nac.handleAddUpdateNodeEvent(node); err != nil {
			t.Fatalf("failed to allocate node %s: %v", node.Name, err)
		}
		updated, err := kubeClient.CoreV1().Nodes().Get(context.Background(), node.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("failed to get node %s: %v", node.Name, err)
		}
		nodeID, err := util.GetNodeID(updated)
		if err != nil {
			t.Fatalf("node %s has no node ID: %v", node.Name, err)
		}
		if nodeID < 2 || nodeID > maxNodeIDs {
			t.Fatalf("node %s got ID %d, outside [2, %d]", node.Name, nodeID, maxNodeIDs)
		}
		if other, ok := seen[nodeID]; ok {
			t.Fatalf("nodes %s and %s share ID %d", other, node.Name, nodeID)
		}
		seen[nodeID] = node.Name

		addrs, err := util.ParseNodeTransitSwitchPortAddrs(updated)
		if err != nil {
			t.Fatalf("node %s has no transit switch port address: %v", node.Name, err)
		}
		want, err := transitIPGenerator.GenerateIP(nodeID)
		if err != nil {
			t.Fatalf("failed to generate the transit switch port address for ID %d: %v", nodeID, err)
		}
		if len(addrs) != 1 || addrs[0].String() != want.String() {
			t.Fatalf("node %s with ID %d has transit switch port addresses %v, want [%s]", node.Name, nodeID, addrs, want)
		}
		if !transitSubnet.Contains(addrs[0].IP) {
			t.Fatalf("node %s transit switch port address %s is outside %s", node.Name, addrs[0], transitSubnet)
		}
	}

	// IDs 0 and 1 are reserved, so maxNodeIDs-1 nodes fit. Fill the rest of the
	// allocator, then the next node must be refused.
	for i := numNodes; i < maxNodeIDs-1; i++ {
		nodeID, err := nac.nodeIDAllocator.AllocateID(fmt.Sprintf("node-%d", i))
		if err != nil {
			t.Fatalf("failed to allocate ID number %d of %d: %v", i+1, maxNodeIDs-1, err)
		}
		if nodeID < 2 || nodeID > maxNodeIDs {
			t.Fatalf("allocated ID %d, outside [2, %d]", nodeID, maxNodeIDs)
		}
	}
	if nodeID, err := nac.nodeIDAllocator.AllocateID("one-too-many"); err == nil {
		t.Fatalf("allocated ID %d beyond the %d available", nodeID, maxNodeIDs-1)
	}
}
