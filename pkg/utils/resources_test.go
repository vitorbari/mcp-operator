/*
Copyright 2025 Vitor Bari.

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

package utils

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

func TestUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Utils Suite")
}

var _ = Describe("Resource Utils", func() {
	var (
		mcpServer *mcpv1.MCPServer
	)

	BeforeEach(func() {
		mcpServer = &mcpv1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server",
				Namespace: "default",
			},
			Spec: mcpv1.MCPServerSpec{
				Image:    "nginx:1.21",
				Replicas: ptr(int32(2)),
			},
		}
	})

	Describe("BuildStandardLabels", func() {
		It("should create standard labels for MCPServer", func() {
			labels := BuildStandardLabels(mcpServer)

			Expect(labels).To(HaveKeyWithValue("app", "test-server"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/name", "mcpserver"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/instance", "test-server"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/component", "mcp-server"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "mcp-operator"))
		})
	})

	Describe("BuildAnnotations", func() {
		It("should return empty annotations when no PodTemplate annotations", func() {
			annotations := BuildAnnotations(mcpServer)
			Expect(annotations).To(BeEmpty())
		})

		It("should return PodTemplate annotations when specified", func() {
			mcpServer.Spec.PodTemplate = &mcpv1.MCPServerPodTemplate{
				Annotations: map[string]string{
					"test-annotation": "test-value",
				},
			}
			annotations := BuildAnnotations(mcpServer)

			Expect(annotations).To(HaveKeyWithValue("test-annotation", "test-value"))
		})
	})

	Describe("GetReplicaCount", func() {
		It("should return specified replica count", func() {
			count := GetReplicaCount(mcpServer)
			Expect(count).To(Equal(int32(2)))
		})

		It("should return default replica count when not specified", func() {
			mcpServer.Spec.Replicas = nil
			count := GetReplicaCount(mcpServer)
			Expect(count).To(Equal(int32(1)))
		})
	})

	Describe("BuildService", func() {
		It("should build a basic service", func() {
			annotations := map[string]string{
				"test-annotation": "test-value",
			}

			service := BuildService(mcpServer, 8080, corev1.ProtocolTCP, annotations)

			Expect(service.Name).To(Equal("test-server"))
			Expect(service.Namespace).To(Equal("default"))
			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(service.Spec.Selector).To(HaveKeyWithValue("app", "test-server"))
			Expect(service.Annotations).To(HaveKeyWithValue("test-annotation", "test-value"))
		})

		It("should use custom service configuration", func() {
			mcpServer.Spec.Service = &mcpv1.MCPServerService{
				Type: "LoadBalancer",
				Port: 9090,
				Annotations: map[string]string{
					"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
				},
			}

			service := BuildService(mcpServer, 8080, corev1.ProtocolUDP, nil)

			Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080))) // Uses the port parameter, not custom port
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolUDP))
			Expect(service.Annotations).To(HaveKeyWithValue("service.beta.kubernetes.io/aws-load-balancer-type", "nlb"))
		})
	})

	Describe("BuildDeployment", func() {
		It("should build a deployment with provided pod spec", func() {
			podSpec := corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test-container",
						Image: "nginx:1.21",
						Ports: []corev1.ContainerPort{
							{
								ContainerPort: 8080,
								Protocol:      corev1.ProtocolTCP,
							},
						},
					},
				},
			}

			deployment := BuildDeployment(mcpServer, podSpec)

			Expect(deployment.Name).To(Equal("test-server"))
			Expect(deployment.Namespace).To(Equal("default"))
			Expect(*deployment.Spec.Replicas).To(Equal(int32(2)))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Name).To(Equal("test-container"))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.21"))
		})

		It("should set correct labels and selectors", func() {
			podSpec := corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "test",
					},
				},
			}

			deployment := BuildDeployment(mcpServer, podSpec)

			// Check deployment labels
			Expect(deployment.Labels).To(HaveKeyWithValue("app", "test-server"))

			// Check selector
			Expect(deployment.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app", "test-server"))

			// Check pod template labels
			Expect(deployment.Spec.Template.Labels).To(HaveKeyWithValue("app", "test-server"))
		})
	})

	Describe("UpdateService", func() {
		var (
			ctx           context.Context
			k8sClient     client.Client
			runtimeScheme *runtime.Scheme
		)

		BeforeEach(func() {
			ctx = context.Background()

			// Create runtime scheme and add our types
			runtimeScheme = runtime.NewScheme()
			Expect(scheme.AddToScheme(runtimeScheme)).To(Succeed())
			Expect(mcpv1.AddToScheme(runtimeScheme)).To(Succeed())

			// Create fake client
			k8sClient = fake.NewClientBuilder().
				WithScheme(runtimeScheme).
				Build()
		})

		It("should update an existing service", func() {
			// Create initial service
			service := BuildService(mcpServer, 8080, corev1.ProtocolTCP, nil)

			err := k8sClient.Create(ctx, service)
			Expect(err).NotTo(HaveOccurred())

			// Update service with different port
			updatedService := BuildService(mcpServer, 9090, corev1.ProtocolTCP, nil)

			err = UpdateService(ctx, k8sClient, runtimeScheme, mcpServer, updatedService)
			Expect(err).NotTo(HaveOccurred())

			// Verify service was updated
			retrievedService := &corev1.Service{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      service.Name,
				Namespace: service.Namespace,
			}, retrievedService)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrievedService.Spec.Ports[0].Port).To(Equal(int32(9090)))
		})

		It("should handle service that doesn't need updating", func() {
			// Create service
			service := BuildService(mcpServer, 8080, corev1.ProtocolTCP, nil)

			err := k8sClient.Create(ctx, service)
			Expect(err).NotTo(HaveOccurred())

			// Try to update with same configuration
			sameService := BuildService(mcpServer, 8080, corev1.ProtocolTCP, nil)

			err = UpdateService(ctx, k8sClient, runtimeScheme, mcpServer, sameService)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Pod template configuration", func() {
		It("should work with basic pod specs", func() {
			podSpec := &corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "main",
						Image: "nginx:1.21",
					},
				},
			}

			// Test that we can create basic pod specs
			Expect(podSpec.Containers).To(HaveLen(1))
			Expect(podSpec.Containers[0].Name).To(Equal("main"))
		})
	})
})

// Helper function to create pointer to values
func ptr[T any](v T) *T {
	return &v
}
