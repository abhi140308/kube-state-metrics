/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"context"

	"k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

var (
	descPodDisruptionBudgetAnnotationsName     = "kube_poddisruptionbudget_annotations"
	descPodDisruptionBudgetAnnotationsHelp     = "Kubernetes annotations converted to Prometheus labels."
	descPodDisruptionBudgetLabelsName          = "kube_poddisruptionbudget_labels"
	descPodDisruptionBudgetLabelsHelp          = "Kubernetes labels converted to Prometheus labels."
	descPodDisruptionBudgetLabelsDefaultLabels = []string{"namespace", "poddisruptionbudget"}
)

func podDisruptionBudgetMetricFamilies(allowAnnotationsList, allowLabelsList []string) []generator.FamilyGenerator {
	return []generator.FamilyGenerator{
		*generator.NewFamilyGenerator(
			"kube_poddisruptionbudget_created",
			"Unix creation timestamp",
			metric.Gauge,
			"",
			wrapPodDisruptionBudgetFunc(func(p *v1beta1.PodDisruptionBudget) *metric.Family {
				ms := []*metric.Metric{}

				if !p.CreationTimestamp.IsZero() {
					ms = append(ms, &metric.Metric{
						Value: float64(p.CreationTimestamp.Unix()),
					})
				}

				return &metric.Family{
					Metrics: ms,
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"kube_poddisruptionbudget_status_current_healthy",
			"Current number of healthy pods",
			metric.Gauge,
			"",
			wrapPodDisruptionBudgetFunc(func(p *v1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.CurrentHealthy),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"kube_poddisruptionbudget_status_desired_healthy",
			"Minimum desired number of healthy pods",
			metric.Gauge,
			"",
			wrapPodDisruptionBudgetFunc(func(p *v1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.DesiredHealthy),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"kube_poddisruptionbudget_status_pod_disruptions_allowed",
			"Number of pod disruptions that are currently allowed",
			metric.Gauge,
			"",
			wrapPodDisruptionBudgetFunc(func(p *v1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.DisruptionsAllowed),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"kube_poddisruptionbudget_status_expected_pods",
			"Total number of pods counted by this disruption budget",
			metric.Gauge,
			"",
			wrapPodDisruptionBudgetFunc(func(p *v1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.ExpectedPods),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			"kube_poddisruptionbudget_status_observed_generation",
			"Most recent generation observed when updating this PDB status",
			metric.Gauge,
			"",
			wrapPodDisruptionBudgetFunc(func(p *v1beta1.PodDisruptionBudget) *metric.Family {
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							Value: float64(p.Status.ObservedGeneration),
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			descPodDisruptionBudgetAnnotationsName,
			descPersistentVolumeAnnotationsHelp,
			metric.Gauge,
			"",
			wrapPodDisruptionBudgetFunc(func(n *v1beta1.PodDisruptionBudget) *metric.Family {
				annotationKeys, annotationValues := createPrometheusLabelKeysValues("annotation", n.Annotations, allowAnnotationsList)
				return &metric.Family{
					Metrics: []*metric.Metric{
						{
							LabelKeys:   annotationKeys,
							LabelValues: annotationValues,
							Value:       1,
						},
					},
				}
			}),
		),
		*generator.NewFamilyGenerator(
			descPodDisruptionBudgetLabelsName,
			descPodDisruptionBudgetLabelsHelp,
			metric.Gauge,
			"",
			wrapPodDisruptionBudgetFunc(func(n *v1beta1.PodDisruptionBudget) *metric.Family {
				labelKeys, labelValues := createPrometheusLabelKeysValues("label", n.Labels, allowLabelsList)
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
		),
	}
}

func wrapPodDisruptionBudgetFunc(f func(*v1beta1.PodDisruptionBudget) *metric.Family) func(interface{}) *metric.Family {
	return func(obj interface{}) *metric.Family {
		podDisruptionBudget := obj.(*v1beta1.PodDisruptionBudget)

		metricFamily := f(podDisruptionBudget)

		for _, m := range metricFamily.Metrics {
			m.LabelKeys = append(descPodDisruptionBudgetLabelsDefaultLabels, m.LabelKeys...)
			m.LabelValues = append([]string{podDisruptionBudget.Namespace, podDisruptionBudget.Name}, m.LabelValues...)
		}

		return metricFamily
	}
}

func createPodDisruptionBudgetListWatch(kubeClient clientset.Interface, ns string, fieldSelector string) cache.ListerWatcher {
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return kubeClient.PolicyV1beta1().PodDisruptionBudgets(ns).List(context.TODO(), opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return kubeClient.PolicyV1beta1().PodDisruptionBudgets(ns).Watch(context.TODO(), opts)
		},
	}
}
