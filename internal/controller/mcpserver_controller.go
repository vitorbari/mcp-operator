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
	"fmt"
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
)

// MCPServerReconciler reconciles a MCPServer object
type MCPServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=mcp.mcp-operator.io,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.mcp-operator.io,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.mcp-operator.io,resources=mcpservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

const (
	mcpServerFinalizer = "mcp.mcp-operator.io/finalizer"
	defaultPort        = 8080
	defaultHealthPath  = "/health"
	defaultMetricsPath = "/metrics"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("mcpserver", req.NamespacedName)

	// Fetch the MCPServer instance
	mcpServer := &mcpv1.MCPServer{}
	if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
		if errors.IsNotFound(err) {
			log.Info("MCPServer resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get MCPServer")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if mcpServer.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, mcpServer)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(mcpServer, mcpServerFinalizer) {
		controllerutil.AddFinalizer(mcpServer, mcpServerFinalizer)
		if err := r.Update(ctx, mcpServer); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Update status phase to Creating if it's empty
	if mcpServer.Status.Phase == "" {
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseCreating
		mcpServer.Status.Message = "Starting MCPServer deployment"
		if err := r.updateStatus(ctx, mcpServer); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile ServiceAccount
	if err := r.reconcileServiceAccount(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to reconcile ServiceAccount")
		return r.updateStatusWithError(ctx, mcpServer, err)
	}

	// Reconcile RBAC if security settings are provided
	if mcpServer.Spec.Security != nil {
		if err := r.reconcileRBAC(ctx, mcpServer); err != nil {
			log.Error(err, "Failed to reconcile RBAC")
			return r.updateStatusWithError(ctx, mcpServer, err)
		}
	}

	// Reconcile Deployment
	if err := r.reconcileDeployment(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to reconcile Deployment")
		return r.updateStatusWithError(ctx, mcpServer, err)
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to reconcile Service")
		return r.updateStatusWithError(ctx, mcpServer, err)
	}

	// Reconcile HPA if enabled
	if err := r.reconcileHPA(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to reconcile HPA")
		return r.updateStatusWithError(ctx, mcpServer, err)
	}

	// Update status based on deployment status
	if err := r.updateMCPServerStatus(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to update MCPServer status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled MCPServer")
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// handleDeletion handles the deletion of MCPServer resources
func (r *MCPServerReconciler) handleDeletion(ctx context.Context, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Update status to Terminating
	mcpServer.Status.Phase = mcpv1.MCPServerPhaseTerminating
	mcpServer.Status.Message = "Terminating MCPServer resources"
	if err := r.updateStatus(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to update status during deletion")
	}

	// Remove finalizer to allow deletion
	controllerutil.RemoveFinalizer(mcpServer, mcpServerFinalizer)
	if err := r.Update(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("Successfully handled MCPServer deletion")
	return ctrl.Result{}, nil
}

// reconcileServiceAccount ensures the ServiceAccount exists
func (r *MCPServerReconciler) reconcileServiceAccount(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
		},
	}

	if err := controllerutil.SetControllerReference(mcpServer, serviceAccount, r.Scheme); err != nil {
		return err
	}

	found := &corev1.ServiceAccount{}
	err := r.Get(ctx, types.NamespacedName{Name: serviceAccount.Name, Namespace: serviceAccount.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, serviceAccount); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

// reconcileRBAC creates Role and RoleBinding for user/group access control
func (r *MCPServerReconciler) reconcileRBAC(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	if mcpServer.Spec.Security == nil || (len(mcpServer.Spec.Security.AllowedUsers) == 0 && len(mcpServer.Spec.Security.AllowedGroups) == 0) {
		return nil
	}

	// Create Role
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-access", mcpServer.Name),
			Namespace: mcpServer.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"services"},
				Verbs:         []string{"get", "list"},
				ResourceNames: []string{mcpServer.Name},
			},
		},
	}

	if err := controllerutil.SetControllerReference(mcpServer, role, r.Scheme); err != nil {
		return err
	}

	found := &rbacv1.Role{}
	err := r.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: role.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, role); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// Create RoleBinding
	subjects := []rbacv1.Subject{}

	// Add allowed users
	for _, user := range mcpServer.Spec.Security.AllowedUsers {
		subjects = append(subjects, rbacv1.Subject{
			Kind: "User",
			Name: user,
		})
	}

	// Add allowed groups
	for _, group := range mcpServer.Spec.Security.AllowedGroups {
		subjects = append(subjects, rbacv1.Subject{
			Kind: "Group",
			Name: group,
		})
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-access", mcpServer.Name),
			Namespace: mcpServer.Namespace,
		},
		Subjects: subjects,
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     role.Name,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	if err := controllerutil.SetControllerReference(mcpServer, roleBinding, r.Scheme); err != nil {
		return err
	}

	foundRB := &rbacv1.RoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Name: roleBinding.Name, Namespace: roleBinding.Namespace}, foundRB)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, roleBinding); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if !reflect.DeepEqual(foundRB.Subjects, roleBinding.Subjects) {
		foundRB.Subjects = roleBinding.Subjects
		if err := r.Update(ctx, foundRB); err != nil {
			return err
		}
	}

	return nil
}

