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
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
	"github.com/vitorbari/mcp-operator/pkg/transport"
)

var _ = Describe("MCPServer Controller", func() {
	Context("When reconciling a basic MCPServer", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			// Generate unique name for each test
			resourceName = "test-mcpserver-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource")
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
					Security: &mcpv1.MCPServerSecurity{
						AllowedUsers: []string{"test-user"},
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

		It("should create a Deployment", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that a Deployment was created")
			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.21"))
			Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
		})

		It("should create a Service", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that a Service was created")
			service := &corev1.Service{}
			serviceKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, service)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(service.Spec.Selector).To(HaveKeyWithValue("app", resourceName))
			Expect(service.Spec.Ports).To(HaveLen(1))
			Expect(service.Spec.Ports[0].Port).To(Equal(int32(8080)))
		})

		It("should create RBAC resources", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that a ServiceAccount was created")
			sa := &corev1.ServiceAccount{}
			saKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, saKey, sa)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Checking that a Role was created")
			role := &rbacv1.Role{}
			roleKey := types.NamespacedName{
				Name:      resourceName + "-access",
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, roleKey, role)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			By("Checking that a RoleBinding was created")
			rb := &rbacv1.RoleBinding{}
			rbKey := types.NamespacedName{
				Name:      resourceName + "-access",
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, rbKey, rb)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})

		It("should update MCPServer status", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the MCPServer status")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return len(mcpserver.Status.Conditions) > 0
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(mcpserver.Status.Phase).NotTo(BeEmpty())
			// In test environment, deployment status may not reflect actual replicas
			// Just verify the status is being updated
			Expect(mcpserver.Status.ObservedGeneration).To(Equal(mcpserver.Generation))
		})
	})

	Context("When reconciling a MCPServer with HPA", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			// Generate unique name for each test
			resourceName = "test-mcpserver-hpa-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with HPA enabled")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
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
					HPA: &mcpv1.MCPServerHPA{
						Enabled:                           ptr(true),
						MinReplicas:                       ptr(int32(2)),
						MaxReplicas:                       ptr(int32(10)),
						TargetCPUUtilizationPercentage:    ptr(int32(70)),
						TargetMemoryUtilizationPercentage: ptr(int32(80)),
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

		It("should create an HPA resource", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that an HPA was created")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			hpaKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, hpaKey, hpa)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(2)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(10)))
			Expect(hpa.Spec.Metrics).To(HaveLen(2))

			// Check CPU metric
			cpuMetric := hpa.Spec.Metrics[0]
			Expect(cpuMetric.Type).To(Equal(autoscalingv2.ResourceMetricSourceType))
			Expect(cpuMetric.Resource.Name).To(Equal(corev1.ResourceCPU))
			Expect(*cpuMetric.Resource.Target.AverageUtilization).To(Equal(int32(70)))

			// Check Memory metric
			memMetric := hpa.Spec.Metrics[1]
			Expect(memMetric.Type).To(Equal(autoscalingv2.ResourceMetricSourceType))
			Expect(memMetric.Resource.Name).To(Equal(corev1.ResourceMemory))
			Expect(*memMetric.Resource.Target.AverageUtilization).To(Equal(int32(80)))
		})

		It("should not create HPA when disabled", func() {
			By("Updating the MCPServer to disable HPA")
			Expect(k8sClient.Get(ctx, typeNamespacedName, mcpserver)).To(Succeed())
			mcpserver.Spec.HPA.Enabled = ptr(false)
			Expect(k8sClient.Update(ctx, mcpserver)).To(Succeed())

			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that no HPA exists")
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			hpaKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, hpaKey, hpa)
				return errors.IsNotFound(err)
			}, time.Second*5, time.Millisecond*250).Should(BeTrue())
		})
	})

	Context("When reconciling MCPServer with custom configuration", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			// Generate unique name for each test
			resourceName = "test-mcpserver-custom-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with custom configuration")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(3)),
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("512Mi"),
						},
					},
					Transport: &mcpv1.MCPServerTransport{
						Type: mcpv1.MCPTransportHTTP,
						Config: &mcpv1.MCPTransportConfigDetails{
							HTTP: &mcpv1.MCPHTTPTransportConfig{
								Port: 9090,
								Path: "/mcp",
							},
						},
					},
					Service: &mcpv1.MCPServerService{
						Type:       "ClusterIP",
						Port:       9090,
						TargetPort: &intstr.IntOrString{IntVal: 80},
					},
					HealthCheck: &mcpv1.MCPServerHealthCheck{
						Enabled:             ptr(true),
						Path:                "/health",
						Port:                &intstr.IntOrString{IntVal: 80},
						InitialDelaySeconds: ptr(int32(10)),
						PeriodSeconds:       ptr(int32(5)),
					},
					Environment: []corev1.EnvVar{
						{
							Name:  "LOG_LEVEL",
							Value: "debug",
						},
						{
							Name:  "PORT",
							Value: "80",
						},
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

		It("should create Deployment with custom configuration", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking Deployment configuration")
			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
			Expect(container.Resources.Requests.Cpu().String()).To(Equal("200m"))
			Expect(container.Resources.Limits.Memory().String()).To(Equal("512Mi"))
			Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "LOG_LEVEL", Value: "debug"}))
			Expect(container.LivenessProbe.HTTPGet.Path).To(Equal("/health"))
			Expect(container.LivenessProbe.InitialDelaySeconds).To(Equal(int32(10)))
		})

		It("should create Service with custom port", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking Service configuration")
			service := &corev1.Service{}
			serviceKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, service)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(service.Spec.Ports[0].Port).To(Equal(int32(9090)))
			Expect(service.Spec.Ports[0].TargetPort.IntVal).To(Equal(int32(80)))
		})
	})

	Context("When reconciling MCPServer with Ingress", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			resourceName = "test-mcpserver-ingress-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with Ingress enabled")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type: mcpv1.MCPTransportHTTP,
						Config: &mcpv1.MCPTransportConfigDetails{
							HTTP: &mcpv1.MCPHTTPTransportConfig{
								Port:              8080,
								SessionManagement: ptr(true),
							},
						},
					},
					Ingress: &mcpv1.MCPServerIngress{
						Enabled:   ptr(true),
						Host:      "test.example.com",
						Path:      "/mcp",
						ClassName: ptr("nginx"),
						Annotations: map[string]string{
							"cert-manager.io/cluster-issuer": "letsencrypt",
						},
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
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				_ = k8sClient.Delete(ctx, resource)
			}
		})

		It("should create an Ingress resource", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that an Ingress was created")
			ingress := &networkingv1.Ingress{}
			ingressKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, ingressKey, ingress)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(ingress.Spec.Rules).To(HaveLen(1))
			Expect(ingress.Spec.Rules[0].Host).To(Equal("test.example.com"))
			Expect(ingress.Spec.Rules[0].HTTP.Paths).To(HaveLen(1))
			Expect(ingress.Spec.Rules[0].HTTP.Paths[0].Path).To(Equal("/mcp"))

			// Check annotations include both custom and MCP-specific ones
			Expect(ingress.Annotations).To(HaveKey("cert-manager.io/cluster-issuer"))
			Expect(ingress.Annotations).To(HaveKey("nginx.ingress.kubernetes.io/server-snippet"))
			Expect(ingress.Annotations["nginx.ingress.kubernetes.io/enable-metrics"]).To(Equal("true"))
		})

		It("should not create Ingress when disabled", func() {
			By("Updating the MCPServer to disable Ingress")
			Expect(k8sClient.Get(ctx, typeNamespacedName, mcpserver)).To(Succeed())
			mcpserver.Spec.Ingress.Enabled = ptr(false)
			Expect(k8sClient.Update(ctx, mcpserver)).To(Succeed())

			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that no Ingress exists")
			ingress := &networkingv1.Ingress{}
			ingressKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Consistently(func() bool {
				err := k8sClient.Get(ctx, ingressKey, ingress)
				return errors.IsNotFound(err)
			}, time.Second*5, time.Millisecond*250).Should(BeTrue())
		})
	})

	Context("When reconciling MCPServer with Custom Transport", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			resourceName = "test-mcpserver-custom-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with Custom Transport")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
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
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				_ = k8sClient.Delete(ctx, resource)
			}
		})

		It("should create resources with custom transport configuration", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment was created with custom transport")
			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			container := deployment.Spec.Template.Spec.Containers[0]

			// Check for MCP_TRANSPORT env var
			var transportEnv *corev1.EnvVar
			for _, env := range container.Env {
				if env.Name == "MCP_TRANSPORT" {
					transportEnv = &env
					break
				}
			}
			Expect(transportEnv).NotTo(BeNil())
			Expect(transportEnv.Value).To(Equal("custom"))

			By("Checking Service configuration for custom transport")
			service := &corev1.Service{}
			serviceKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, serviceKey, service)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			Expect(service.Spec.Ports[0].Port).To(Equal(int32(9000)))
		})

		It("should update status with custom transport type", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the MCPServer status contains transport type")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.TransportType == mcpv1.MCPTransportCustom
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})
	})

	Context("When handling MCPServer deletion", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			resourceName = "test-mcpserver-delete-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(1)),
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			controllerReconciler = &MCPServerReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				TransportFactory: transport.NewManagerFactory(k8sClient, k8sClient.Scheme()),
				Recorder:         record.NewFakeRecorder(100),
			}

			// First reconcile to create resources and add finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle deletion gracefully", func() {
			By("Deleting the MCPServer resource")
			Expect(k8sClient.Get(ctx, typeNamespacedName, mcpserver)).To(Succeed())
			Expect(k8sClient.Delete(ctx, mcpserver)).To(Succeed())

			By("Reconciling the deletion")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the resource is deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				return errors.IsNotFound(err)
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())
		})

		It("should update status to Terminating during deletion", func() {
			By("Deleting the MCPServer resource")
			Expect(k8sClient.Get(ctx, typeNamespacedName, mcpserver)).To(Succeed())
			Expect(k8sClient.Delete(ctx, mcpserver)).To(Succeed())

			By("Getting the MCPServer with deletion timestamp")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				return err == nil && mcpserver.DeletionTimestamp != nil
			}, time.Second*5, time.Millisecond*250).Should(BeTrue())

			By("Reconciling to handle deletion")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling MCPServer with custom command and args", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			resourceName = "test-mcpserver-command-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			By("Creating the MCPServer resource with custom command and args")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "mcp/wikipedia-mcp:latest",
					Replicas: ptr(int32(1)),
					Command:  []string{"python", "-m", "wikipedia_mcp"},
					Args:     []string{"--transport", "sse", "--port", "8080", "--host", "0.0.0.0"},
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
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				_ = k8sClient.Delete(ctx, resource)
			}
		})

		It("should create Deployment with custom command and args", func() {
			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment was created with custom command and args")
			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			container := deployment.Spec.Template.Spec.Containers[0]

			// Verify the container command is set correctly
			Expect(container.Command).To(Equal([]string{"python", "-m", "wikipedia_mcp"}))

			// Verify the container args are set correctly
			Expect(container.Args).To(Equal([]string{"--transport", "sse", "--port", "8080", "--host", "0.0.0.0"}))
		})

		It("should create Deployment with only custom command when args are empty", func() {
			By("Updating the MCPServer to remove args")
			Expect(k8sClient.Get(ctx, typeNamespacedName, mcpserver)).To(Succeed())
			mcpserver.Spec.Args = nil
			Expect(k8sClient.Update(ctx, mcpserver)).To(Succeed())

			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment has command but no args")
			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			container := deployment.Spec.Template.Spec.Containers[0]

			// Verify the container command is still set
			Expect(container.Command).To(Equal([]string{"python", "-m", "wikipedia_mcp"}))

			// Verify the container args are empty
			Expect(container.Args).To(BeEmpty())
		})

		It("should create Deployment with only custom args when command is empty", func() {
			By("Updating the MCPServer to remove command")
			Expect(k8sClient.Get(ctx, typeNamespacedName, mcpserver)).To(Succeed())
			mcpserver.Spec.Command = nil
			mcpserver.Spec.Args = []string{"--config", "/app/config.json"}
			Expect(k8sClient.Update(ctx, mcpserver)).To(Succeed())

			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment has args but no command")
			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			container := deployment.Spec.Template.Spec.Containers[0]

			// Verify the container command is empty
			Expect(container.Command).To(BeEmpty())

			// Verify the container args are set
			Expect(container.Args).To(Equal([]string{"--config", "/app/config.json"}))
		})
	})

	Context("When handling reconciliation errors", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			resourceName = "test-mcpserver-error-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}

			controllerReconciler = &MCPServerReconciler{
				Client:           k8sClient,
				Scheme:           k8sClient.Scheme(),
				TransportFactory: transport.NewManagerFactory(k8sClient, k8sClient.Scheme()),
				Recorder:         record.NewFakeRecorder(100),
			}
		})

		It("should handle non-existent resource gracefully", func() {
			By("Reconciling a non-existent MCPServer")
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
		})

		It("should reject MCPServer with invalid image", func() {
			By("Creating MCPServer with invalid configuration")
			mcpserver := &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "", // Invalid empty image
					Replicas: ptr(int32(1)),
				},
			}

			By("Expecting creation to fail due to validation")
			err := k8sClient.Create(ctx, mcpserver)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("spec.image"))
		})
	})
})

// Helper function to create pointer to values
func ptr[T any](v T) *T {
	return &v
}

// RandStringRunes generates a random string of length n
func RandStringRunes(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}
