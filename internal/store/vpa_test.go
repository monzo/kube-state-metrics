/*
Copyright 2019 The Kubernetes Authors All rights reserved.

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

package store

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	k8sautoscaling "k8s.io/api/autoscaling/v1"
	autoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1beta2"
	"k8s.io/kube-state-metrics/pkg/metric"
)

func TestVPAStore(t *testing.T) {

	const metadata = `
	`

	updateMode := autoscaling.UpdateModeRecreate

	v1Resource := func(cpu, mem string) v1.ResourceList {
		return v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse(cpu),
			v1.ResourceMemory: resource.MustParse(mem),
		}
	}

	cases := []generateMetricsTestCase{
		{
			Obj: &autoscaling.VerticalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 2,
					Name:       "vpa1",
					Namespace:  "ns1",
					Labels: map[string]string{
						"app": "foobar",
					},
				},
				Spec: autoscaling.VerticalPodAutoscalerSpec{
					TargetRef: &k8sautoscaling.CrossVersionObjectReference{
						APIVersion: "extensions/v1beta1",
						Kind:       "Deployment",
						Name:       "deployment1",
					},
					UpdatePolicy: &autoscaling.PodUpdatePolicy{
						UpdateMode: &updateMode,
					},
					ResourcePolicy: &autoscaling.PodResourcePolicy{
						ContainerPolicies: []autoscaling.ContainerResourcePolicy{
							{
								ContainerName: "*",
								MinAllowed:    v1Resource("1", "4Gi"),
								MaxAllowed:    v1Resource("4", "8Gi"),
							},
						},
					},
				},
				Status: autoscaling.VerticalPodAutoscalerStatus{
					Recommendation: &autoscaling.RecommendedPodResources{
						ContainerRecommendations: []autoscaling.RecommendedContainerResources{
							{
								ContainerName:  "container1",
								LowerBound:     v1Resource("1", "4Gi"),
								UpperBound:     v1Resource("4", "8Gi"),
								Target:         v1Resource("3", "7Gi"),
								UncappedTarget: v1Resource("6", "10Gi"),
							},
						},
					},
				},
			},
			Want: `
				kube_vpa_container_resource_policy_max_cpu_cores{container_name="*",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 4
				kube_vpa_container_resource_policy_max_memory_bytes{container_name="*",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 8.589934592e+09
				kube_vpa_container_resource_policy_min_cpu_cores{container_name="*",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 1
				kube_vpa_container_resource_policy_min_memory_bytes{container_name="*",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 4.294967296e+09
				kube_vpa_container_status_recommendation_lower_bound_cpu_cores{container_name="container1",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 1
				kube_vpa_container_status_recommendation_lower_bound_memory_bytes{container_name="container1",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 4.294967296e+09
				kube_vpa_container_status_recommendation_target_cpu_cores{container_name="container1",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 3
				kube_vpa_container_status_recommendation_target_memory_bytes{container_name="container1",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 7.516192768e+09
				kube_vpa_container_status_recommendation_uncapped_target_cpu_cores{container_name="container1",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 6
				kube_vpa_container_status_recommendation_uncapped_target_memory_bytes{container_name="container1",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 1.073741824e+10
				kube_vpa_container_status_recommendation_upper_bound_cpu_cores{container_name="container1",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 4
				kube_vpa_container_status_recommendation_upper_bound_memory_bytes{container_name="container1",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 8.589934592e+09
				kube_vpa_labels{label_app="foobar",namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",vpa="vpa1"} 1
				kube_vpa_update_mode{namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",update_mode="Auto",vpa="vpa1"} 0
				kube_vpa_update_mode{namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",update_mode="Initial",vpa="vpa1"} 0
				kube_vpa_update_mode{namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",update_mode="Off",vpa="vpa1"} 0
				kube_vpa_update_mode{namespace="ns1",targetRef="extensions/v1beta1/Deployment/deployment1",update_mode="Recreate",vpa="vpa1"} 1
			`,
			MetricNames: []string{
				"kube_vpa_labels",
				"kube_vpa_update_mode",
				"kube_vpa_container_resource_policy_min_cpu_cores",
				"kube_vpa_container_resource_policy_min_memory_bytes",
				"kube_vpa_container_resource_policy_max_cpu_cores",
				"kube_vpa_container_resource_policy_max_memory_bytes",
				"kube_vpa_container_status_recommendation_lower_bound_cpu_cores",
				"kube_vpa_container_status_recommendation_lower_bound_memory_bytes",
				"kube_vpa_container_status_recommendation_upper_bound_cpu_cores",
				"kube_vpa_container_status_recommendation_upper_bound_memory_bytes",
				"kube_vpa_container_status_recommendation_target_cpu_cores",
				"kube_vpa_container_status_recommendation_target_memory_bytes",
				"kube_vpa_container_status_recommendation_uncapped_target_cpu_cores",
				"kube_vpa_container_status_recommendation_uncapped_target_memory_bytes",
			},
		},
	}
	for i, c := range cases {
		c.Func = metric.ComposeMetricGenFuncs(vpaMetricFamilies)
		if err := c.run(); err != nil {
			t.Errorf("unexpected collecting result in %vth run:\n%s", i, err)
		}
	}
}