// reconcileDeployment ensures the Deployment exists and matches the spec
func (r *MCPServerReconciler) reconcileDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	deployment := r.buildDeployment(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, deployment, r.Scheme); err != nil {
		return err
	}

	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, deployment); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update deployment if necessary
		if !reflect.DeepEqual(found.Spec, deployment.Spec) {
			found.Spec = deployment.Spec
			if err := r.Update(ctx, found); err != nil {
				return err
			}
		}
	}

	return nil
}

// buildDeployment creates a Deployment object for the MCPServer
func (r *MCPServerReconciler) buildDeployment(mcpServer *mcpv1.MCPServer) *appsv1.Deployment {
	replicas := r.getReplicaCount(mcpServer)
	labels := r.buildLabels(mcpServer)
	annotations := r.buildAnnotations(mcpServer)

	container := r.buildContainer(mcpServer)
	podSpec := r.buildPodSpec(mcpServer, container)

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

// getReplicaCount returns the desired replica count for the MCPServer
func (r *MCPServerReconciler) getReplicaCount(mcpServer *mcpv1.MCPServer) int32 {
	if mcpServer.Spec.Replicas != nil {
		return *mcpServer.Spec.Replicas
	}
	return 1
}

// buildLabels constructs the standard labels for MCPServer resources
func (r *MCPServerReconciler) buildLabels(mcpServer *mcpv1.MCPServer) map[string]string {
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

	// Merge with custom labels from PodTemplate
	if mcpServer.Spec.PodTemplate != nil && mcpServer.Spec.PodTemplate.Labels != nil {
		for k, v := range mcpServer.Spec.PodTemplate.Labels {
			labels[k] = v
		}
	}

	return labels
}

// buildAnnotations constructs the annotations for the pod template
func (r *MCPServerReconciler) buildAnnotations(mcpServer *mcpv1.MCPServer) map[string]string {
	annotations := map[string]string{}
	if mcpServer.Spec.PodTemplate != nil && mcpServer.Spec.PodTemplate.Annotations != nil {
		annotations = mcpServer.Spec.PodTemplate.Annotations
	}
	return annotations
}

// buildContainer creates the main MCP server container
func (r *MCPServerReconciler) buildContainer(mcpServer *mcpv1.MCPServer) corev1.Container {
	container := corev1.Container{
		Name:  "mcp-server",
		Image: mcpServer.Spec.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: r.getServicePort(mcpServer),
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: mcpServer.Spec.Environment,
	}

	// Add resource requirements
	if mcpServer.Spec.Resources != nil {
		container.Resources = *mcpServer.Spec.Resources
	}

	// Add health probes
	r.addHealthProbes(&container, mcpServer)

	// Add volume mounts
	if mcpServer.Spec.PodTemplate != nil && len(mcpServer.Spec.PodTemplate.VolumeMounts) > 0 {
		container.VolumeMounts = mcpServer.Spec.PodTemplate.VolumeMounts
	}

	// Add security context
	r.addSecurityContext(&container, mcpServer)

	return container
}

// addHealthProbes configures health check probes for the container
func (r *MCPServerReconciler) addHealthProbes(container *corev1.Container, mcpServer *mcpv1.MCPServer) {
	if mcpServer.Spec.HealthCheck != nil && mcpServer.Spec.HealthCheck.Enabled != nil && !*mcpServer.Spec.HealthCheck.Enabled {
		return
	}

	healthPath := defaultHealthPath
	if mcpServer.Spec.HealthCheck != nil && mcpServer.Spec.HealthCheck.Path != "" {
		healthPath = mcpServer.Spec.HealthCheck.Path
	}

	healthPort := intstr.FromInt(int(r.getServicePort(mcpServer)))
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

// addSecurityContext configures security context for the container
func (r *MCPServerReconciler) addSecurityContext(container *corev1.Container, mcpServer *mcpv1.MCPServer) {
	if mcpServer.Spec.Security == nil {
		return
	}

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

// buildPodSpec creates the PodSpec with the given container and MCPServer configuration
func (r *MCPServerReconciler) buildPodSpec(mcpServer *mcpv1.MCPServer, container corev1.Container) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		ServiceAccountName: mcpServer.Name,
		Containers:         []corev1.Container{container},
	}

	// Apply pod template specifications
	if mcpServer.Spec.PodTemplate != nil {
		r.applyPodTemplateSpec(&podSpec, mcpServer.Spec.PodTemplate)
	}

	return podSpec
}

// applyPodTemplateSpec applies pod template configurations to the pod spec
func (r *MCPServerReconciler) applyPodTemplateSpec(podSpec *corev1.PodSpec, template *mcpv1.MCPServerPodTemplate) {
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

// reconcileService ensures the Service exists and matches the spec
func (r *MCPServerReconciler) reconcileService(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	service := r.buildService(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, service, r.Scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, service); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update service if necessary (excluding fields that are managed by Kubernetes)
		if found.Spec.Type != service.Spec.Type ||
			!reflect.DeepEqual(found.Spec.Ports, service.Spec.Ports) ||
			!reflect.DeepEqual(found.Spec.Selector, service.Spec.Selector) {
			found.Spec.Type = service.Spec.Type
			found.Spec.Ports = service.Spec.Ports
			found.Spec.Selector = service.Spec.Selector
			if err := r.Update(ctx, found); err != nil {
				return err
			}
		}
	}

	return nil
}

// buildService creates a Service object for the MCPServer
func (r *MCPServerReconciler) buildService(mcpServer *mcpv1.MCPServer) *corev1.Service {
	labels := map[string]string{
		"app":                          mcpServer.Name,
		"app.kubernetes.io/name":       "mcpserver",
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/component":  "mcp-server",
		"app.kubernetes.io/managed-by": "mcp-operator",
	}

	serviceType := corev1.ServiceTypeClusterIP
	port := r.getServicePort(mcpServer)
	protocol := corev1.ProtocolTCP
	annotations := map[string]string{}

	if mcpServer.Spec.Service != nil {
		if mcpServer.Spec.Service.Type != "" {
			serviceType = mcpServer.Spec.Service.Type
		}
		if mcpServer.Spec.Service.Port != 0 {
			port = mcpServer.Spec.Service.Port
		}
		if mcpServer.Spec.Service.Protocol != "" {
			protocol = mcpServer.Spec.Service.Protocol
		}
		if mcpServer.Spec.Service.Annotations != nil {
			annotations = mcpServer.Spec.Service.Annotations
		}
	}

	targetPort := intstr.FromInt(int(port))
	if mcpServer.Spec.Service != nil && mcpServer.Spec.Service.TargetPort != nil {
		targetPort = *mcpServer.Spec.Service.TargetPort
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        mcpServer.Name,
			Namespace:   mcpServer.Namespace,
			Labels:      labels,
			Annotations: annotations,
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

// getServicePort returns the service port for the MCPServer
func (r *MCPServerReconciler) getServicePort(mcpServer *mcpv1.MCPServer) int32 {
	if mcpServer.Spec.Service != nil && mcpServer.Spec.Service.Port != 0 {
		return mcpServer.Spec.Service.Port
	}
	return defaultPort
}

// reconcileHPA ensures the HorizontalPodAutoscaler exists if enabled
func (r *MCPServerReconciler) reconcileHPA(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// If HPA is not enabled, delete any existing HPA
	if mcpServer.Spec.HPA == nil || mcpServer.Spec.HPA.Enabled == nil || !*mcpServer.Spec.HPA.Enabled {
		// Try to delete existing HPA if it exists
		existingHPA := &autoscalingv2.HorizontalPodAutoscaler{}
		err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, existingHPA)
		if err == nil {
			// HPA exists, delete it
			if err := r.Delete(ctx, existingHPA); err != nil {
				return err
			}
		} else if !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	hpa := r.buildHPA(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, hpa, r.Scheme); err != nil {
		return err
	}

	found := &autoscalingv2.HorizontalPodAutoscaler{}
	err := r.Get(ctx, types.NamespacedName{Name: hpa.Name, Namespace: hpa.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, hpa); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update HPA if necessary
		if !reflect.DeepEqual(found.Spec, hpa.Spec) {
			found.Spec = hpa.Spec
			if err := r.Update(ctx, found); err != nil {
				return err
			}
		}
	}

	return nil
}

// buildHPA creates a HorizontalPodAutoscaler object for the MCPServer
func (r *MCPServerReconciler) buildHPA(mcpServer *mcpv1.MCPServer) *autoscalingv2.HorizontalPodAutoscaler {
	labels := map[string]string{
		"app":                          mcpServer.Name,
		"app.kubernetes.io/name":       "mcpserver",
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/component":  "mcp-server",
		"app.kubernetes.io/managed-by": "mcp-operator",
	}

	// Set default values
	minReplicas := int32(1)
	if mcpServer.Spec.HPA.MinReplicas != nil {
		minReplicas = *mcpServer.Spec.HPA.MinReplicas
	}

	maxReplicas := int32(10)
	if mcpServer.Spec.HPA.MaxReplicas != nil {
		maxReplicas = *mcpServer.Spec.HPA.MaxReplicas
	}

	// Build metrics
	var metrics []autoscalingv2.MetricSpec

	// CPU metric
	if mcpServer.Spec.HPA.TargetCPUUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: mcpServer.Spec.HPA.TargetCPUUtilizationPercentage,
				},
			},
		})
	}

	// Memory metric
	if mcpServer.Spec.HPA.TargetMemoryUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: mcpServer.Spec.HPA.TargetMemoryUtilizationPercentage,
				},
			},
		})
	}

	hpaSpec := autoscalingv2.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       mcpServer.Name,
		},
		MinReplicas: &minReplicas,
		MaxReplicas: maxReplicas,
		Metrics:     metrics,
	}

	// Add scaling behavior if specified
	if mcpServer.Spec.HPA.ScaleUpBehavior != nil || mcpServer.Spec.HPA.ScaleDownBehavior != nil {
		behavior := &autoscalingv2.HorizontalPodAutoscalerBehavior{}

		if mcpServer.Spec.HPA.ScaleUpBehavior != nil {
			behavior.ScaleUp = r.buildHPAScalingRules(mcpServer.Spec.HPA.ScaleUpBehavior)
		}

		if mcpServer.Spec.HPA.ScaleDownBehavior != nil {
			behavior.ScaleDown = r.buildHPAScalingRules(mcpServer.Spec.HPA.ScaleDownBehavior)
		}

		hpaSpec.Behavior = behavior
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: hpaSpec,
	}

	return hpa
}

