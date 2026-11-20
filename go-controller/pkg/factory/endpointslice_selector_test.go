// SPDX-FileCopyrightText: Copyright The OVN-Kubernetes Contributors
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/config"
	"github.com/ovn-kubernetes/ovn-kubernetes/go-controller/pkg/types"
)

// TestEndpointSliceSelector checks that the EndpointSlice watch never selects the
// slices of headless Services, with network segmentation on or off.
func TestEndpointSliceSelector(t *testing.T) {
	t.Cleanup(func() { _ = config.PrepareTestConfig() })

	tests := []struct {
		name         string
		segmentation bool
		labels       map[string]string
		match        bool
	}{
		{"service slice", false, map[string]string{discovery.LabelServiceName: "s"}, true},
		{"headless service slice", false, map[string]string{discovery.LabelServiceName: "s", corev1.IsHeadlessService: ""}, false},
		{"empty service name", false, map[string]string{discovery.LabelServiceName: ""}, false},
		{"no service name", false, map[string]string{}, false},
		{"mirrored slice", false, map[string]string{types.LabelUserDefinedServiceName: "s"}, false},
		{"service slice", true, map[string]string{discovery.LabelServiceName: "s"}, true},
		{"headless service slice", true, map[string]string{discovery.LabelServiceName: "s", corev1.IsHeadlessService: ""}, false},
		{"mirrored slice", true, map[string]string{types.LabelUserDefinedServiceName: "s"}, true},
		{"headless label without the default service name", true, map[string]string{types.LabelUserDefinedServiceName: "s", corev1.IsHeadlessService: ""}, false},
		{"no service name", true, map[string]string{}, true},
	}
	for _, tc := range tests {
		if err := config.PrepareTestConfig(); err != nil {
			t.Fatalf("failed to prepare the test config: %v", err)
		}
		config.OVNKubernetesFeature.EnableMultiNetwork = tc.segmentation
		config.OVNKubernetesFeature.EnableNetworkSegmentation = tc.segmentation

		selectorFunc := getEndpointSliceSelector()
		if selectorFunc == nil {
			t.Fatalf("segmentation=%t: nil selector, the watch would select every EndpointSlice", tc.segmentation)
		}
		opts := metav1.ListOptions{}
		selectorFunc(&opts)
		selector, err := labels.Parse(opts.LabelSelector)
		if err != nil {
			t.Fatalf("segmentation=%t: failed to parse selector %q: %v", tc.segmentation, opts.LabelSelector, err)
		}
		if got := selector.Matches(labels.Set(tc.labels)); got != tc.match {
			t.Errorf("segmentation=%t, %s %v: selector %q matches=%t, want %t",
				tc.segmentation, tc.name, tc.labels, opts.LabelSelector, got, tc.match)
		}
	}
}
