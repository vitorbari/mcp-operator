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
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
	"github.com/vitorbari/mcp-operator/internal/metrics"
	"github.com/vitorbari/mcp-operator/pkg/transport"
)

// MCPServerReconciler reconciles a MCPServer object
type MCPServerReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	TransportFactory *transport.ManagerFactory
	Recorder         record.EventRecorder
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
// +kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

const (
	mcpServerFinalizer = "mcp.mcp-operator.io/finalizer"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx).WithValues("mcpserver", req.NamespacedName)
	startTime := time.Now()

	// Fetch the MCPServer instance
	mcpServer := &mcpv1.MCPServer{}
	if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
		if errors.IsNotFound(err) {
			log.Info("MCPServer resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get MCPServer")
		metrics.RecordReconcileMetrics("mcpserver", time.Since(startTime).Seconds(), "error")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if mcpServer.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, mcpServer)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(mcpServer, mcpServerFinalizer) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			// Get the latest version
			latest := &mcpv1.MCPServer{}
			if err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, latest); err != nil {
				return err
			}
			// Add finalizer to latest version
			controllerutil.AddFinalizer(latest, mcpServerFinalizer)
			// Update with latest version
			return r.Update(ctx, latest)
		})
		if err != nil {
			log.Error(err, "Failed to add finalizer")
			metrics.RecordReconcileMetrics("mcpserver", time.Since(startTime).Seconds(), "error")
			return ctrl.Result{}, err
		}
		// Re-fetch after update to get latest resourceVersion
		if err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, mcpServer); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Update status phase to Creating if it's empty
	if mcpServer.Status.Phase == "" {
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseCreating
		mcpServer.Status.Message = "Starting MCPServer deployment"
		r.Recorder.Event(mcpServer, corev1.EventTypeNormal, "Creating", "Starting MCPServer deployment")
		if err := r.updateStatus(ctx, mcpServer); err != nil {
			metrics.RecordReconcileMetrics("mcpserver", time.Since(startTime).Seconds(), "error")
			return ctrl.Result{}, err
		}
	}

	// Reconcile ServiceAccount
	if err := r.reconcileServiceAccount(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to reconcile ServiceAccount")
		r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "ServiceAccountFailed", fmt.Sprintf("Failed to reconcile ServiceAccount: %v", err))
		return r.updateStatusWithError(ctx, mcpServer, err)
	}

	// Reconcile RBAC if security settings are provided
	if mcpServer.Spec.Security != nil {
		if err := r.reconcileRBAC(ctx, mcpServer); err != nil {
			log.Error(err, "Failed to reconcile RBAC")
			r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "RBACFailed", fmt.Sprintf("Failed to reconcile RBAC: %v", err))
			return r.updateStatusWithError(ctx, mcpServer, err)
		}
	}

	// Reconcile transport-specific resources
	if err := r.reconcileTransportResources(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to reconcile transport resources")
		r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "TransportResourcesFailed", fmt.Sprintf("Failed to reconcile transport resources: %v", err))
		return r.updateStatusWithError(ctx, mcpServer, err)
	}

	// Reconcile HPA if enabled
	if err := r.reconcileHPA(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to reconcile HPA")
		r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "HPAFailed", fmt.Sprintf("Failed to reconcile HPA: %v", err))
		return r.updateStatusWithError(ctx, mcpServer, err)
	}

	// Reconcile Ingress if enabled
	if err := r.reconcileIngress(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to reconcile Ingress")
		r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "IngressFailed", fmt.Sprintf("Failed to reconcile Ingress: %v", err))
		return r.updateStatusWithError(ctx, mcpServer, err)
	}

	// Update status based on deployment status
	statusChanged := false
	if err := r.updateMCPServerStatus(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to update MCPServer status")
		metrics.RecordReconcileMetrics("mcpserver", time.Since(startTime).Seconds(), "error")
		return ctrl.Result{}, err
	} else {
		// Check if this was the first successful reconciliation
		if mcpServer.Status.ObservedGeneration != mcpServer.Generation {
			statusChanged = true
		}
	}

	// Record reconciliation metrics
	metrics.RecordReconcileMetrics("mcpserver", time.Since(startTime).Seconds(), "success")

	// Only log and emit events for significant changes to reduce noise
	if statusChanged {
		log.Info("Successfully reconciled MCPServer")
		r.Recorder.Event(mcpServer, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled MCPServer")
	}

	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// handleDeletion handles the deletion of MCPServer resources
func (r *MCPServerReconciler) handleDeletion(ctx context.Context, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Delete MCPServer metrics
	metrics.DeleteMCPServerMetrics(mcpServer)

	// Update status to Terminating
	mcpServer.Status.Phase = mcpv1.MCPServerPhaseTerminating
	mcpServer.Status.Message = "Terminating MCPServer resources"
	r.Recorder.Event(mcpServer, corev1.EventTypeNormal, "Terminating", "Terminating MCPServer resources")
	if err := r.updateStatus(ctx, mcpServer); err != nil {
		// Ignore NotFound errors during deletion - object may have been deleted already
		if !errors.IsNotFound(err) {
			log.Error(err, "Failed to update status during deletion")
		}
	}

	// Remove finalizer to allow deletion with retry
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest version
		latest := &mcpv1.MCPServer{}
		if err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, latest); err != nil {
			return err
		}
		// Remove finalizer from latest version
		controllerutil.RemoveFinalizer(latest, mcpServerFinalizer)
		// Update with latest version
		return r.Update(ctx, latest)
	})
	if err != nil {
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

// reconcileTransportResources handles transport-specific resource reconciliation
func (r *MCPServerReconciler) reconcileTransportResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Get the appropriate transport manager
	manager, err := r.TransportFactory.GetManagerForMCPServer(mcpServer)
	if err != nil {
		return err
	}

	// Use retry logic for resource conflicts
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Create resources (this should be idempotent)
		if err := manager.CreateResources(ctx, mcpServer); err != nil {
			return err
		}

		// Update resources (this handles changes)
		return manager.UpdateResources(ctx, mcpServer)
	})
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

	// Use CreateOrUpdate with retry logic to handle both creation and updates idempotently
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		hpa := r.buildHPA(mcpServer)

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, hpa, func() error {
			// Set controller reference
			if err := controllerutil.SetControllerReference(mcpServer, hpa, r.Scheme); err != nil {
				return err
			}

			// Update the spec (this will be used for both create and update)
			desiredSpec := r.buildHPA(mcpServer).Spec
			hpa.Spec = desiredSpec

			return nil
		})

		return err
	})
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
	var metricSpecs []autoscalingv2.MetricSpec

	// CPU metric
	if mcpServer.Spec.HPA.TargetCPUUtilizationPercentage != nil {
		metricSpecs = append(metricSpecs, autoscalingv2.MetricSpec{
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
		metricSpecs = append(metricSpecs, autoscalingv2.MetricSpec{
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
		Metrics:     metricSpecs,
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

// reconcileIngress ensures the Ingress exists if enabled
func (r *MCPServerReconciler) reconcileIngress(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Check if ingress is enabled and transport supports it
	if mcpServer.Spec.Ingress == nil || mcpServer.Spec.Ingress.Enabled == nil || !*mcpServer.Spec.Ingress.Enabled {
		// If Ingress is not enabled, delete any existing Ingress
		existingIngress := &networkingv1.Ingress{}
		err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, existingIngress)
		if err == nil {
			// Ingress exists, delete it
			if err := r.Delete(ctx, existingIngress); err != nil {
				return err
			}
		} else if !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	// Check if transport supports ingress
	if mcpServer.Spec.Transport != nil {
		manager, err := r.TransportFactory.GetManagerForMCPServer(mcpServer)
		if err != nil {
			return err
		}
		if !manager.RequiresIngress() {
			// Transport doesn't support ingress, skip creation
			return nil
		}
	}

	ingress := r.buildIngress(mcpServer)

	if err := controllerutil.SetControllerReference(mcpServer, ingress, r.Scheme); err != nil {
		return err
	}

	found := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: ingress.Name, Namespace: ingress.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		if err := r.Create(ctx, ingress); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update ingress if necessary
		if !reflect.DeepEqual(found.Spec, ingress.Spec) {
			found.Spec = ingress.Spec
			found.Annotations = ingress.Annotations
			if err := r.Update(ctx, found); err != nil {
				return err
			}
		}
	}

	return nil
}

// buildIngress creates an Ingress object for the MCPServer
func (r *MCPServerReconciler) buildIngress(mcpServer *mcpv1.MCPServer) *networkingv1.Ingress {
	labels := map[string]string{
		"app":                          mcpServer.Name,
		"app.kubernetes.io/name":       "mcpserver",
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/component":  "mcp-server",
		"app.kubernetes.io/managed-by": "mcp-operator",
	}

	// Start with base annotations for streaming and metrics
	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-read-timeout":    "3600",
		"nginx.ingress.kubernetes.io/proxy-send-timeout":    "3600",
		"nginx.ingress.kubernetes.io/proxy-connect-timeout": "60",
		"nginx.ingress.kubernetes.io/proxy-buffering":       "off",
		// Enable metrics collection by default
		"nginx.ingress.kubernetes.io/enable-metrics": "true",
		"nginx.org/prometheus-metrics":               "true",
		"nginx.org/prometheus-port":                  "9113",
	}

	// Get transport type for headers and logging
	transportType := r.getMCPTransportType(mcpServer)

	// Add advanced server snippet with MCP transport headers and analytics
	serverSnippet := fmt.Sprintf(`
		# Advanced logging for MCP analytics
		access_log /var/log/nginx/%s-access.log json_combined;

		# Add MCP transport type header for all requests
		more_set_headers "X-MCP-Transport: %s";
		more_set_headers "X-MCP-Server: %s";

		# Location-specific configurations
		location /mcp {
			proxy_set_header X-MCP-Protocol-Version $http_mcp_protocol_version;
			proxy_set_header X-MCP-Session-ID $http_mcp_session_id;
			proxy_pass $target;
		}
	`, mcpServer.Name, transportType, mcpServer.Name)

	annotations["nginx.ingress.kubernetes.io/server-snippet"] = serverSnippet

	// Add transport-specific annotations
	if mcpServer.Spec.Transport != nil {
		switch mcpServer.Spec.Transport.Type {
		case mcpv1.MCPTransportHTTP:
			// Add session affinity for HTTP transport if session management is enabled
			if mcpServer.Spec.Transport.Config != nil &&
				mcpServer.Spec.Transport.Config.HTTP != nil &&
				mcpServer.Spec.Transport.Config.HTTP.SessionManagement != nil &&
				*mcpServer.Spec.Transport.Config.HTTP.SessionManagement {
				annotations["nginx.ingress.kubernetes.io/affinity"] = "cookie"
				annotations["nginx.ingress.kubernetes.io/session-cookie-name"] = "mcp-session"
				annotations["nginx.ingress.kubernetes.io/session-cookie-expires"] = "86400"
				// Use session ID for consistent routing
				annotations["nginx.ingress.kubernetes.io/upstream-hash-by"] = "$http_mcp_session_id"
			}
		}
	}

	// Merge custom annotations
	if mcpServer.Spec.Ingress.Annotations != nil {
		for k, v := range mcpServer.Spec.Ingress.Annotations {
			annotations[k] = v
		}
	}

	// Default values
	path := "/"
	if mcpServer.Spec.Ingress.Path != "" {
		path = mcpServer.Spec.Ingress.Path
	}

	pathType := networkingv1.PathTypePrefix
	if mcpServer.Spec.Ingress.PathType != nil {
		pathType = *mcpServer.Spec.Ingress.PathType
	}

	// Get service port
	servicePort := int32(8080)
	if mcpServer.Spec.Service != nil && mcpServer.Spec.Service.Port != 0 {
		servicePort = mcpServer.Spec.Service.Port
	}

	// Build ingress spec
	ingressSpec := networkingv1.IngressSpec{
		Rules: []networkingv1.IngressRule{
			{
				Host: mcpServer.Spec.Ingress.Host,
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{
							{
								Path:     path,
								PathType: &pathType,
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: mcpServer.Name,
										Port: networkingv1.ServiceBackendPort{
											Number: servicePort,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Add TLS if specified
	if len(mcpServer.Spec.Ingress.TLS) > 0 {
		ingressSpec.TLS = mcpServer.Spec.Ingress.TLS
	}

	// Add ingress class if specified
	if mcpServer.Spec.Ingress.ClassName != nil {
		ingressSpec.IngressClassName = mcpServer.Spec.Ingress.ClassName
	}

	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:        mcpServer.Name,
			Namespace:   mcpServer.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: ingressSpec,
	}
}

// updateStatus updates the MCPServer status with retry logic for conflicts
func (r *MCPServerReconciler) updateStatus(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	mcpServer.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
	mcpServer.Status.ObservedGeneration = mcpServer.Generation

	// Retry logic for optimistic concurrency conflicts
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Re-fetch the MCPServer to get the latest resource version
		latest := &mcpv1.MCPServer{}
		if err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, latest); err != nil {
			return err
		}

		// Copy the status to the latest version
		latest.Status = mcpServer.Status
		latest.Status.LastReconcileTime = &metav1.Time{Time: time.Now()}
		latest.Status.ObservedGeneration = latest.Generation

		return r.Status().Update(ctx, latest)
	})
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

	// Update metrics with current MCPServer state
	metrics.UpdateMCPServerMetrics(mcpServer)

	if updateErr := r.updateStatus(ctx, mcpServer); updateErr != nil {
		return ctrl.Result{}, updateErr
	}
	return ctrl.Result{RequeueAfter: time.Minute * 2}, err
}

// updateMCPServerStatus updates the MCPServer status based on deployment status
func (r *MCPServerReconciler) updateMCPServerStatus(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	// Note: updateStatus() will handle re-fetching for conflict resolution

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

	// Update transport type in status
	transportType := transport.GetDefaultTransportType()
	if mcpServer.Spec.Transport != nil {
		transportType = mcpServer.Spec.Transport.Type
	}
	mcpServer.Status.TransportType = transportType

	// Determine phase based on deployment status
	previousPhase := mcpServer.Status.Phase
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

	// Emit events for phase transitions
	if previousPhase != mcpServer.Status.Phase {
		switch mcpServer.Status.Phase {
		case mcpv1.MCPServerPhaseRunning:
			r.Recorder.Event(mcpServer, corev1.EventTypeNormal, "Ready", "MCPServer is ready and running")
		case mcpv1.MCPServerPhaseScaling:
			r.Recorder.Event(mcpServer, corev1.EventTypeNormal, "Scaling", "MCPServer is scaling")
		case mcpv1.MCPServerPhaseUpdating:
			r.Recorder.Event(mcpServer, corev1.EventTypeNormal, "Updating", "MCPServer is rolling out update")
		case mcpv1.MCPServerPhaseCreating:
			if previousPhase == mcpv1.MCPServerPhaseRunning {
				r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "NotReady", "MCPServer is no longer ready")
			}
		}
	}

	// Set service endpoint
	service := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, service); err == nil {
		port := transport.GetTransportPort(mcpServer)
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

	// Capture the current status for comparison
	originalStatus := mcpServer.Status.DeepCopy()

	// Update conditions
	r.updateConditions(mcpServer, deployment)

	// Update metrics
	metrics.UpdateMCPServerMetrics(mcpServer)

	// Only update status if there are actual changes
	if !reflect.DeepEqual(originalStatus, &mcpServer.Status) {
		return r.updateStatus(ctx, mcpServer)
	}

	return nil
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
			// Only update LastTransitionTime when the condition actually changes
			// If condition content is the same, preserve the existing timestamp
			// This prevents unnecessary status updates that trigger reconciliation loops
			if condition.Status != newCondition.Status || condition.Reason != newCondition.Reason || condition.Message != newCondition.Message {
				mcpServer.Status.Conditions[i] = newCondition
			}
			return
		}
	}
	mcpServer.Status.Conditions = append(mcpServer.Status.Conditions, newCondition)
}

// getMCPTransportType returns the transport type string for headers and logging
func (r *MCPServerReconciler) getMCPTransportType(mcpServer *mcpv1.MCPServer) string {
	if mcpServer.Spec.Transport != nil {
		return string(mcpServer.Spec.Transport.Type)
	}
	return "http" // default transport type
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
		Owns(&networkingv1.Ingress{}).
		Named("mcpserver").
		Complete(r)
}