// buildHPAScalingRules converts MCPServerHPABehavior to autoscalingv2.HPAScalingRules
func (r *MCPServerReconciler) buildHPAScalingRules(behavior *mcpv1.MCPServerHPABehavior) *autoscalingv2.HPAScalingRules {
	rules := &autoscalingv2.HPAScalingRules{}

	if behavior.StabilizationWindowSeconds != nil {
		rules.StabilizationWindowSeconds = behavior.StabilizationWindowSeconds
	}

	if len(behavior.Policies) > 0 {
		policies := make([]autoscalingv2.HPAScalingPolicy, len(behavior.Policies))
		for i, policy := range behavior.Policies {
			policies[i] = autoscalingv2.HPAScalingPolicy{
				Type:          autoscalingv2.HPAScalingPolicyType(policy.Type),
				Value:         policy.Value,
				PeriodSeconds: policy.PeriodSeconds,
			}
		}
		rules.Policies = policies
	}

	return rules
}

// updateStatus updates the MCPServer status
func (r *MCPServerReconciler) updateStatus(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	mcpServer.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
	mcpServer.Status.ObservedGeneration = mcpServer.Generation
	return r.Status().Update(ctx, mcpServer)
}

// updateStatusWithError updates the MCPServer status with error information
func (r *MCPServerReconciler) updateStatusWithError(ctx context.Context, mcpServer *mcpv1.MCPServer, err error) (ctrl.Result, error) {
	mcpServer.Status.Phase = mcpv1.MCPServerPhaseFailed
	mcpServer.Status.Message = fmt.Sprintf("Error: %v", err)

	// Update conditions
	condition := mcpv1.MCPServerCondition{
		Type:               mcpv1.MCPServerConditionReconciled,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             "ReconcileError",
		Message:            err.Error(),
	}
	r.setCondition(mcpServer, condition)

	if updateErr := r.updateStatus(ctx, mcpServer); updateErr != nil {
		return ctrl.Result{}, updateErr
	}
	return ctrl.Result{RequeueAfter: time.Minute * 2}, err
}

