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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

var _ = Describe("ManagerFactory", func() {
	var (
		factory       *ManagerFactory
		runtimeScheme *runtime.Scheme
	)

	BeforeEach(func() {
		// Create runtime scheme and add our types
		runtimeScheme = runtime.NewScheme()
		Expect(scheme.AddToScheme(runtimeScheme)).To(Succeed())
		Expect(mcpv1.AddToScheme(runtimeScheme)).To(Succeed())

		// Create fake client
		k8sClient := fake.NewClientBuilder().
			WithScheme(runtimeScheme).
			Build()

		// Create factory
		factory = NewManagerFactory(k8sClient, runtimeScheme)
	})

	Describe("GetManager", func() {
		It("should return HTTPResourceManager for HTTP transport", func() {
			manager, err := factory.GetManager(mcpv1.MCPTransportHTTP)
			Expect(err).NotTo(HaveOccurred())
			Expect(manager).NotTo(BeNil())

			httpManager, ok := manager.(*HTTPResourceManager)
			Expect(ok).To(BeTrue())
			Expect(httpManager.GetTransportType()).To(Equal(mcpv1.MCPTransportHTTP))
		})

		It("should return CustomResourceManager for Custom transport", func() {
			manager, err := factory.GetManager(mcpv1.MCPTransportCustom)
			Expect(err).NotTo(HaveOccurred())
			Expect(manager).NotTo(BeNil())

			customManager, ok := manager.(*CustomResourceManager)
			Expect(ok).To(BeTrue())
			Expect(customManager.GetTransportType()).To(Equal(mcpv1.MCPTransportCustom))
		})

		It("should return error for unsupported transport type", func() {
			_, err := factory.GetManager("unsupported")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unsupported transport type"))
		})
	})

	Describe("GetManagerForMCPServer", func() {
		It("should return HTTP manager for MCPServer with HTTP transport", func() {
			mcpServer := &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "default",
				},
				Spec: mcpv1.MCPServerSpec{
					Transport: &mcpv1.MCPServerTransport{
						Type: mcpv1.MCPTransportHTTP,
					},
				},
			}

			manager, err := factory.GetManagerForMCPServer(mcpServer)
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetTransportType()).To(Equal(mcpv1.MCPTransportHTTP))
		})

		It("should return Custom manager for MCPServer with Custom transport", func() {
			mcpServer := &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "default",
				},
				Spec: mcpv1.MCPServerSpec{
					Transport: &mcpv1.MCPServerTransport{
						Type: mcpv1.MCPTransportCustom,
					},
				},
			}

			manager, err := factory.GetManagerForMCPServer(mcpServer)
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetTransportType()).To(Equal(mcpv1.MCPTransportCustom))
		})

		It("should return HTTP manager for MCPServer with no transport specified", func() {
			mcpServer := &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-server",
					Namespace: "default",
				},
				Spec: mcpv1.MCPServerSpec{
					// No transport specified, should default to HTTP
				},
			}

			manager, err := factory.GetManagerForMCPServer(mcpServer)
			Expect(err).NotTo(HaveOccurred())
			Expect(manager.GetTransportType()).To(Equal(mcpv1.MCPTransportHTTP))
		})
	})

	Describe("GetDefaultTransportType", func() {
		It("should return HTTP as default transport type", func() {
			defaultType := GetDefaultTransportType()
			Expect(defaultType).To(Equal(mcpv1.MCPTransportHTTP))
		})
	})

	Describe("GetTransportPort", func() {
		It("should return correct port for HTTP transport", func() {
			mcpServer := &mcpv1.MCPServer{
				Spec: mcpv1.MCPServerSpec{
					Transport: &mcpv1.MCPServerTransport{
						Type: mcpv1.MCPTransportHTTP,
						Config: &mcpv1.MCPTransportConfigDetails{
							HTTP: &mcpv1.MCPHTTPTransportConfig{
								Port: 9090,
							},
						},
					},
				},
			}

			port := GetTransportPort(mcpServer)
			Expect(port).To(Equal(int32(9090)))
		})

		It("should return correct port for Custom transport", func() {
			mcpServer := &mcpv1.MCPServer{
				Spec: mcpv1.MCPServerSpec{
					Transport: &mcpv1.MCPServerTransport{
						Type: mcpv1.MCPTransportCustom,
						Config: &mcpv1.MCPTransportConfigDetails{
							Custom: &mcpv1.MCPCustomTransportConfig{
								Port: 9000,
							},
						},
					},
				},
			}

			port := GetTransportPort(mcpServer)
			Expect(port).To(Equal(int32(9000)))
		})

		It("should return default port when no transport specified", func() {
			mcpServer := &mcpv1.MCPServer{
				Spec: mcpv1.MCPServerSpec{
					// No transport specified
				},
			}

			port := GetTransportPort(mcpServer)
			Expect(port).To(Equal(int32(8080))) // Default HTTP port
		})
	})
})
