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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

var _ = Describe("CustomResourceManager", func() {
	var (
		ctx           context.Context
		k8sClient     client.Client
		customManager *CustomResourceManager
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

		// Create Custom manager
		customManager = NewCustomResourceManager(k8sClient, runtimeScheme)

		// Create test MCPServer
		mcpServer = &mcpv1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-custom-mcpserver",
				Namespace: "default",
			},
			Spec: mcpv1.MCPServerSpec{
				Image:    "custom-mcp:v1.0.0",
				Replicas: ptr(int32(1)),
				Transport: &mcpv1.MCPServerTransport{
					Type: mcpv1.MCPTransportCustom,
					Config: &mcpv1.MCPTransportConfigDetails{
						Custom: &mcpv1.MCPCustomTransportConfig{
							Protocol: "tcp",
							Port:     9000,
							Config: map[string]string{
								"buffer_size": "1024",
								"timeout":     "30s",
							},
						},
					},
				},
			},
		}
	})

	Describe("CreateResources", func() {
		It("should create deployment and service for TCP custom transport", func() {
			err := customManager.CreateResources(ctx, mcpServer)
			Expect(err).NotTo(HaveOccurred())

			// Check deployment was created
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("custom-mcp:v1.0.0"))
			Expect(container.Ports).To(HaveLen(1))
			Expect(container.Ports[0].ContainerPort).To(Equal(int32(9000)))
			Expect(container.Ports[0].Name).To(Equal(protocolTCP))

			// Check MCP transport environment variable
			var transportEnv *corev1.EnvVar
			for _, env := range container.Env {
				if env.Name == "MCP_TRANSPORT" {
					transportEnv = &env
					break
				}
			}
			Expect(transportEnv).NotTo(BeNil())
			Expect(transportEnv.Value).To(Equal("custom"))

			// Check custom config environment variables
			configEnvs := make(map[string]string)
			for _, env := range container.Env {
				if env.Name == "MCP_CUSTOM_buffer_size" {
					configEnvs["buffer_size"] = env.Value
				}
				if env.Name == "MCP_CUSTOM_timeout" {
					configEnvs["timeout"] = env.Value
				}
			}
			Expect(configEnvs).To(HaveKeyWithValue("buffer_size", "1024"))
			Expect(configEnvs).To(HaveKeyWithValue("timeout", "30s"))

			// Check service was created
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(9000)))
			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
		})

		It("should create resources for UDP custom transport", func() {
			mcpServer.Spec.Transport.Config.Custom.Protocol = "udp"

			err := customManager.CreateResources(ctx, mcpServer)
			Expect(err).NotTo(HaveOccurred())

			// Check deployment container port protocol
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Ports[0].Name).To(Equal(protocolUDP))
			Expect(container.Ports[0].Protocol).To(Equal(corev1.ProtocolUDP))

			// Check service protocol
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolUDP))
		})

		It("should create resources for SCTP custom transport", func() {
			mcpServer.Spec.Transport.Config.Custom.Protocol = "sctp"

			err := customManager.CreateResources(ctx, mcpServer)
			Expect(err).NotTo(HaveOccurred())

			// Check deployment container port protocol
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Ports[0].Name).To(Equal(protocolSCTP))
			Expect(container.Ports[0].Protocol).To(Equal(corev1.ProtocolSCTP))

			// Check service protocol
			service := &corev1.Service{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, service)
			Expect(err).NotTo(HaveOccurred())

			Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolSCTP))
		})
	})

	Describe("UpdateResources", func() {
		BeforeEach(func() {
			// Create initial resources
			err := customManager.CreateResources(ctx, mcpServer)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should update existing deployment", func() {
			// Change replica count
			mcpServer.Spec.Replicas = ptr(int32(3))

			err := customManager.UpdateResources(ctx, mcpServer)
			Expect(err).NotTo(HaveOccurred())

			// Check deployment was updated
			deployment := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, client.ObjectKey{
				Name:      mcpServer.Name,
				Namespace: mcpServer.Namespace,
			}, deployment)
			Expect(err).NotTo(HaveOccurred())

			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
		})
	})

	Describe("GetTransportType", func() {
		It("should return Custom transport type", func() {
			transportType := customManager.GetTransportType()
			Expect(transportType).To(Equal(mcpv1.MCPTransportCustom))
		})
	})

	Describe("RequiresService", func() {
		It("should return true for custom transport", func() {
			requiresService := customManager.RequiresService()
			Expect(requiresService).To(BeTrue())
		})
	})

	Describe("RequiresIngress", func() {
		It("should return false for custom transport by default", func() {
			requiresIngress := customManager.RequiresIngress()
			Expect(requiresIngress).To(BeFalse())
		})
	})

	Describe("Transport port handling", func() {
		It("should work with custom port", func() {
			port := GetTransportPort(mcpServer)
			Expect(port).To(Equal(int32(9000)))
		})

		It("should work with default port when not specified", func() {
			mcpServer.Spec.Transport.Config.Custom.Port = 0
			port := GetTransportPort(mcpServer)
			Expect(port).To(Equal(int32(8080))) // default port
		})
	})

	Describe("Protocol handling", func() {
		Context("when protocol is HTTP-like", func() {
			BeforeEach(func() {
				mcpServer.Spec.Transport.Config.Custom.Protocol = protocolHTTP
			})

			It("should configure for HTTP traffic", func() {
				err := customManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				service := &corev1.Service{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, service)
				Expect(err).NotTo(HaveOccurred())

				// Should use TCP protocol even for HTTP-like custom transports
				Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			})
		})

		Context("when protocol is HTTPS-like", func() {
			BeforeEach(func() {
				mcpServer.Spec.Transport.Config.Custom.Protocol = protocolHTTPS
			})

			It("should configure for HTTPS traffic", func() {
				err := customManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				service := &corev1.Service{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, service)
				Expect(err).NotTo(HaveOccurred())

				// Should use TCP protocol for HTTPS-like custom transports
				Expect(service.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))
			})
		})
	})
})
