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
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

// BuildStandardLabels constructs the standard labels for MCPServer resources
func BuildStandardLabels(mcpServer *mcpv1.MCPServer) map[string]string {
	labels := map[string]string{
		"app":                          mcpServer.Name,
		"app.kubernetes.io/name":       "mcpserver",
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/component":  "mcp-server",
		"app.kubernetes.io/managed-by": "mcp-operator",
	}

	// Add capabilities as labels
	if len(mcpServer.Spec.Capabilities) > 0 {
		for i, cap := range mcpServer.Spec.Capabilities {
			labels[fmt.Sprintf("mcp.capability.%d", i)] = cap
		}
	}

	// Add transport type as label
	if mcpServer.Spec.Transport != nil {
		labels["mcp.transport.type"] = string(mcpServer.Spec.Transport.Type)
	}

	// Merge with custom labels from PodTemplate
	if mcpServer.Spec.PodTemplate != nil && mcpServer.Spec.PodTemplate.Labels != nil {
		for k, v := range mcpServer.Spec.PodTemplate.Labels {
			labels[k] = v
		}
	}

	return labels
}

// BuildAnnotations constructs the annotations for the pod template
func BuildAnnotations(mcpServer *mcpv1.MCPServer) map[string]string {
	annotations := map[string]string{}
	if mcpServer.Spec.PodTemplate != nil && mcpServer.Spec.PodTemplate.Annotations != nil {
		annotations = mcpServer.Spec.PodTemplate.Annotations
	}
	return annotations
}

// GetReplicaCount returns the desired replica count for the MCPServer
func GetReplicaCount(mcpServer *mcpv1.MCPServer) int32 {
	if mcpServer.Spec.Replicas != nil {
		return *mcpServer.Spec.Replicas
	}
	return 1
}

// BuildBaseContainer creates a base container with common configuration
func BuildBaseContainer(mcpServer *mcpv1.MCPServer, port int32) corev1.Container {
	container := corev1.Container{
		Name:  "mcp-server",
		Image: mcpServer.Spec.Image,
		Env:   mcpServer.Spec.Environment,
	}

	// Add custom command and args if specified
	if len(mcpServer.Spec.Command) > 0 {
		container.Command = mcpServer.Spec.Command
	}
	if len(mcpServer.Spec.Args) > 0 {
		container.Args = mcpServer.Spec.Args
	}

	// Add port if needed
	if port > 0 {
		container.Ports = []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: port,
				Protocol:      corev1.ProtocolTCP,
			},
		}
	}

	// Add resource requirements
	if mcpServer.Spec.Resources != nil {
		container.Resources = *mcpServer.Spec.Resources
	}

	// Add volume mounts
	if mcpServer.Spec.PodTemplate != nil && len(mcpServer.Spec.PodTemplate.VolumeMounts) > 0 {
		container.VolumeMounts = mcpServer.Spec.PodTemplate.VolumeMounts
	}

	// Add security context
	if mcpServer.Spec.Security != nil {
		securityContext := &corev1.SecurityContext{}
		if mcpServer.Spec.Security.RunAsUser != nil {
			securityContext.RunAsUser = mcpServer.Spec.Security.RunAsUser
		}
		if mcpServer.Spec.Security.RunAsGroup != nil {
			securityContext.RunAsGroup = mcpServer.Spec.Security.RunAsGroup
		}
		if mcpServer.Spec.Security.ReadOnlyRootFilesystem != nil {
			securityContext.ReadOnlyRootFilesystem = mcpServer.Spec.Security.ReadOnlyRootFilesystem
		}
		container.SecurityContext = securityContext
	}

	return container
}

// AddHealthProbes adds health check probes to a container
func AddHealthProbes(container *corev1.Container, mcpServer *mcpv1.MCPServer, port int32) {
	// Only add health probes if HealthCheck is explicitly configured
	if mcpServer.Spec.HealthCheck == nil {
		return
	}

	// If HealthCheck is specified but explicitly disabled, skip adding probes
	if mcpServer.Spec.HealthCheck.Enabled != nil && !*mcpServer.Spec.HealthCheck.Enabled {
		return
	}

	healthPath := "/health"
	if mcpServer.Spec.HealthCheck != nil && mcpServer.Spec.HealthCheck.Path != "" {
		healthPath = mcpServer.Spec.HealthCheck.Path
	}

	healthPort := intstr.FromInt(int(port))
	if mcpServer.Spec.HealthCheck != nil && mcpServer.Spec.HealthCheck.Port != nil {
		healthPort = *mcpServer.Spec.HealthCheck.Port
	}

	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: healthPath,
				Port: healthPort,
			},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
		SuccessThreshold:    1,
	}

	// Apply custom health check settings
	if mcpServer.Spec.HealthCheck != nil {
		if mcpServer.Spec.HealthCheck.InitialDelaySeconds != nil {
			probe.InitialDelaySeconds = *mcpServer.Spec.HealthCheck.InitialDelaySeconds
		}
		if mcpServer.Spec.HealthCheck.PeriodSeconds != nil {
			probe.PeriodSeconds = *mcpServer.Spec.HealthCheck.PeriodSeconds
		}
		if mcpServer.Spec.HealthCheck.TimeoutSeconds != nil {
			probe.TimeoutSeconds = *mcpServer.Spec.HealthCheck.TimeoutSeconds
		}
		if mcpServer.Spec.HealthCheck.FailureThreshold != nil {
			probe.FailureThreshold = *mcpServer.Spec.HealthCheck.FailureThreshold
		}
		if mcpServer.Spec.HealthCheck.SuccessThreshold != nil {
			probe.SuccessThreshold = *mcpServer.Spec.HealthCheck.SuccessThreshold
		}
	}

	container.LivenessProbe = probe
	container.ReadinessProbe = probe
}

