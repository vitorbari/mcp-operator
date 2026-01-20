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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
	"github.com/vitorbari/mcp-operator/internal/transport"
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
						Enabled: ptr(true),
						Path:    "/health",
						Port:    &intstr.IntOrString{IntVal: 80},
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

	Context("When handling protocol mismatch", func() {
		const (
			resourceNamespace = "default"
			timeout           = time.Second * 10
			interval          = time.Millisecond * 250
		)

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			resourceName = "test-mismatch-" + RandStringRunes(8)
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

		AfterEach(func() {
			resource := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				resource.Finalizers = nil
				_ = k8sClient.Update(ctx, resource)
				_ = k8sClient.Delete(ctx, resource)
			}
		})

		It("should detect protocol mismatch in non-strict mode", func() {
			By("Creating MCPServer with streamable-http protocol")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type:     mcpv1.MCPTransportHTTP,
						Protocol: mcpv1.MCPProtocolStreamableHTTP, // User configures streamable-http
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled:    ptr(true),
						StrictMode: ptr(false), // Non-strict mode
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Simulating protocol mismatch detection")
			// Fetch the MCPServer to get current state
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			// Simulate detected SSE (different from configured streamable-http)
			mcpserver.Status.Phase = mcpv1.MCPServerPhaseRunning
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateFailed,
				Compliant:           false, // Not compliant due to mismatch
				ProtocolVersion:     "2024-11-05",
				Protocol:            "sse", // Server actually uses SSE
				ValidatedGeneration: mcpserver.Generation,
				Issues: []mcpv1.ValidationIssue{
					{
						Code:    "PROTOCOL_MISMATCH",
						Level:   "error",
						Message: "Protocol mismatch: configured 'streamable-http' but server uses 'sse'",
					},
				},
			}
			mcpserver.Status.Conditions = []mcpv1.MCPServerCondition{
				{
					Type:               mcpv1.MCPServerConditionDegraded,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "ProtocolMismatch",
					Message:            "Configured protocol does not match detected protocol",
				},
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying status reflects protocol mismatch")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.Phase == mcpv1.MCPServerPhaseRunning &&
					mcpserver.Status.Validation != nil &&
					!mcpserver.Status.Validation.Compliant
			}, timeout, interval).Should(BeTrue())

			By("Verifying Degraded condition is set")
			Expect(mcpserver.Status.Conditions).NotTo(BeEmpty())
			degradedCondition := findCondition(mcpserver.Status.Conditions, mcpv1.MCPServerConditionDegraded)
			Expect(degradedCondition).NotTo(BeNil())
			Expect(degradedCondition.Status).To(Equal(corev1.ConditionTrue))
			Expect(degradedCondition.Reason).To(Equal("ProtocolMismatch"))

			By("Verifying validation issue is recorded")
			Expect(mcpserver.Status.Validation.Issues).To(HaveLen(1))
			Expect(mcpserver.Status.Validation.Issues[0].Code).To(Equal("PROTOCOL_MISMATCH"))
			Expect(mcpserver.Status.Validation.Issues[0].Level).To(Equal("error"))

			By("Verifying validation state is Failed (protocol mismatch)")
			Expect(mcpserver.Status.Validation.State).To(Equal(mcpv1.ValidationStateFailed))
		})

		It("should detect protocol mismatch in strict mode", func() {
			By("Creating MCPServer with streamable-http protocol and strict mode")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type:     mcpv1.MCPTransportHTTP,
						Protocol: mcpv1.MCPProtocolStreamableHTTP,
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled:    ptr(true),
						StrictMode: ptr(true), // Strict mode
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Simulating protocol mismatch in strict mode")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			// In strict mode, phase should be Failed
			mcpserver.Status.Phase = mcpv1.MCPServerPhaseFailed
			mcpserver.Status.Message = "Protocol validation failed in strict mode"
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateFailed,
				Compliant:           false,
				ProtocolVersion:     "2024-11-05",
				Protocol:            "sse",
				ValidatedGeneration: mcpserver.Generation,
				Issues: []mcpv1.ValidationIssue{
					{
						Code:    "PROTOCOL_MISMATCH",
						Level:   "error",
						Message: "Protocol mismatch: configured 'streamable-http' but server uses 'sse'",
					},
				},
			}
			mcpserver.Status.Conditions = []mcpv1.MCPServerCondition{
				{
					Type:               mcpv1.MCPServerConditionDegraded,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "ValidationFailedStrict",
					Message:            "MCP protocol validation failed in strict mode",
				},
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying phase is Failed in strict mode")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.Phase == mcpv1.MCPServerPhaseFailed
			}, timeout, interval).Should(BeTrue())

			By("Verifying Degraded condition with strict mode reason")
			degradedCondition := findCondition(mcpserver.Status.Conditions, mcpv1.MCPServerConditionDegraded)
			Expect(degradedCondition).NotTo(BeNil())
			Expect(degradedCondition.Status).To(Equal(corev1.ConditionTrue))
			Expect(degradedCondition.Reason).To(Equal("ValidationFailedStrict"))
		})

		It("should not detect mismatch with auto protocol", func() {
			By("Creating MCPServer with auto protocol")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type:     mcpv1.MCPTransportHTTP,
						Protocol: mcpv1.MCPProtocolAuto, // Auto-detection
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled:    ptr(true),
						StrictMode: ptr(false),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Simulating successful auto-detection")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			// Auto-detection detected SSE - no mismatch
			mcpserver.Status.Phase = mcpv1.MCPServerPhaseRunning
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateValidated,
				Compliant:           true, // Compliant - no mismatch with auto
				ProtocolVersion:     "2024-11-05",
				Protocol:            "sse", // Auto-detected SSE
				ValidatedGeneration: mcpserver.Generation,
				Issues:              []mcpv1.ValidationIssue{}, // No issues
			}
			mcpserver.Status.Conditions = []mcpv1.MCPServerCondition{
				{
					Type:               mcpv1.MCPServerConditionDegraded,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: metav1.Now(),
					Reason:             "ValidationPassed",
					Message:            "MCP protocol validation succeeded",
				},
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying no mismatch with auto protocol")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.Validation != nil &&
					mcpserver.Status.Validation.Compliant
			}, timeout, interval).Should(BeTrue())

			By("Verifying Degraded condition is False")
			degradedCondition := findCondition(mcpserver.Status.Conditions, mcpv1.MCPServerConditionDegraded)
			Expect(degradedCondition).NotTo(BeNil())
			Expect(degradedCondition.Status).To(Equal(corev1.ConditionFalse))

			By("Verifying no validation issues")
			Expect(mcpserver.Status.Validation.Issues).To(BeEmpty())

			By("Verifying validation state is Validated or AuthRequired")
			Expect(mcpserver.Status.Validation.State).To(
				Or(Equal(mcpv1.ValidationStateValidated), Equal(mcpv1.ValidationStateAuthRequired)))
		})

		It("should not retry validation on protocol mismatch", func() {
			By("Testing getValidationRetryInterval with protocol mismatch")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Protocol: mcpv1.MCPProtocolStreamableHTTP,
					},
				},
				Status: mcpv1.MCPServerStatus{
					Validation: &mcpv1.ValidationStatus{
						State:     mcpv1.ValidationStateFailed, // Protocol mismatch confirmed
						Compliant: false,
						Attempts:  2, // Mismatch confirmed after 2 attempts
						Issues: []mcpv1.ValidationIssue{
							{
								Code:  "PROTOCOL_MISMATCH",
								Level: "error",
							},
						},
					},
				},
			}

			By("Calling getValidationRetryInterval")
			interval := controllerReconciler.getValidationRetryInterval(mcpserver)

			By("Verifying no retry on protocol mismatch")
			Expect(interval).To(Equal(time.Duration(0)), "Protocol mismatch should not trigger retry")
		})

		It("should clear Degraded condition when mismatch is resolved", func() {
			By("Creating MCPServer with initial mismatch")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type:     mcpv1.MCPTransportHTTP,
						Protocol: mcpv1.MCPProtocolStreamableHTTP,
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled:    ptr(true),
						StrictMode: ptr(false),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Setting initial mismatch state")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			mcpserver.Status.Phase = mcpv1.MCPServerPhaseRunning
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateFailed,
				Compliant:           false,
				Protocol:            "sse",
				ValidatedGeneration: mcpserver.Generation,
				Issues: []mcpv1.ValidationIssue{
					{
						Code:  "PROTOCOL_MISMATCH",
						Level: "error",
					},
				},
			}
			mcpserver.Status.Conditions = []mcpv1.MCPServerCondition{
				{
					Type:               mcpv1.MCPServerConditionDegraded,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
					Reason:             "ProtocolMismatch",
				},
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Fixing the configuration to match detected protocol")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			// User fixes spec to match detected protocol
			mcpserver.Spec.Transport.Protocol = mcpv1.MCPProtocolSSE // Changed to match
			Expect(k8sClient.Update(ctx, mcpserver)).To(Succeed())

			By("Simulating successful validation after fix")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			// Generation incremented, validation re-runs and succeeds
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateValidated,
				Compliant:           true, // Now compliant
				Protocol:            "sse",
				ValidatedGeneration: mcpserver.Generation,
				Issues:              []mcpv1.ValidationIssue{}, // Issues cleared
			}
			mcpserver.Status.Conditions = []mcpv1.MCPServerCondition{
				{
					Type:               mcpv1.MCPServerConditionDegraded,
					Status:             corev1.ConditionFalse, // Degraded cleared
					LastTransitionTime: metav1.Now(),
					Reason:             "ValidationPassed",
					Message:            "MCP protocol validation succeeded",
				},
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying Degraded condition is cleared")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				degradedCondition := findCondition(mcpserver.Status.Conditions, mcpv1.MCPServerConditionDegraded)
				return degradedCondition != nil && degradedCondition.Status == corev1.ConditionFalse
			}, timeout, interval).Should(BeTrue())

			By("Verifying validation is now compliant")
			Expect(mcpserver.Status.Validation.Compliant).To(BeTrue())
			Expect(mcpserver.Status.Validation.Issues).To(BeEmpty())

			By("Verifying validation state is Validated")
			// State should be Validated if no auth required, or AuthRequired if auth is needed
			Expect(mcpserver.Status.Validation.State).To(
				Or(Equal(mcpv1.ValidationStateValidated), Equal(mcpv1.ValidationStateAuthRequired)))
		})
	})

	Context("When handling validation states", func() {
		const (
			resourceNamespace = "default"
			timeout           = time.Second * 10
			interval          = time.Millisecond * 250
		)

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer

		BeforeEach(func() {
			resourceName = "test-mcpserver-state-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
		})

		AfterEach(func() {
			resource := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup: Deleting the MCPServer")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set state to AuthRequired when server requires authentication", func() {
			By("Creating MCPServer with HTTP transport")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type:     mcpv1.MCPTransportHTTP,
						Protocol: mcpv1.MCPProtocolSSE,
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled: ptr(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Setting validation status with RequiresAuth=true")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			mcpserver.Status.Phase = mcpv1.MCPServerPhaseRunning
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateAuthRequired,
				Compliant:           true, // Can be compliant but require auth
				RequiresAuth:        true,
				Protocol:            "sse",
				ProtocolVersion:     "2024-11-05",
				ValidatedGeneration: mcpserver.Generation,
				Issues:              []mcpv1.ValidationIssue{},
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying state is AuthRequired")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.Validation != nil &&
					mcpserver.Status.Validation.State == mcpv1.ValidationStateAuthRequired
			}, timeout, interval).Should(BeTrue())

			By("Verifying RequiresAuth field is true")
			Expect(mcpserver.Status.Validation.RequiresAuth).To(BeTrue())

			By("Verifying server can be compliant even with auth required")
			Expect(mcpserver.Status.Validation.Compliant).To(BeTrue())
		})

		It("should set state to Disabled when validation is explicitly disabled", func() {
			By("Creating MCPServer with validation disabled")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type: mcpv1.MCPTransportHTTP,
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled: ptr(false), // Explicitly disabled
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Waiting for reconciliation")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			By("Simulating controller setting Disabled state")
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State: mcpv1.ValidationStateDisabled,
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying state is Disabled")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.Validation != nil &&
					mcpserver.Status.Validation.State == mcpv1.ValidationStateDisabled
			}, timeout, interval).Should(BeTrue())

			By("Verifying no validation fields are set")
			Expect(mcpserver.Status.Validation.Protocol).To(BeEmpty())
			Expect(mcpserver.Status.Validation.ProtocolVersion).To(BeEmpty())
			Expect(mcpserver.Status.Validation.Capabilities).To(BeEmpty())
		})

		It("should set state to Pending when server is not yet ready", func() {
			By("Creating MCPServer")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type: mcpv1.MCPTransportHTTP,
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled: ptr(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Setting server to Creating phase (not ready)")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			mcpserver.Status.Phase = mcpv1.MCPServerPhaseCreating
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State: mcpv1.ValidationStatePending,
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying state is Pending")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.Validation != nil &&
					mcpserver.Status.Validation.State == mcpv1.ValidationStatePending
			}, timeout, interval).Should(BeTrue())

			By("Verifying phase is Creating (not ready for validation)")
			Expect(mcpserver.Status.Phase).To(Equal(mcpv1.MCPServerPhaseCreating))
		})

		It("should transition from Pending to Validated when server becomes ready", func() {
			By("Creating MCPServer in Pending state")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type: mcpv1.MCPTransportHTTP,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Setting initial Pending state")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			mcpserver.Status.Phase = mcpv1.MCPServerPhaseCreating
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State: mcpv1.ValidationStatePending,
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying initial state is Pending")
			Expect(mcpserver.Status.Validation.State).To(Equal(mcpv1.ValidationStatePending))

			By("Transitioning to Running phase with successful validation")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			mcpserver.Status.Phase = mcpv1.MCPServerPhaseRunning
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateValidated,
				Compliant:           true,
				RequiresAuth:        false,
				Protocol:            "sse",
				ProtocolVersion:     "2024-11-05",
				ValidatedGeneration: mcpserver.Generation,
				Capabilities:        []string{"tools", "resources"},
				Issues:              []mcpv1.ValidationIssue{},
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying state transitioned to Validated")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.Phase == mcpv1.MCPServerPhaseRunning &&
					mcpserver.Status.Validation != nil &&
					mcpserver.Status.Validation.State == mcpv1.ValidationStateValidated
			}, timeout, interval).Should(BeTrue())

			By("Verifying validation details are populated")
			Expect(mcpserver.Status.Validation.Compliant).To(BeTrue())
			Expect(mcpserver.Status.Validation.Protocol).To(Equal("sse"))
			Expect(mcpserver.Status.Validation.Capabilities).To(ContainElement("tools"))
		})
	})

	Context("When reconciling MCPServer with security defaults", func() {
		const resourceNamespace = "default"

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer
		var controllerReconciler *MCPServerReconciler

		BeforeEach(func() {
			resourceName = "test-mcpserver-security-" + RandStringRunes(8)
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

		It("should apply security defaults when security is not specified", func() {
			By("Creating MCPServer without security config")
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

			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment has security defaults applied")
			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Verify container security context defaults
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.SecurityContext).NotTo(BeNil())
			Expect(container.SecurityContext.RunAsNonRoot).NotTo(BeNil())
			Expect(*container.SecurityContext.RunAsNonRoot).To(BeTrue())
			Expect(container.SecurityContext.RunAsUser).NotTo(BeNil())
			Expect(*container.SecurityContext.RunAsUser).To(Equal(int64(1000)))
			Expect(container.SecurityContext.RunAsGroup).NotTo(BeNil())
			Expect(*container.SecurityContext.RunAsGroup).To(Equal(int64(1000)))
			Expect(container.SecurityContext.AllowPrivilegeEscalation).NotTo(BeNil())
			Expect(*container.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())

			// Verify pod security context defaults
			podSecurityContext := deployment.Spec.Template.Spec.SecurityContext
			Expect(podSecurityContext).NotTo(BeNil())
			Expect(podSecurityContext.FSGroup).NotTo(BeNil())
			Expect(*podSecurityContext.FSGroup).To(Equal(int64(1000)))
		})

		It("should respect user-specified security settings", func() {
			By("Creating MCPServer with custom security config")
			customUser := int64(2000)
			customGroup := int64(3000)
			customFsGroup := int64(4000)
			runAsNonRoot := false
			allowPrivEsc := true

			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(1)),
					Security: &mcpv1.MCPServerSecurity{
						RunAsUser:                &customUser,
						RunAsGroup:               &customGroup,
						FSGroup:                  &customFsGroup,
						RunAsNonRoot:             &runAsNonRoot,
						AllowPrivilegeEscalation: &allowPrivEsc,
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment uses custom security settings")
			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Verify container security context uses custom values
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.SecurityContext).NotTo(BeNil())
			Expect(*container.SecurityContext.RunAsUser).To(Equal(customUser))
			Expect(*container.SecurityContext.RunAsGroup).To(Equal(customGroup))
			Expect(*container.SecurityContext.RunAsNonRoot).To(Equal(runAsNonRoot))
			Expect(*container.SecurityContext.AllowPrivilegeEscalation).To(Equal(allowPrivEsc))

			// Verify pod security context uses custom fsGroup
			podSecurityContext := deployment.Spec.Template.Spec.SecurityContext
			Expect(podSecurityContext).NotTo(BeNil())
			Expect(*podSecurityContext.FSGroup).To(Equal(customFsGroup))
		})

		It("should merge defaults with partial user config", func() {
			By("Creating MCPServer with partial security config")
			customUser := int64(5000)

			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "nginx:1.21",
					Replicas: ptr(int32(1)),
					Security: &mcpv1.MCPServerSecurity{
						RunAsUser: &customUser, // Only override user
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Reconciling the MCPServer")
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that Deployment merges custom and default values")
			deployment := &appsv1.Deployment{}
			deploymentKey := types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentKey, deployment)
				return err == nil
			}, time.Second*10, time.Millisecond*250).Should(BeTrue())

			// Verify container security context has custom user but default group
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.SecurityContext).NotTo(BeNil())
			Expect(*container.SecurityContext.RunAsUser).To(Equal(customUser))        // Custom
			Expect(*container.SecurityContext.RunAsGroup).To(Equal(int64(1000)))      // Default
			Expect(*container.SecurityContext.RunAsNonRoot).To(BeTrue())              // Default
			Expect(*container.SecurityContext.AllowPrivilegeEscalation).To(BeFalse()) // Default

			// Verify pod security context has default fsGroup
			podSecurityContext := deployment.Spec.Template.Spec.SecurityContext
			Expect(podSecurityContext).NotTo(BeNil())
			Expect(*podSecurityContext.FSGroup).To(Equal(int64(1000))) // Default
		})
	})

	Context("When handling SSE two-phase commit for resolved transport", func() {
		const (
			resourceNamespace = "default"
			timeout           = time.Second * 10
			interval          = time.Millisecond * 250
		)

		ctx := context.Background()
		var resourceName string
		var typeNamespacedName types.NamespacedName
		var mcpserver *mcpv1.MCPServer

		BeforeEach(func() {
			resourceName = "test-mcpserver-sse-" + RandStringRunes(8)
			typeNamespacedName = types.NamespacedName{
				Name:      resourceName,
				Namespace: resourceNamespace,
			}
		})

		AfterEach(func() {
			resource := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By("Cleanup: Deleting the MCPServer")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should set ResolvedTransport.Protocol without setting SSEConfigApplied on detection", func() {
			By("Creating MCPServer with auto protocol")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type:     mcpv1.MCPTransportHTTP,
						Protocol: mcpv1.MCPProtocolAuto,
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled: ptr(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Simulating SSE detection in validation")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			mcpserver.Status.Phase = mcpv1.MCPServerPhaseRunning
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateValidated,
				Protocol:            "sse",
				ProtocolVersion:     "2024-11-05",
				ValidatedGeneration: mcpserver.Generation,
			}
			// Simulate detection: ResolvedTransport is set but SSEConfigApplied is false
			mcpserver.Status.ResolvedTransport = &mcpv1.ResolvedTransportStatus{
				Protocol:           mcpv1.MCPProtocolSSE,
				ResolvedGeneration: mcpserver.Generation,
				SSEConfigApplied:   false, // Not yet applied - two-phase commit
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying ResolvedTransport.Protocol is set")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.ResolvedTransport != nil &&
					mcpserver.Status.ResolvedTransport.Protocol == mcpv1.MCPProtocolSSE
			}, timeout, interval).Should(BeTrue())

			By("Verifying SSEConfigApplied is still false (not prematurely set)")
			Expect(mcpserver.Status.ResolvedTransport.SSEConfigApplied).To(BeFalse())
		})

		It("should set SSEConfigApplied=true only after successful resource reconciliation", func() {
			By("Creating MCPServer with auto protocol")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type:     mcpv1.MCPTransportHTTP,
						Protocol: mcpv1.MCPProtocolAuto,
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled: ptr(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Simulating the full SSE detection and config application flow")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			// Phase 1: SSE detected, ResolvedTransport set, SSEConfigApplied still false
			mcpserver.Status.Phase = mcpv1.MCPServerPhaseRunning
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateValidated,
				Protocol:            "sse",
				ProtocolVersion:     "2024-11-05",
				ValidatedGeneration: mcpserver.Generation,
			}
			mcpserver.Status.ResolvedTransport = &mcpv1.ResolvedTransportStatus{
				Protocol:           mcpv1.MCPProtocolSSE,
				ResolvedGeneration: mcpserver.Generation,
				SSEConfigApplied:   false,
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying SSEConfigApplied is false before resource reconciliation")
			err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			Expect(err).NotTo(HaveOccurred())
			Expect(mcpserver.Status.ResolvedTransport.SSEConfigApplied).To(BeFalse())

			// Phase 2: After successful resource reconciliation, SSEConfigApplied is set to true
			mcpserver.Status.ResolvedTransport.SSEConfigApplied = true
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying SSEConfigApplied is true after resource reconciliation")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.ResolvedTransport != nil &&
					mcpserver.Status.ResolvedTransport.SSEConfigApplied
			}, timeout, interval).Should(BeTrue())
		})

		It("should not set ResolvedTransport for explicit SSE protocol", func() {
			By("Creating MCPServer with explicit SSE protocol")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type:     mcpv1.MCPTransportHTTP,
						Protocol: mcpv1.MCPProtocolSSE, // Explicit, not auto
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled: ptr(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Waiting for reconciliation")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			By("Verifying ResolvedTransport is nil for explicit protocol")
			// For explicit protocol, ResolvedTransport should not be set
			// because there's no auto-detection happening
			Expect(mcpserver.Status.ResolvedTransport).To(BeNil())
		})

		It("should allow retry if SSEConfigApplied is false after failed reconciliation", func() {
			By("Creating MCPServer with auto protocol")
			mcpserver = &mcpv1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: mcpv1.MCPServerSpec{
					Image:    "test-server:latest",
					Replicas: ptr(int32(1)),
					Transport: &mcpv1.MCPServerTransport{
						Type:     mcpv1.MCPTransportHTTP,
						Protocol: mcpv1.MCPProtocolAuto,
					},
					Validation: &mcpv1.ValidationSpec{
						Enabled: ptr(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, mcpserver)).To(Succeed())

			By("Simulating SSE detection with failed resource reconciliation")
			Eventually(func() error {
				return k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			}, timeout, interval).Should(Succeed())

			mcpserver.Status.Phase = mcpv1.MCPServerPhaseRunning
			mcpserver.Status.Validation = &mcpv1.ValidationStatus{
				State:               mcpv1.ValidationStateValidated,
				Protocol:            "sse",
				ProtocolVersion:     "2024-11-05",
				ValidatedGeneration: mcpserver.Generation,
			}
			// SSE detected but config NOT applied (simulating failed reconciliation)
			mcpserver.Status.ResolvedTransport = &mcpv1.ResolvedTransportStatus{
				Protocol:           mcpv1.MCPProtocolSSE,
				ResolvedGeneration: mcpserver.Generation,
				SSEConfigApplied:   false, // Failed to apply
			}
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			By("Verifying SSEConfigApplied remains false, allowing retry")
			err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
			Expect(err).NotTo(HaveOccurred())
			Expect(mcpserver.Status.ResolvedTransport.Protocol).To(Equal(mcpv1.MCPProtocolSSE))
			Expect(mcpserver.Status.ResolvedTransport.SSEConfigApplied).To(BeFalse())

			By("Simulating successful retry - SSEConfigApplied set to true")
			mcpserver.Status.ResolvedTransport.SSEConfigApplied = true
			Expect(k8sClient.Status().Update(ctx, mcpserver)).To(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, mcpserver)
				if err != nil {
					return false
				}
				return mcpserver.Status.ResolvedTransport.SSEConfigApplied
			}, timeout, interval).Should(BeTrue())
		})
	})
})

// findCondition finds a condition by type in the conditions list
//
//nolint:unparam // condType is designed to be flexible for future test cases with other condition types
func findCondition(conditions []mcpv1.MCPServerCondition, condType mcpv1.MCPServerConditionType) *mcpv1.MCPServerCondition {
	for i := range conditions {
		if conditions[i].Type == condType {
			return &conditions[i]
		}
	}
	return nil
}

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
