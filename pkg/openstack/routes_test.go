/*
Copyright 2016 The Kubernetes Authors.

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

package openstack

import (
	"context"
	"net"
	"testing"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/routers"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/cloud-provider-openstack/pkg/client"
)

func TestRoutes(t *testing.T) {
	const clusterName = "ignored"

	cfg := ConfigFromEnv()
	testConfigFromEnv(t, &cfg)

	os, err := NewOpenStack(cfg)
	if err != nil {
		t.Fatalf("Failed to construct/authenticate OpenStack: %v", err)
	}

	vms := getServers(os)
	_, err = os.InstanceID()
	if err != nil || len(vms) == 0 {
		t.Skipf("Please run this test in an OpenStack vm or create at least one VM in OpenStack before you run this test.")
	}

	// We know we have at least one vm.
	servername := vms[0].Name

	// Pick the first router and server to try a test with
	os.routeOpts.RouterID = getRouters(os)[0].ID

	r, ok := os.Routes()
	if !ok {
		t.Skip("Routes() returned false - perhaps your stack does not support Neutron extraroute extension?")
	}

	newroute := cloudprovider.Route{
		DestinationCIDR: "10.164.2.0/24",
		TargetNode:      types.NodeName(servername),
	}
	err = r.CreateRoute(context.TODO(), clusterName, "myhint", &newroute)
	if err != nil {
		t.Fatalf("CreateRoute error: %v", err)
	}

	routelist, err := r.ListRoutes(context.TODO(), clusterName)
	if err != nil {
		t.Fatalf("ListRoutes() error: %v", err)
	}
	for _, route := range routelist {
		_, cidr, err := net.ParseCIDR(route.DestinationCIDR)
		if err != nil {
			t.Logf("Ignoring route %s, unparsable CIDR: %v", route.Name, err)
			continue
		}
		t.Logf("%s via %s", cidr, route.TargetNode)
	}

	err = r.DeleteRoute(context.TODO(), clusterName, &newroute)
	if err != nil {
		t.Fatalf("DeleteRoute error: %v", err)
	}
}

func TestGetAddrByNodeName(t *testing.T) {
	fixtures := [][]*v1.Node{
		{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: "2001:4800:790e::82a8",
						},
						{
							Type:    v1.NodeInternalIP,
							Address: "1.2.3.4",
						},
						{
							Type:    v1.NodeInternalIP,
							Address: "2001:4800:790e::82a9",
						},
					},
				},
			},
		},
		{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node-1",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: "1.2.3.5",
						},
						{
							Type:    v1.NodeInternalIP,
							Address: "2001:4800:790e::82a8",
						},
						{
							Type:    v1.NodeInternalIP,
							Address: "1.2.3.4",
						},
						{
							Type:    v1.NodeInternalIP,
							Address: "2001:4800:790e::82a9",
						},
					},
				},
			},
		},
	}
	tests := []struct {
		ipv6 bool
		ip   string
	}{
		{
			ipv6: false,
			ip:   "1.2.3.4",
		},
		{
			ipv6: true,
			ip:   "2001:4800:790e::82a8",
		},
	}

	for i, v := range fixtures {
		ip := getAddrByNodeName("test-node-1", tests[i].ipv6, v)

		if expected := tests[i].ip; expected != ip {
			t.Fatalf("Expected %q IP doesn't correspond %q actual IP", expected, ip)
		}
	}
}

func getServers(os *OpenStack) []servers.Server {
	c, err := client.NewComputeV2(os.provider, os.epOpts)
	if err != nil {
		panic(err)
	}
	opts := servers.ListOpts{
		Status: "ACTIVE",
	}
	allPages, err := servers.List(c, opts).AllPages()
	if err != nil {
		panic(err)
	}
	allServers, err := servers.ExtractServers(allPages)
	if err != nil {
		panic(err)
	}
	if len(allServers) == 0 {
		panic("No servers to test with")
	}
	return allServers
}

func getRouters(os *OpenStack) []routers.Router {
	listOpts := routers.ListOpts{}
	n, err := client.NewNetworkV2(os.provider, os.epOpts)
	if err != nil {
		panic(err)
	}
	allPages, err := routers.List(n, listOpts).AllPages()
	if err != nil {
		panic(err)
	}
	allRouters, err := routers.ExtractRouters(allPages)
	if err != nil {
		panic(err)
	}
	if len(allRouters) == 0 {
		panic("No routers to test with")
	}
	return allRouters
}
