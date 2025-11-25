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

package transport

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

func TestHTTPTransport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Transport Suite")
}

var _ = Describe("HTTPResourceManager", func() {
	var (
		ctx           context.Context
		k8sClient     client.Client
		httpManager   *HTTPResourceManager
		mcpServer     *mcpv1.MCPServer
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

		// Create HTTP manager
		httpManager = NewHTTPResourceManager(k8sClient, runtimeScheme)

		// Create test MCPServer
		mcpServer = &mcpv1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-mcpserver",
				Namespace: "default",
			},
			Spec: mcpv1.MCPServerSpec{
				Image:    "nginx:1.21",
				Replicas: ptr(int32(2)),
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				Transport: &mcpv1.MCPServerTransport{
					Type: mcpv1.MCPTransportHTTP,
					Config: &mcpv1.MCPTransportConfigDetails{
						HTTP: &mcpv1.MCPHTTPTransportConfig{
							Port: 8080,
							Path: "/mcp",
						},
					},
				},
			},
		}
	})

	Describe("CreateResources", func() {
		It("should create deployment and service for HTTP transport", func() {
			err := httpManager.CreateResources(ctx, mcpServer)
			Expect(err).NotTo(HaveOccurred())

			// Check deployment was created
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("nginx:1.21"))
			Expect(container.Ports).To(HaveLen(1))
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(8080)))

			// Check MCP transport environment variable
			var transportEnv *corev1.EnvVar
			for _, env := range container.Env {
				if env.Name == "MCP_TRANSPORT" {
					transportEnv = &env
					break
				}
			}
			Expect(transportEnv).NotTo(BeNil())
			Expect(transportEnv.Value).To(Equal("http"))

			// Check service was created
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should create resources with custom HTTP port", func() {
			mcpServer.Spec.Transport.Config.HTTP.Port = 9090

			err := httpManager.CreateResources(ctx, mcpServer)
			Expect(err).NotTo(HaveOccurred())

			// Check deployment uses custom port
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(9090)))

			// Check service uses custom port
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports[0].Port).To(Equal(int32(9090)))
		})
	})

	Describe("UpdateResources", func() {
		BeforeEach(func() {
			// Create initial resources
			err := httpManager.CreateResources(ctx, mcpServer)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update existing deployment", func() {
			// Change replica count
			mcpServer.Spec.Replicas = ptr(int32(5))

			err := httpManager.UpdateResources(ctx, mcpServer)
			Expect(err).NotTo(HaveOccurred())

			// Check deployment was updated
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			Expect(*deployment.Spec.Replicas).To(Equal(int32(5)))
		})
	})

	Describe("GetTransportType", func() {
		It("should return HTTP transport type", func() {
			transportType := httpManager.GetTransportType()
			Expect(transportType).To(Equal(mcpv1.MCPTransportHTTP))
		})
	})

	Describe("RequiresService", func() {
		It("should return true for HTTP transport", func() {
			requiresService := httpManager.RequiresService()
			Expect(requiresService).To(BeTrue())
		})
	})

	Describe("Transport port handling", func() {
		It("should work with default HTTP port", func() {
			port := GetTransportPort(mcpServer)
			Expect(port).To(Equal(int32(8080)))
		})

		It("should work with custom HTTP port", func() {
			mcpServer.Spec.Transport.Config.HTTP.Port = 9090
			port := GetTransportPort(mcpServer)
			Expect(port).To(Equal(int32(9090)))
		})
	})
})

// Helper function to create pointer to values
func ptr[T any](v T) *T {
	return &v
}
