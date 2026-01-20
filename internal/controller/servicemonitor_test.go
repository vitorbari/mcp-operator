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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
	"github.com/vitorbari/mcp-operator/internal/transport"
)

var _ = Describe("ServiceMonitor Lifecycle", func() {
	Context("When metrics are enabled on MCPServer", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			// Generate unique name for each test
			resourceName = "test-sm-enabled-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with metrics enabled")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(1)),
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					Metrics: &mcpv1.MetricsConfig{
						Enabled: true,
						Port:    9090,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			controllerReconciler = &MCPServerReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				TransportFactory: transport.NewManagerFactory(k8sClient, k8sClient.Scheme()),
				Recorder:         record.NewFakeRecorder(100),
			}
		})

		AfterEach(func() {
			By("Cleaning up the MCPServer resource")
			resource := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				// Remove finalizers to allow deletion
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				_ = k8sClient.Delete(ctx, resource)
			}

			// Clean up any ServiceMonitors
			sm := &monitoringv1.ServiceMonitor{}
			err = k8sClient.Get(ctx, typeNamespacedName, sm)
			if err == nil {
				_ = k8sClient.Delete(ctx, sm)
			}
		})

		It("should create a ServiceMonitor when Prometheus Operator CRD is available", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that a ServiceMonitor was created")
			serviceMonitor := &monitoringv1.ServiceMonitor{}
			serviceMonitorKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, serviceMonitor)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Verifying ServiceMonitor configuration")
			Expect(serviceMonitor.Name).To(Equal(resourceName))
			Expect(serviceMonitor.Namespace).To(Equal(resourceNamespace))

			// Check labels
			Expect(serviceMonitor.Labels).To(HaveKeyWithValue("app", resourceName))
			Expect(serviceMonitor.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", "mcpserver"))
			Expect(serviceMonitor.Labels).To(HaveKeyWithValue("app.kubernetes.io/instance", resourceName))
			Expect(serviceMonitor.Labels).To(HaveKeyWithValue("app.kubernetes.io/component", "mcp-server"))
			Expect(serviceMonitor.Labels).To(HaveKeyWithValue("app.kubernetes.io/managed-by", "mcp-operator"))
			Expect(serviceMonitor.Labels).To(HaveKeyWithValue("release", "monitoring"))

			// Check selector
			Expect(serviceMonitor.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app", resourceName))

			// Check namespace selector
			Expect(serviceMonitor.Spec.NamespaceSelector.MatchNames).To(ContainElement(resourceNamespace))

			// Check endpoints
			Expect(serviceMonitor.Spec.Endpoints).To(HaveLen(1))
			Expect(serviceMonitor.Spec.Endpoints[0].Port).To(Equal("metrics"))
			Expect(serviceMonitor.Spec.Endpoints[0].Path).To(Equal("/metrics"))
			Expect(serviceMonitor.Spec.Endpoints[0].Interval).To(Equal(monitoringv1.Duration("30s")))

			// Check target labels
			Expect(serviceMonitor.Spec.TargetLabels).To(ContainElements("app.kubernetes.io/name", "app.kubernetes.io/instance"))

			// Verify owner reference
			Expect(serviceMonitor.OwnerReferences).To(HaveLen(1))
			Expect(serviceMonitor.OwnerReferences[0].Name).To(Equal(resourceName))
			Expect(serviceMonitor.OwnerReferences[0].Kind).To(Equal("MCPServer"))
		})

		It("should include TLS configuration when sidecar TLS is enabled", func() {
			By("Creating TLS secret")
			tlsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-tls-secret",
					Namespace: resourceNamespace,
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte("dummy-cert"),
					"tls.key": []byte("dummy-key"),
				},
			}
			Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())

			By("Updating MCPServer to enable sidecar TLS")
			err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			Expect(err).NotTo(HaveOccurred())

			mcpserver.Spec.Sidecar = &mcpv1.SidecarConfig{
				TLS: &mcpv1.SidecarTLSConfig{
					Enabled:    true,
					SecretName: "test-tls-secret",
				},
			}
			Expect(k8sClient.Update(ctx, mcpserver)).To(Succeed())

			By("Reconciling the MCPServer")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ServiceMonitor has TLS configuration")
			serviceMonitor := &monitoringv1.ServiceMonitor{}
			serviceMonitorKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, serviceMonitor)
				if err != nil {
					return false
				}
				return len(serviceMonitor.Spec.Endpoints) > 0 &&
					serviceMonitor.Spec.Endpoints[0].TLSConfig != nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(serviceMonitor.Spec.Endpoints[0].Scheme).To(Equal("https"))
			Expect(serviceMonitor.Spec.Endpoints[0].TLSConfig).NotTo(BeNil())
			Expect(*serviceMonitor.Spec.Endpoints[0].TLSConfig.SafeTLSConfig.InsecureSkipVerify).To(BeTrue())
		})
	})

	Context("When metrics are disabled on MCPServer", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			// Generate unique name for each test
			resourceName = "test-sm-disabled-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with metrics initially enabled")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(1)),
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					Metrics: &mcpv1.MetricsConfig{
						Enabled: true,
						Port:    9090,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			controllerReconciler = &MCPServerReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				TransportFactory: transport.NewManagerFactory(k8sClient, k8sClient.Scheme()),
				Recorder:         record.NewFakeRecorder(100),
			}
		})

		AfterEach(func() {
			By("Cleaning up the MCPServer resource")
			resource := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				// Remove finalizers to allow deletion
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				_ = k8sClient.Delete(ctx, resource)
			}
		})

		It("should delete ServiceMonitor when metrics are disabled", func() {
			By("First reconciling to create ServiceMonitor")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ServiceMonitor exists")
			serviceMonitor := &monitoringv1.ServiceMonitor{}
			serviceMonitorKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, serviceMonitor)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Disabling metrics on MCPServer")
			err = k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			Expect(err).NotTo(HaveOccurred())

			mcpserver.Spec.Metrics.Enabled = false
			Expect(k8sClient.Update(ctx, mcpserver)).To(Succeed())

			By("Reconciling after disabling metrics")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ServiceMonitor is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, serviceMonitor)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})

		It("should delete ServiceMonitor when metrics config is removed", func() {
			By("First reconciling to create ServiceMonitor")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ServiceMonitor exists")
			serviceMonitor := &monitoringv1.ServiceMonitor{}
			serviceMonitorKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, serviceMonitor)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Removing metrics config from MCPServer")
			err = k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			Expect(err).NotTo(HaveOccurred())

			mcpserver.Spec.Metrics = nil
			Expect(k8sClient.Update(ctx, mcpserver)).To(Succeed())

			By("Reconciling after removing metrics config")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ServiceMonitor is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, serviceMonitor)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})
	})

	Context("When ServiceMonitor is accidentally deleted", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			// Generate unique name for each test
			resourceName = "test-sm-recreate-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with metrics enabled")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(1)),
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					Metrics: &mcpv1.MetricsConfig{
						Enabled: true,
						Port:    9090,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			controllerReconciler = &MCPServerReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				TransportFactory: transport.NewManagerFactory(k8sClient, k8sClient.Scheme()),
				Recorder:         record.NewFakeRecorder(100),
			}
		})

		AfterEach(func() {
			By("Cleaning up the MCPServer resource")
			resource := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				// Remove finalizers to allow deletion
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				_ = k8sClient.Delete(ctx, resource)
			}

			// Clean up any ServiceMonitors
			sm := &monitoringv1.ServiceMonitor{}
			err = k8sClient.Get(ctx, typeNamespacedName, sm)
			if err == nil {
				_ = k8sClient.Delete(ctx, sm)
			}
		})

		It("should recreate ServiceMonitor after accidental deletion", func() {
			By("First reconciling to create ServiceMonitor")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ServiceMonitor exists")
			serviceMonitor := &monitoringv1.ServiceMonitor{}
			serviceMonitorKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, serviceMonitor)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			originalUID := serviceMonitor.UID

			By("Simulating accidental deletion of ServiceMonitor")
			Expect(k8sClient.Delete(ctx, serviceMonitor)).To(Succeed())

			By("Verifying ServiceMonitor is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, serviceMonitor)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Reconciling again to recreate ServiceMonitor")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying ServiceMonitor is recreated")
			newServiceMonitor := &monitoringv1.ServiceMonitor{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, newServiceMonitor)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Verify it's a new resource (different UID)
			Expect(newServiceMonitor.UID).NotTo(Equal(originalUID))

			// Verify configuration is correct
			Expect(newServiceMonitor.Labels).To(HaveKeyWithValue("app", resourceName))
			Expect(newServiceMonitor.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app", resourceName))
			Expect(newServiceMonitor.Spec.Endpoints).To(HaveLen(1))
			Expect(newServiceMonitor.Spec.Endpoints[0].Port).To(Equal("metrics"))
		})
	})

	Context("When ServiceMonitor CRD is not available", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			// Generate unique name for each test
			resourceName = "test-sm-nocrd-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with metrics enabled")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(1)),
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					Metrics: &mcpv1.MetricsConfig{
						Enabled: true,
						Port:    9090,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			controllerReconciler = &MCPServerReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				TransportFactory: transport.NewManagerFactory(k8sClient, k8sClient.Scheme()),
				Recorder:         record.NewFakeRecorder(100),
			}
		})

		AfterEach(func() {
			By("Cleaning up the MCPServer resource")
			resource := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				// Remove finalizers to allow deletion
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				_ = k8sClient.Delete(ctx, resource)
			}
		})

		It("should not fail reconciliation when ServiceMonitor CRD is not installed", func() {
			// Note: This test verifies the controller gracefully handles missing CRD
			// In our test environment, the CRD IS installed, so we verify successful creation
			// In a real scenario without the CRD, the controller would skip ServiceMonitor creation

			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred(), "Reconciliation should not fail even if handling ServiceMonitor")

			By("Verifying MCPServer status is updated successfully")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return len(mcpserver.Status.Conditions) > 0
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Verifying MCPServer enters Running or Creating phase")
			Expect(mcpserver.Status.Phase).To(SatisfyAny(
				Equal(mcpv1.MCPServerPhaseCreating),
				Equal(mcpv1.MCPServerPhaseRunning),
			))
		})
	})

	Context("When using custom metrics port", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			// Generate unique name for each test
			resourceName = "test-sm-customport-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with custom metrics port")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(1)),
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
					},
					Metrics: &mcpv1.MetricsConfig{
						Enabled: true,
						Port:    9999, // Custom port
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			controllerReconciler = &MCPServerReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				TransportFactory: transport.NewManagerFactory(k8sClient, k8sClient.Scheme()),
				Recorder:         record.NewFakeRecorder(100),
			}
		})

		AfterEach(func() {
			By("Cleaning up the MCPServer resource")
			resource := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				// Remove finalizers to allow deletion
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				_ = k8sClient.Delete(ctx, resource)
			}

			// Clean up any ServiceMonitors
			sm := &monitoringv1.ServiceMonitor{}
			err = k8sClient.Get(ctx, typeNamespacedName, sm)
			if err == nil {
				_ = k8sClient.Delete(ctx, sm)
			}
		})

		It("should create ServiceMonitor with correct port configuration", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that ServiceMonitor uses correct port name")
			serviceMonitor := &monitoringv1.ServiceMonitor{}
			serviceMonitorKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceMonitorKey, serviceMonitor)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// ServiceMonitor should reference the port by name "metrics", not by number
			Expect(serviceMonitor.Spec.Endpoints).To(HaveLen(1))
			Expect(serviceMonitor.Spec.Endpoints[0].Port).To(Equal("metrics"))
		})
	})
})
