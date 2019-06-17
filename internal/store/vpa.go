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
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"

	autoscaling "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1beta2"
	vpaclientset "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kube-state-metrics/pkg/metric"
)

var (
	descVerticalPodAutoscalerLabelsName          = "kube_vpa_labels"
	descVerticalPodAutoscalerLabelsHelp          = "Kubernetes labels converted to Prometheus labels."
	descVerticalPodAutoscalerLabelsDefaultLabels = []string{"namespace", "vpa", "targetRef"}

	vpaMetricFamilies = []metric.FamilyGenerator{
		{
			Name: descVerticalPodAutoscalerLabelsName,
			Type: metric.Gauge,
			Help: descVerticalPodAutoscalerLabelsHelp,
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				labelKeys, labelValues := kubeLabelsToPrometheusLabels(a.Labels)
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
							Value:       1,
						},
					},
				}
			}),
		},
		{
			Name: "kube_vpa_update_mode",
			Type: metric.Gauge,
			Help: "Update mode of the VPA.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}

				if a.Spec.UpdatePolicy.UpdateMode == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}

				for _, mode := range []autoscaling.UpdateMode{
					autoscaling.UpdateModeOff,
					autoscaling.UpdateModeInitial,
					autoscaling.UpdateModeRecreate,
					autoscaling.UpdateModeAuto,
				} {
					var v float64
					if *a.Spec.UpdatePolicy.UpdateMode == mode {
						v = 1
					} else {
						v = 0
					}
					ms = append(ms, &metric.Metric{
						LabelKeys:   []string{"update_mode"},
						LabelValues: []string{string(mode)},
						Value:       v,
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_resource_policy_min_cpu_cores",
			Type: metric.Gauge,
			Help: "Minimum CPU cores the VPA can set for containers matching the name.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				for _, c := range a.Spec.ResourcePolicy.ContainerPolicies {
					min := c.MinAllowed
					if cpu, ok := min[v1.ResourceCPU]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(cpu.MilliValue()) / 1000,
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_resource_policy_min_memory_bytes",
			Type: metric.Gauge,
			Help: "Minimum memory bytes the VPA can set for containers matching the name.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				for _, c := range a.Spec.ResourcePolicy.ContainerPolicies {
					min := c.MinAllowed
					if mem, ok := min[v1.ResourceMemory]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(mem.Value()),
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_resource_policy_max_cpu_cores",
			Type: metric.Gauge,
			Help: "Maximum CPU cores the VPA can set for containers matching the name.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				for _, c := range a.Spec.ResourcePolicy.ContainerPolicies {
					max := c.MaxAllowed
					if cpu, ok := max[v1.ResourceCPU]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(cpu.MilliValue()) / 1000,
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_resource_policy_max_memory_bytes",
			Type: metric.Gauge,
			Help: "Maximum memory bytes the VPA can set for containers matching the name.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				for _, c := range a.Spec.ResourcePolicy.ContainerPolicies {
					max := c.MaxAllowed
					if mem, ok := max[v1.ResourceMemory]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(mem.Value()),
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_status_recommendation_lower_bound_cpu_cores",
			Type: metric.Gauge,
			Help: "Minimum CPU cores the container can use before the VPA updater evicts it.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					v := c.LowerBound
					if cpu, ok := v[v1.ResourceCPU]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(cpu.MilliValue()) / 1000,
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_status_recommendation_lower_bound_memory_bytes",
			Type: metric.Gauge,
			Help: "Minimum memory bytes the container can use before the VPA updater evicts it.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					v := c.LowerBound
					if mem, ok := v[v1.ResourceMemory]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(mem.Value()),
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_status_recommendation_upper_bound_cpu_cores",
			Type: metric.Gauge,
			Help: "Maximum CPU cores the container can use before the VPA updater evicts it.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					v := c.UpperBound
					if cpu, ok := v[v1.ResourceCPU]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(cpu.MilliValue()) / 1000,
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_status_recommendation_upper_bound_memory_bytes",
			Type: metric.Gauge,
			Help: "Maximum memory bytes the container can use before the VPA updater evicts it.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					v := c.UpperBound
					if mem, ok := v[v1.ResourceMemory]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(mem.Value()),
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_status_recommendation_target_cpu_cores",
			Type: metric.Gauge,
			Help: "Target CPU cores the VPA recommends for the container.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					v := c.Target
					if cpu, ok := v[v1.ResourceCPU]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(cpu.MilliValue()) / 1000,
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_status_recommendation_target_memory_bytes",
			Type: metric.Gauge,
			Help: "Target memory bytes the VPA recommends for the container.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					v := c.Target
					if mem, ok := v[v1.ResourceMemory]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(mem.Value()),
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_status_recommendation_uncapped_target_cpu_cores",
			Type: metric.Gauge,
			Help: "Target CPU cores the VPA recommends for the container ignoring bounds.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					v := c.UncappedTarget
					if cpu, ok := v[v1.ResourceCPU]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(cpu.MilliValue()) / 1000,
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
		{
			Name: "kube_vpa_container_status_recommendation_uncapped_target_memory_bytes",
			Type: metric.Gauge,
			Help: "Target memory bytes the VPA recommends for the container ignoring bounds.",
			GenerateFunc: wrapVPAFunc(func(a *autoscaling.VerticalPodAutoscaler) *metric.Family {
				ms := []*metric.Metric{}
				if a.Status.Recommendation == nil || a.Status.Recommendation.ContainerRecommendations == nil {
					return &metric.Family{
						Metrics: ms,
					}
				}
				for _, c := range a.Status.Recommendation.ContainerRecommendations {
					v := c.UncappedTarget
					if mem, ok := v[v1.ResourceMemory]; ok {
						ms = append(ms, &metric.Metric{
							LabelKeys:   []string{"container_name"},
							LabelValues: []string{c.ContainerName},
							Value:       float64(mem.Value()),
						})
					}
				}
				return &metric.Family{
					Metrics: ms,
				}
			}),
		},
	}
)

func wrapVPAFunc(f func(*autoscaling.VerticalPodAutoscaler) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		vpa := obj.(*autoscaling.VerticalPodAutoscaler)

		metricFamily := f(vpa)
		targetRef := fmt.Sprintf("%s/%s/%s", vpa.Spec.TargetRef.APIVersion, vpa.Spec.TargetRef.Kind, vpa.Spec.TargetRef.Name)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(descVerticalPodAutoscalerLabelsDefaultLabels, m.LabelKeys...)
			m.LabelValues = append([]string{vpa.Namespace, vpa.Name, targetRef}, m.LabelValues...)
		}

		return metricFamily
	}
}

func createVPAListWatchFunc(kubeCfg *rest.Config) func(kubeClient clientset.Interface, ns string) cache.ListerWatcher {
	vpaClient, err := vpaclientset.NewForConfig(kubeCfg)
	if err != nil {
		panic(fmt.Sprintf("error creating VerticalPodAutoscaler client: %s", err.Error()))
	}
	return func(kubeClient clientset.Interface, ns string) cache.ListerWatcher {
		return &cache.ListWatch{
			ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
				return vpaClient.AutoscalingV1beta2().VerticalPodAutoscalers(ns).List(opts)
			},
			WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
				return vpaClient.AutoscalingV1beta2().VerticalPodAutoscalers(ns).Watch(opts)
			},
		}
	}
}
