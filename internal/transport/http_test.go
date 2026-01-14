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

var _ = Describe("SSE-Aware Reconciliation", func() {
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
				Name:      "test-sse-mcpserver",
				Namespace: "default",
			},
			Spec: mcpv1.MCPServerSpec{
				Image:    "nginx:1.21",
				Replicas: ptr(int32(2)),
				Transport: &mcpv1.MCPServerTransport{
					Type: mcpv1.MCPTransportHTTP,
					Config: &mcpv1.MCPTransportConfigDetails{
						HTTP: &mcpv1.MCPHTTPTransportConfig{
							Port: 8080,
							Path: "/sse",
						},
					},
				},
			},
		}
	})

	Describe("isSSEActive", func() {
		It("should return false when protocol is streamable-http", func() {
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolStreamableHTTP
			Expect(httpManager.isSSEActive(mcpServer)).To(BeFalse())
		})

		It("should return true when protocol is explicitly sse", func() {
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolSSE
			Expect(httpManager.isSSEActive(mcpServer)).To(BeTrue())
		})

		It("should return true when protocol is auto and SSE is detected in status", func() {
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolAuto
			mcpServer.Status.ResolvedTransport = &mcpv1.ResolvedTransportStatus{
				Protocol: mcpv1.MCPProtocolSSE,
			}
			Expect(httpManager.isSSEActive(mcpServer)).To(BeTrue())
		})

		It("should return false when protocol is auto and no detection yet", func() {
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolAuto
			Expect(httpManager.isSSEActive(mcpServer)).To(BeFalse())
		})

		It("should return true when protocol is auto and validation detected SSE", func() {
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolAuto
			mcpServer.Status.Validation = &mcpv1.ValidationStatus{
				Protocol: "sse",
			}
			Expect(httpManager.isSSEActive(mcpServer)).To(BeTrue())
		})
	})

	Describe("isExplicitSSE", func() {
		It("should return true only for explicit SSE configuration", func() {
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolSSE
			Expect(httpManager.isExplicitSSE(mcpServer)).To(BeTrue())

			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolAuto
			Expect(httpManager.isExplicitSSE(mcpServer)).To(BeFalse())
		})
	})

	Describe("SSE Deployment Settings", func() {
		Context("when SSE is explicitly configured", func() {
			BeforeEach(func() {
				mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolSSE
			})

			It("should apply rolling update strategy with maxUnavailable=0", func() {
				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, deployment)
				Expect(err).NotTo(HaveOccurred())

				Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RollingUpdateDeploymentStrategyType))
				Expect(deployment.Spec.Strategy.RollingUpdate).NotTo(BeNil())
				Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue()).To(Equal(0))
			})

			It("should apply SSE-specific pod annotations", func() {
				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, deployment)
				Expect(err).NotTo(HaveOccurred())

				Expect(deployment.Spec.Template.Annotations).To(HaveKeyWithValue("mcp.transport.protocol", "sse"))
				Expect(deployment.Spec.Template.Annotations).To(HaveKeyWithValue("mcp.transport.streaming", "true"))
			})

			It("should apply default termination grace period", func() {
				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, deployment)
				Expect(err).NotTo(HaveOccurred())

				Expect(deployment.Spec.Template.Spec.TerminationGracePeriodSeconds).NotTo(BeNil())
				Expect(*deployment.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(60)))
			})

			It("should apply custom termination grace period when configured", func() {
				customGracePeriod := int64(120)
				mcpServer.Spec.Transport.Config.HTTP.SSE = &mcpv1.SSEConfig{
					TerminationGracePeriodSeconds: &customGracePeriod,
				}

				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, deployment)
				Expect(err).NotTo(HaveOccurred())

				Expect(deployment.Spec.Template.Spec.TerminationGracePeriodSeconds).NotTo(BeNil())
				Expect(*deployment.Spec.Template.Spec.TerminationGracePeriodSeconds).To(Equal(int64(120)))
			})
		})

		Context("when SSE is auto-detected", func() {
			BeforeEach(func() {
				mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolAuto
				mcpServer.Status.ResolvedTransport = &mcpv1.ResolvedTransportStatus{
					Protocol:         mcpv1.MCPProtocolSSE,
					SSEConfigApplied: true,
				}
			})

			It("should apply SSE settings after detection", func() {
				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, deployment)
				Expect(err).NotTo(HaveOccurred())

				// SSE settings should be applied
				Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RollingUpdateDeploymentStrategyType))
				Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.IntValue()).To(Equal(0))
			})
		})

		Context("when protocol is streamable-http", func() {
			BeforeEach(func() {
				mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolStreamableHTTP
			})

			It("should NOT apply SSE-specific settings", func() {
				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				deployment := &appsv1.Deployment{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, deployment)
				Expect(err).NotTo(HaveOccurred())

				// SSE annotations should not be present
				Expect(deployment.Spec.Template.Annotations).NotTo(HaveKey("mcp.transport.protocol"))
				Expect(deployment.Spec.Template.Annotations).NotTo(HaveKey("mcp.transport.streaming"))

				// Termination grace period should not be set (default behavior)
				Expect(deployment.Spec.Template.Spec.TerminationGracePeriodSeconds).To(BeNil())
			})
		})
	})

	Describe("SSE Service Settings", func() {
		Context("when SSE session affinity is enabled", func() {
			BeforeEach(func() {
				mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolSSE
				mcpServer.Spec.Transport.Config.HTTP.SSE = &mcpv1.SSEConfig{
					EnableSessionAffinity: ptr(true),
				}
			})

			It("should enable ClientIP session affinity on the service", func() {
				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				service := &corev1.Service{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, service)
				Expect(err).NotTo(HaveOccurred())

				Expect(service.Spec.SessionAffinity).To(Equal(corev1.ServiceAffinityClientIP))
				Expect(service.Spec.SessionAffinityConfig).NotTo(BeNil())
				Expect(service.Spec.SessionAffinityConfig.ClientIP).NotTo(BeNil())
			})

			It("should add SSE annotations to the service", func() {
				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				service := &corev1.Service{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, service)
				Expect(err).NotTo(HaveOccurred())

				Expect(service.Annotations).To(HaveKeyWithValue("mcp.transport.protocol", "sse"))
				Expect(service.Annotations).To(HaveKeyWithValue("mcp.transport.streaming", "true"))
			})
		})

		Context("when SSE is active but session affinity is NOT enabled", func() {
			BeforeEach(func() {
				mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolSSE
				// No SSE config or session affinity not enabled
			})

			It("should NOT enable session affinity by default", func() {
				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				service := &corev1.Service{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, service)
				Expect(err).NotTo(HaveOccurred())

				// Session affinity should NOT be set (safe default) - empty string or None
				Expect(service.Spec.SessionAffinity).NotTo(Equal(corev1.ServiceAffinityClientIP))
			})
		})

		Context("when auto-detect mode and SSE detected but no explicit affinity opt-in", func() {
			BeforeEach(func() {
				mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolAuto
				mcpServer.Status.ResolvedTransport = &mcpv1.ResolvedTransportStatus{
					Protocol:         mcpv1.MCPProtocolSSE,
					SSEConfigApplied: true,
				}
				// No SSE config - user didn't opt in
			})

			It("should NOT enable session affinity implicitly", func() {
				err := httpManager.CreateResources(ctx, mcpServer)
				Expect(err).NotTo(HaveOccurred())

				service := &corev1.Service{}
				err = k8sClient.Get(ctx, client.ObjectKey{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				}, service)
				Expect(err).NotTo(HaveOccurred())

				// Session affinity should NOT be set in auto-detect mode without explicit opt-in
				Expect(service.Spec.SessionAffinity).NotTo(Equal(corev1.ServiceAffinityClientIP))
			})
		})
	})

	Describe("SSE Configuration Helpers", func() {
		It("should return correct termination grace period", func() {
			// No SSE config - should return nil for non-SSE
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolStreamableHTTP
			gracePeriod := httpManager.getSSETerminationGracePeriod(mcpServer)
			Expect(gracePeriod).To(BeNil())

			// SSE active but no custom config - should return default 60s
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolSSE
			gracePeriod = httpManager.getSSETerminationGracePeriod(mcpServer)
			Expect(gracePeriod).NotTo(BeNil())
			Expect(*gracePeriod).To(Equal(int64(60)))

			// SSE with custom config
			customPeriod := int64(90)
			mcpServer.Spec.Transport.Config.HTTP.SSE = &mcpv1.SSEConfig{
				TerminationGracePeriodSeconds: &customPeriod,
			}
			gracePeriod = httpManager.getSSETerminationGracePeriod(mcpServer)
			Expect(gracePeriod).NotTo(BeNil())
			Expect(*gracePeriod).To(Equal(int64(90)))
		})

		It("should correctly determine session affinity requirement", func() {
			// Non-SSE should not require affinity
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolStreamableHTTP
			Expect(httpManager.shouldEnableSSESessionAffinity(mcpServer)).To(BeFalse())

			// SSE without config should not require affinity
			mcpServer.Spec.Transport.Protocol = mcpv1.MCPProtocolSSE
			Expect(httpManager.shouldEnableSSESessionAffinity(mcpServer)).To(BeFalse())

			// SSE with explicit opt-in should require affinity
			mcpServer.Spec.Transport.Config.HTTP.SSE = &mcpv1.SSEConfig{
				EnableSessionAffinity: ptr(true),
			}
			Expect(httpManager.shouldEnableSSESessionAffinity(mcpServer)).To(BeTrue())

			// SSE with explicit opt-out should not require affinity
			mcpServer.Spec.Transport.Config.HTTP.SSE.EnableSessionAffinity = ptr(false)
			Expect(httpManager.shouldEnableSSESessionAffinity(mcpServer)).To(BeFalse())
		})
	})
})