// updateMCPServerStatus updates the MCPServer status based on deployment status
func (r *MCPServerReconciler) updateMCPServerStatus(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Re-fetch the MCPServer to get the latest resourceVersion and avoid conflicts
	if err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, mcpServer); err != nil {
		return err
	}

	// Get deployment status
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, deployment)
	if err != nil {
		return err
	}

	// Update replica counts
	mcpServer.Status.Replicas = deployment.Status.Replicas
	mcpServer.Status.ReadyReplicas = deployment.Status.ReadyReplicas
	mcpServer.Status.AvailableReplicas = deployment.Status.AvailableReplicas

	// Determine phase based on deployment status
	if deployment.Status.Replicas == 0 {
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseCreating
		mcpServer.Status.Message = "Creating deployment"
	} else if deployment.Status.ReadyReplicas == 0 {
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseCreating
		mcpServer.Status.Message = "Waiting for pods to become ready"
	} else if deployment.Status.ReadyReplicas < deployment.Status.Replicas {
		if deployment.Status.UpdatedReplicas < deployment.Status.Replicas {
			mcpServer.Status.Phase = mcpv1.MCPServerPhaseUpdating
			mcpServer.Status.Message = "Rolling out update"
		} else {
			mcpServer.Status.Phase = mcpv1.MCPServerPhaseScaling
			mcpServer.Status.Message = "Scaling deployment"
		}
	} else {
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseRunning
		mcpServer.Status.Message = "MCPServer is running"
	}

	// Set service endpoint
	service := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, service); err == nil {
		port := r.getServicePort(mcpServer)
		if service.Spec.Type == corev1.ServiceTypeLoadBalancer && len(service.Status.LoadBalancer.Ingress) > 0 {
			ingress := service.Status.LoadBalancer.Ingress[0]
			if ingress.IP != "" {
				mcpServer.Status.ServiceEndpoint = fmt.Sprintf("http://%s:%d", ingress.IP, port)
			} else if ingress.Hostname != "" {
				mcpServer.Status.ServiceEndpoint = fmt.Sprintf("http://%s:%d", ingress.Hostname, port)
			}
		} else {
			mcpServer.Status.ServiceEndpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d", service.Name, service.Namespace, port)
		}
	}

	// Update conditions
	r.updateConditions(mcpServer, deployment)

	// Update status
	return r.updateStatus(ctx, mcpServer)
}