// BuildBasePodSpec creates a base PodSpec with common configuration
func BuildBasePodSpec(mcpServer *mcpv1.MCPServer, containers []corev1.Container) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		ServiceAccountName: mcpServer.Name,
		Containers:         containers,
	}

	// Apply pod template specifications
	if mcpServer.Spec.PodTemplate != nil {
		applyPodTemplateSpec(&podSpec, mcpServer.Spec.PodTemplate)
	}

	return podSpec
}

// applyPodTemplateSpec applies pod template configurations to the pod spec
func applyPodTemplateSpec(podSpec *corev1.PodSpec, template *mcpv1.MCPServerPodTemplate) {
	if template.NodeSelector != nil {
		podSpec.NodeSelector = template.NodeSelector
	}
	if len(template.Tolerations) > 0 {
		podSpec.Tolerations = template.Tolerations
	}
	if template.Affinity != nil {
		podSpec.Affinity = template.Affinity
	}
	if template.ServiceAccountName != "" {
		podSpec.ServiceAccountName = template.ServiceAccountName
	}
	if len(template.ImagePullSecrets) > 0 {
		podSpec.ImagePullSecrets = template.ImagePullSecrets
	}
	if len(template.Volumes) > 0 {
		podSpec.Volumes = template.Volumes
	}
}

// BuildService creates a Service object for network transports
func BuildService(
	mcpServer *mcpv1.MCPServer,
	port int32,
	protocol corev1.Protocol,
	annotations map[string]string,
) *corev1.Service {
	labels := BuildStandardLabels(mcpServer)

	serviceType := corev1.ServiceTypeClusterIP
	if mcpServer.Spec.Service != nil && mcpServer.Spec.Service.Type != "" {
		serviceType = mcpServer.Spec.Service.Type
	}

	targetPort := intstr.FromInt(int(port))
	if mcpServer.Spec.Service != nil && mcpServer.Spec.Service.TargetPort != nil {
		targetPort = *mcpServer.Spec.Service.TargetPort
	}

	// Merge service annotations
	serviceAnnotations := map[string]string{}
	if mcpServer.Spec.Service != nil && mcpServer.Spec.Service.Annotations != nil {
		for k, v := range mcpServer.Spec.Service.Annotations {
			serviceAnnotations[k] = v
		}
	}
	for k, v := range annotations {
		serviceAnnotations[k] = v
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        mcpServer.Name,
			Namespace:   mcpServer.Namespace,
			Labels:      labels,
			Annotations: serviceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type: serviceType,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: targetPort,
					Protocol:   protocol,
				},
			},
			Selector: map[string]string{
				"app": mcpServer.Name,
			},
		},
	}

	return service
}

// BuildDeployment creates a Deployment object
func BuildDeployment(mcpServer *mcpv1.MCPServer, podSpec corev1.PodSpec) *appsv1.Deployment {
	replicas := GetReplicaCount(mcpServer)
	labels := BuildStandardLabels(mcpServer)
	annotations := BuildAnnotations(mcpServer)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": mcpServer.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: annotations,
				},
				Spec: podSpec,
			},
		},
	}
}

// UpdateService updates a service with the given spec, avoiding duplication
func UpdateService(
	ctx context.Context,
	k8sClient client.Client,
	scheme *runtime.Scheme,
	mcpServer *mcpv1.MCPServer,
	service *corev1.Service,
) error {
	if err := controllerutil.SetControllerReference(mcpServer, service, scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil {
		return err
	}

	// Update service if necessary
	if found.Spec.Type != service.Spec.Type ||
		!reflect.DeepEqual(found.Spec.Ports, service.Spec.Ports) ||
		!reflect.DeepEqual(found.Spec.Selector, service.Spec.Selector) {
		found.Spec.Type = service.Spec.Type
		found.Spec.Ports = service.Spec.Ports
		found.Spec.Selector = service.Spec.Selector
		return k8sClient.Update(ctx, found)
	}

	return nil
}