// updateConditions updates the MCPServer conditions based on deployment status
func (r *MCPServerReconciler) updateConditions(mcpServer *mcpv1.MCPServer, deployment *appsv1.Deployment) {
	now := metav1.Now()

	// Ready condition
	readyCondition := mcpv1.MCPServerCondition{
		Type:               mcpv1.MCPServerConditionReady,
		LastTransitionTime: now,
	}
	if deployment.Status.ReadyReplicas > 0 {
		readyCondition.Status = corev1.ConditionTrue
		readyCondition.Reason = "DeploymentReady"
		readyCondition.Message = "MCPServer deployment has ready replicas"
	} else {
		readyCondition.Status = corev1.ConditionFalse
		readyCondition.Reason = "DeploymentNotReady"
		readyCondition.Message = "MCPServer deployment has no ready replicas"
	}
	r.setCondition(mcpServer, readyCondition)

	// Available condition
	availableCondition := mcpv1.MCPServerCondition{
		Type:               mcpv1.MCPServerConditionAvailable,
		LastTransitionTime: now,
	}
	if deployment.Status.AvailableReplicas > 0 {
		availableCondition.Status = corev1.ConditionTrue
		availableCondition.Reason = "DeploymentAvailable"
		availableCondition.Message = "MCPServer deployment has available replicas"
	} else {
		availableCondition.Status = corev1.ConditionFalse
		availableCondition.Reason = "DeploymentNotAvailable"
		availableCondition.Message = "MCPServer deployment has no available replicas"
	}
	r.setCondition(mcpServer, availableCondition)

	// Progressing condition
	progressingCondition := mcpv1.MCPServerCondition{
		Type:               mcpv1.MCPServerConditionProgressing,
		LastTransitionTime: now,
	}
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			if condition.Status == corev1.ConditionTrue {
				progressingCondition.Status = corev1.ConditionTrue
				progressingCondition.Reason = condition.Reason
				progressingCondition.Message = condition.Message
			} else {
				progressingCondition.Status = corev1.ConditionFalse
				progressingCondition.Reason = condition.Reason
				progressingCondition.Message = condition.Message
			}
			break
		}
	}
	r.setCondition(mcpServer, progressingCondition)

	// Reconciled condition
	reconciledCondition := mcpv1.MCPServerCondition{
		Type:               mcpv1.MCPServerConditionReconciled,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: now,
		Reason:             "ReconcileSuccess",
		Message:            "MCPServer has been successfully reconciled",
	}
	r.setCondition(mcpServer, reconciledCondition)
}

// setCondition sets or updates a condition in the MCPServer status
func (r *MCPServerReconciler) setCondition(mcpServer *mcpv1.MCPServer, newCondition mcpv1.MCPServerCondition) {
	for i, condition := range mcpServer.Status.Conditions {
		if condition.Type == newCondition.Type {
			if condition.Status != newCondition.Status || condition.Reason != newCondition.Reason || condition.Message != newCondition.Message {
				mcpServer.Status.Conditions[i] = newCondition
			}
			return
		}
	}
	mcpServer.Status.Conditions = append(mcpServer.Status.Conditions, newCondition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ServiceAccount{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Named("mcpserver").
		Complete(r)
}
