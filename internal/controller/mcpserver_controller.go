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
	"os"
	"reflect"
	"strconv"
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
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mcpv1 "github.com/vitorbari/mcp-operator/api/v1"
	"github.com/vitorbari/mcp-operator/internal/metrics"
	"github.com/vitorbari/mcp-operator/internal/transport"
	"github.com/vitorbari/mcp-operator/pkg/validator"
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

var (
	// Maximum validation attempts before giving up (configurable via MCP_MAX_VALIDATION_ATTEMPTS env var)
	maxValidationAttempts = getEnvAsInt32("MCP_MAX_VALIDATION_ATTEMPTS", 5)
	// Maximum attempts for permanent errors (configurable via MCP_MAX_PERMANENT_ERROR_ATTEMPTS env var)
	maxPermanentErrorAttempts = getEnvAsInt32("MCP_MAX_PERMANENT_ERROR_ATTEMPTS", 2)
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
//nolint:gocyclo // Main reconciliation loop naturally has complexity; further extraction would harm readability
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

	// Check for recovery scenario: spec changed after validation failure
	// This allows users to fix their configuration and retry
	if r.shouldResetValidationForRecovery(mcpServer) {
		log.Info("Spec changed after validation failure, resetting for recovery",
			"currentGeneration", mcpServer.Generation,
			"validatedGeneration", mcpServer.Status.Validation.ValidatedGeneration,
			"previousPhase", mcpServer.Status.Phase)

		// Reset validation state for fresh start
		mcpServer.Status.Validation.State = mcpv1.ValidationStatePending
		mcpServer.Status.Validation.Attempts = 0
		mcpServer.Status.Validation.Issues = nil
		mcpServer.Status.Validation.LastAttemptTime = nil

		// Reset phase to allow deployment recreation
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseCreating
		mcpServer.Status.Message = "Retrying deployment after configuration fix"

		// Clear Degraded condition
		now := metav1.Now()
		degradedCondition := mcpv1.MCPServerCondition{
			Type:               mcpv1.MCPServerConditionDegraded,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Reason:             "RecoveryAttempt",
			Message:            "Retrying after configuration change",
		}
		r.setCondition(mcpServer, degradedCondition)

		r.Recorder.Event(mcpServer, corev1.EventTypeNormal, "ValidationRecovery",
			"Configuration changed after validation failure, retrying deployment")

		if err := r.updateStatus(ctx, mcpServer); err != nil {
			log.Error(err, "Failed to update status for validation recovery")
			metrics.RecordReconcileMetrics("mcpserver", time.Since(startTime).Seconds(), "error")
			return ctrl.Result{}, err
		}
	}

	// Check if validation has failed terminally in strict mode
	// If so, skip resource reconciliation and maintain ValidationFailed phase
	if r.isInTerminalValidationFailure(mcpServer) {
		log.Info("Validation failed terminally in strict mode, skipping resource reconciliation",
			"phase", mcpServer.Status.Phase,
			"attempts", mcpServer.Status.Validation.Attempts,
			"state", mcpServer.Status.Validation.State)

		// Track if status needs updating
		needsStatusUpdate := false

		// Ensure phase is set to ValidationFailed
		if mcpServer.Status.Phase != mcpv1.MCPServerPhaseValidationFailed {
			mcpServer.Status.Phase = mcpv1.MCPServerPhaseValidationFailed
			mcpServer.Status.Message = fmt.Sprintf("Validation failed after %d attempts: %s",
				mcpServer.Status.Validation.Attempts,
				getValidationFailureReason(mcpServer.Status.Validation))
			needsStatusUpdate = true
		}

		// Clear replica counts if they're not already zero (deployment is deleted)
		if mcpServer.Status.Replicas != 0 || mcpServer.Status.ReadyReplicas != 0 || mcpServer.Status.AvailableReplicas != 0 {
			mcpServer.Status.Replicas = 0
			mcpServer.Status.ReadyReplicas = 0
			mcpServer.Status.AvailableReplicas = 0
			needsStatusUpdate = true

			// Update conditions to reflect that deployment has been deleted
			now := metav1.Now()

			// Set Ready condition to False
			readyCondition := mcpv1.MCPServerCondition{
				Type:               mcpv1.MCPServerConditionReady,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: now,
				Reason:             "DeploymentDeleted",
				Message:            "Deployment deleted after validation failure in strict mode",
			}
			r.setCondition(mcpServer, readyCondition)

			// Set Available condition to False
			availableCondition := mcpv1.MCPServerCondition{
				Type:               mcpv1.MCPServerConditionAvailable,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: now,
				Reason:             "DeploymentDeleted",
				Message:            "Deployment deleted after validation failure in strict mode",
			}
			r.setCondition(mcpServer, availableCondition)

			// Set Progressing condition to False
			progressingCondition := mcpv1.MCPServerCondition{
				Type:               mcpv1.MCPServerConditionProgressing,
				Status:             corev1.ConditionFalse,
				LastTransitionTime: now,
				Reason:             "DeploymentDeleted",
				Message:            "Deployment deleted after validation failure in strict mode",
			}
			r.setCondition(mcpServer, progressingCondition)
		}

		// Update status if needed
		if needsStatusUpdate {
			if err := r.updateStatus(ctx, mcpServer); err != nil {
				log.Error(err, "Failed to update status for terminal validation failure")
				metrics.RecordReconcileMetrics("mcpserver", time.Since(startTime).Seconds(), "error")
				return ctrl.Result{}, err
			}
		}

		// Ensure deployment is deleted (idempotent)
		if err := r.deleteDeploymentForValidationFailure(ctx, mcpServer); err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "Failed to ensure deployment is deleted in terminal validation failure")
			}
		}

		// Record metrics and return without requeue (terminal state)
		metrics.RecordReconcileMetrics("mcpserver", time.Since(startTime).Seconds(), "success")
		log.Info("MCPServer in terminal validation failure state, no further action until spec changes")
		return ctrl.Result{}, nil
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

	// Perform protocol validation if enabled and server is running
	// Validation occurs on deployment and retries with backoff if it fails
	// Strict mode enforcement (deployment deletion) is handled in updateValidationStatus
	if r.shouldValidate(ctx, mcpServer) {
		validationResult := r.validateServer(ctx, mcpServer)
		if validationResult != nil {
			// Update validation status (this also handles strict mode enforcement)
			if err := r.updateValidationStatus(ctx, mcpServer, validationResult); err != nil {
				log.Error(err, "Failed to update validation status")
			} else {
				// Update metrics after validation status is successfully updated
				// This ensures validation-related gauges (compliant, capabilities, protocol_version) reflect the latest validation
				metrics.UpdateMCPServerMetrics(mcpServer)
			}
		}
	}

	// Calculate retry interval for failed validations (returns 0 if validation succeeded)
	requeueAfter := r.getValidationRetryInterval(mcpServer)

	// Record reconciliation metrics
	metrics.RecordReconcileMetrics("mcpserver", time.Since(startTime).Seconds(), "success")

	// Only log and emit events for significant changes to reduce noise
	if statusChanged {
		log.Info("Successfully reconciled MCPServer")
		r.Recorder.Event(mcpServer, corev1.EventTypeNormal, "Reconciled", "Successfully reconciled MCPServer")
	}

	// Requeue if validation failed or hasn't been attempted yet
	if requeueAfter > 0 {
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	return ctrl.Result{}, nil
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

	// Start with sensible defaults for long-lived connections
	annotations := map[string]string{
		"nginx.ingress.kubernetes.io/proxy-read-timeout":    "3600",
		"nginx.ingress.kubernetes.io/proxy-send-timeout":    "3600",
		"nginx.ingress.kubernetes.io/proxy-connect-timeout": "60",
		"nginx.ingress.kubernetes.io/proxy-buffering":       "off",
	}

	// Add session affinity if session management is enabled
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.HTTP != nil &&
		mcpServer.Spec.Transport.Config.HTTP.SessionManagement != nil &&
		*mcpServer.Spec.Transport.Config.HTTP.SessionManagement {
		annotations["nginx.ingress.kubernetes.io/affinity"] = "cookie"
		annotations["nginx.ingress.kubernetes.io/session-cookie-name"] = "mcp-session"
	}

	// Merge custom annotations (user annotations take precedence)
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

// isInTerminalValidationFailure checks if the MCPServer is in a terminal validation failure state
// where resource reconciliation should be blocked until the spec is changed
func (r *MCPServerReconciler) isInTerminalValidationFailure(mcpServer *mcpv1.MCPServer) bool {
	// No terminal failure if validation hasn't run
	if mcpServer.Status.Validation == nil {
		return false
	}

	// Not in terminal failure if strict mode is not enabled
	if !r.isStrictModeEnabled(mcpServer) {
		return false
	}

	// Check if validation state is Failed
	if mcpServer.Status.Validation.State != mcpv1.ValidationStateFailed {
		return false
	}

	// Check if we've reached max attempts (ensures we don't block prematurely)
	if mcpServer.Status.Validation.Attempts < maxPermanentErrorAttempts {
		// For permanent errors, need at least 2 attempts
		// For transient errors, need at least 5 attempts
		// If less than minimum permanent error attempts, not terminal yet
		return false
	}

	// In terminal failure if:
	// 1. Strict mode enabled
	// 2. Validation state is Failed
	// 3. Attempts >= threshold
	return true
}

// shouldResetValidationForRecovery checks if we should reset validation state
// to allow users to recover from validation failures by fixing their spec
func (r *MCPServerReconciler) shouldResetValidationForRecovery(mcpServer *mcpv1.MCPServer) bool {
	// No recovery needed if validation hasn't run yet
	if mcpServer.Status.Validation == nil {
		return false
	}

	// Check if generation changed (user edited spec)
	if mcpServer.Status.Validation.ValidatedGeneration == mcpServer.Generation {
		return false // No spec change
	}

	// Reset if validation failed (either phase or state indicates failure)
	if mcpServer.Status.Phase == mcpv1.MCPServerPhaseValidationFailed ||
		mcpServer.Status.Validation.State == mcpv1.ValidationStateFailed {
		return true
	}

	// Also reset if currently in Degraded state (non-strict mode failure)
	// This allows recovery from non-compliant state
	if !mcpServer.Status.Validation.Compliant {
		return true
	}

	return false
}

// shouldValidate determines if protocol validation should be performed
func (r *MCPServerReconciler) shouldValidate(ctx context.Context, mcpServer *mcpv1.MCPServer) bool {
	// Check if validation is enabled
	if mcpServer.Spec.Validation == nil {
		return false
	}

	// Check if enabled is explicitly set to false
	if mcpServer.Spec.Validation.Enabled != nil && !*mcpServer.Spec.Validation.Enabled {
		return false
	}

	// Only validate if server is in Running phase
	if mcpServer.Status.Phase != mcpv1.MCPServerPhaseRunning {
		return false
	}

	// Don't validate until pods are ready (prevents false positives during startup)
	if !r.arePodsReady(ctx, mcpServer) {
		return false
	}

	// Validate if we haven't validated yet (first attempt)
	if mcpServer.Status.Validation == nil {
		return true
	}

	// Re-validate if spec changed (generation changed since last validation)
	// This ensures validation runs after image updates, transport config changes, etc.
	// Reset state to pending on spec changes
	if mcpServer.Status.Validation.ValidatedGeneration != mcpServer.Generation {
		return true
	}

	// Continue validation if in Validating state (retrying after transient error)
	if mcpServer.Status.Validation.State == mcpv1.ValidationStateValidating {
		return true
	}

	// Don't re-validate if validation passed (no periodic validation)
	if mcpServer.Status.Validation.State == mcpv1.ValidationStatePassed {
		return false
	}

	// Don't re-validate if validation failed permanently
	if mcpServer.Status.Validation.State == mcpv1.ValidationStateFailed {
		return false
	}

	// Default: don't validate
	return false
}

// arePodsReady checks if the deployment has at least one ready replica
// This prevents false positives during pod startup
func (r *MCPServerReconciler) arePodsReady(ctx context.Context, mcpServer *mcpv1.MCPServer) bool {
	log := logf.FromContext(ctx)

	// Check if deployment exists and has ready replicas
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      mcpServer.Name,
		Namespace: mcpServer.Namespace,
	}, deployment)

	if err != nil {
		log.V(1).Info("Deployment not found, pods not ready", "error", err)
		return false
	}

	// Check if we have at least one ready replica
	if deployment.Status.ReadyReplicas < 1 {
		log.V(1).Info("Waiting for pods to be ready before validation",
			"readyReplicas", deployment.Status.ReadyReplicas,
			"desiredReplicas", deployment.Status.Replicas)
		return false
	}

	log.V(1).Info("Pods are ready for validation",
		"readyReplicas", deployment.Status.ReadyReplicas)
	return true
}

// isPermanentError classifies validation errors as permanent or transient
// Permanent errors should trigger fast-fail behavior (after 2 attempts)
// Transient errors should be retried with exponential backoff
func (r *MCPServerReconciler) isPermanentError(result *validator.ValidationResult, hasMismatch bool) bool {
	// Protocol mismatch is always a permanent configuration error
	if hasMismatch {
		return true
	}

	// Check for permanent error codes in validation issues
	for _, issue := range result.Issues {
		switch issue.Code {
		case validator.CodeProtocolMismatch:
			// Protocol mismatch - user needs to fix configuration
			return true
		case validator.CodeAuthRequired:
			// Auth required - user needs to provide credentials
			return true
		case validator.CodeInvalidProtocolVersion:
			// Invalid protocol version - server incompatibility
			return true
		case validator.CodeMissingServerInfo:
			// Missing server info after successful connection - protocol issue
			if result.Success {
				return true
			}
		}
	}

	// If validation succeeded but IsCompliant is false due to missing capabilities,
	// this is a permanent error (server doesn't support required features)
	if !result.Success && result.ProtocolVersion != "" && len(result.Capabilities) > 0 {
		// Server responded successfully but doesn't meet requirements
		for _, issue := range result.Issues {
			if issue.Code == validator.CodeMissingCapability {
				return true
			}
		}
	}

	// All other errors are considered transient (connection issues, timeouts, etc.)
	return false
}

// deleteDeploymentForValidationFailure deletes the deployment when validation fails in strict mode
func (r *MCPServerReconciler) deleteDeploymentForValidationFailure(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	log := logf.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      mcpServer.Name,
		Namespace: mcpServer.Namespace,
	}, deployment)

	if err != nil {
		if errors.IsNotFound(err) {
			log.V(1).Info("Deployment already deleted")
			return nil // Already deleted
		}
		return err
	}

	log.Info("Deleting deployment due to validation failure in strict mode",
		"deployment", deployment.Name,
		"attempts", mcpServer.Status.Validation.Attempts)

	return r.Delete(ctx, deployment)
}

// getValidationFailureReason returns a human-readable reason for validation failure
func getValidationFailureReason(validation *mcpv1.ValidationStatus) string {
	if validation == nil || len(validation.Issues) == 0 {
		return "Unknown validation failure"
	}

	// Return first error-level issue
	for _, issue := range validation.Issues {
		if issue.Level == validator.LevelError {
			return issue.Message
		}
	}

	// If no errors, return first issue of any level
	return validation.Issues[0].Message
}

// validateServer performs MCP protocol validation on the server
func (r *MCPServerReconciler) validateServer(ctx context.Context, mcpServer *mcpv1.MCPServer) *validator.ValidationResult {
	log := logf.FromContext(ctx)

	// Build the endpoint URL from the service
	endpoint := r.buildValidationEndpoint(mcpServer)
	if endpoint == "" {
		log.Info("Cannot determine validation endpoint, skipping validation")
		return nil
	}

	// Create validator with a fixed timeout
	timeout := 30 * time.Second
	v := validator.NewValidatorWithTimeout(endpoint, timeout)

	// Prepare validation options
	opts := validator.ValidationOptions{
		Timeout: timeout,
	}

	// Add configured path if specified
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.HTTP != nil &&
		mcpServer.Spec.Transport.Config.HTTP.Path != "" {
		opts.ConfiguredPath = mcpServer.Spec.Transport.Config.HTTP.Path
	}

	// Add configured protocol if specified (for protocol mismatch detection)
	// When protocol is not "auto", we still use auto-detection but can compare results
	// Note: We always use auto-detection to discover what the server actually supports,
	// then compare with the configured protocol to detect mismatches
	if mcpServer.Spec.Transport != nil && mcpServer.Spec.Transport.Protocol != "" {
		// Don't pass the protocol to the validator - let it auto-detect
		// We'll compare the detected protocol with the configured one in checkProtocolMismatch
		// This approach allows us to detect what the server actually implements
	}

	// Add required capabilities if specified
	if mcpServer.Spec.Validation != nil && len(mcpServer.Spec.Validation.RequiredCapabilities) > 0 {
		opts.RequiredCapabilities = mcpServer.Spec.Validation.RequiredCapabilities
	}

	// Add strict mode if specified
	if r.isStrictModeEnabled(mcpServer) {
		opts.StrictMode = true
	}

	// Perform validation
	result, err := v.Validate(ctx, opts)
	if err != nil {
		log.Error(err, "Validation call failed")
		// Record failure metrics
		metrics.RecordValidationMetrics(mcpServer, 0, false)
		return nil
	}

	// Record validation metrics (duration is already tracked in result)
	if result != nil {
		metrics.RecordValidationMetrics(mcpServer, result.Duration.Seconds(), result.IsCompliant())
	}

	return result
}

// buildValidationEndpoint constructs the base URL for validation
// Returns base URL without path (e.g., "http://service:8080") to allow auto-detection
func (r *MCPServerReconciler) buildValidationEndpoint(mcpServer *mcpv1.MCPServer) string {
	// Use the internal service endpoint for validation
	serviceName := mcpServer.Name
	namespace := mcpServer.Namespace

	// Determine the port
	port := int32(8080)
	if mcpServer.Spec.Service != nil && mcpServer.Spec.Service.Port != 0 {
		port = mcpServer.Spec.Service.Port
	}
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.HTTP != nil &&
		mcpServer.Spec.Transport.Config.HTTP.Port != 0 {
		port = mcpServer.Spec.Transport.Config.HTTP.Port
	}

	// Build the base URL using the internal service DNS name
	// Path will be auto-detected by the validator
	return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", serviceName, namespace, port)
}

// updateValidationStatus updates the validation status in the MCPServer
func (r *MCPServerReconciler) updateValidationStatus(ctx context.Context, mcpServer *mcpv1.MCPServer, result *validator.ValidationResult) error {
	log := logf.FromContext(ctx)

	now := metav1.Now()

	// Check for protocol mismatch and preserve any mismatch issues
	hasMismatch := r.checkProtocolMismatch(ctx, mcpServer, result)
	var mismatchIssues []mcpv1.ValidationIssue
	if hasMismatch && mcpServer.Status.Validation != nil {
		// Preserve PROTOCOL_MISMATCH issues added by checkProtocolMismatch
		for _, issue := range mcpServer.Status.Validation.Issues {
			if issue.Code == validator.CodeProtocolMismatch {
				mismatchIssues = append(mismatchIssues, issue)
			}
		}
	}

	// Track validation attempts
	attempts := int32(1)
	if mcpServer.Status.Validation != nil {
		attempts = mcpServer.Status.Validation.Attempts + 1
	}

	// Determine validation state based on result and error classification
	var state mcpv1.ValidationState
	isCompliant := result.IsCompliant() && !hasMismatch

	if isCompliant {
		// Validation succeeded
		state = mcpv1.ValidationStatePassed
	} else {
		// Validation failed - determine if it's permanent or transient
		if r.isPermanentError(result, hasMismatch) {
			// Permanent error detected - fail fast
			if attempts >= maxPermanentErrorAttempts {
				// After maxPermanentErrorAttempts with permanent error, mark as Failed
				state = mcpv1.ValidationStateFailed
			} else {
				// First attempt with permanent error - give one more chance
				state = mcpv1.ValidationStateValidating
			}
		} else {
			// Transient error - keep trying with backoff up to maxValidationAttempts
			if attempts >= maxValidationAttempts {
				// After maxValidationAttempts even for transient errors, give up
				state = mcpv1.ValidationStateFailed
			} else {
				state = mcpv1.ValidationStateValidating
			}
		}
	}

	// Convert validator result to CRD status
	validationStatus := &mcpv1.ValidationStatus{
		State:               state,
		Attempts:            attempts,
		LastAttemptTime:     &now,
		ProtocolVersion:     result.ProtocolVersion,
		Capabilities:        result.Capabilities,
		Compliant:           isCompliant,
		LastValidated:       &now,
		TransportUsed:       string(result.DetectedTransport),
		RequiresAuth:        result.RequiresAuth,
		ValidatedGeneration: mcpServer.Generation,
		Issues:              make([]mcpv1.ValidationIssue, 0, len(result.Issues)+len(mismatchIssues)),
	}

	// Add protocol mismatch issues first (these were added by checkProtocolMismatch)
	validationStatus.Issues = append(validationStatus.Issues, mismatchIssues...)

	// Convert issues from validation result
	for _, issue := range result.Issues {
		validationStatus.Issues = append(validationStatus.Issues, mcpv1.ValidationIssue{
			Level:   issue.Level,
			Message: issue.Message,
			Code:    issue.Code,
		})
	}

	// Update the validation status
	mcpServer.Status.Validation = validationStatus

	// Update transport status
	if mcpServer.Status.Transport == nil {
		mcpServer.Status.Transport = &mcpv1.TransportStatus{}
	}
	mcpServer.Status.Transport.DetectedProtocol = string(result.DetectedTransport)
	mcpServer.Status.Transport.Endpoint = result.Endpoint
	mcpServer.Status.Transport.LastDetected = &now

	// Determine session support based on spec configuration
	sessionSupport := false
	if mcpServer.Spec.Transport != nil &&
		mcpServer.Spec.Transport.Config != nil &&
		mcpServer.Spec.Transport.Config.HTTP != nil &&
		mcpServer.Spec.Transport.Config.HTTP.SessionManagement != nil {
		sessionSupport = *mcpServer.Spec.Transport.Config.HTTP.SessionManagement
	}
	mcpServer.Status.Transport.SessionSupport = sessionSupport

	// Update conditions based on validation result
	r.updateValidationConditions(mcpServer, result, hasMismatch)

	// Update status with retry logic
	if err := r.updateStatus(ctx, mcpServer); err != nil {
		log.Error(err, "Failed to update validation status")
		return err
	}

	// Emit events based on validation state and attempts
	switch state {
	case mcpv1.ValidationStatePassed:
		authStatus := "no auth"
		if result.RequiresAuth {
			authStatus = "requires auth"
		}
		r.Recorder.Event(mcpServer, corev1.EventTypeNormal, "ValidationPassed",
			fmt.Sprintf("MCP protocol validation succeeded (transport: %s, %s)", result.DetectedTransport, authStatus))
	case mcpv1.ValidationStateValidating:
		// Validation is still in progress (transient error or first permanent error attempt)
		isPermanent := r.isPermanentError(result, hasMismatch)
		errorType := "transient"
		maxAttempts := maxValidationAttempts
		if isPermanent {
			errorType = "configuration"
			maxAttempts = maxPermanentErrorAttempts
		}
		firstErrorMsg := getValidationFailureReason(validationStatus)
		r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "ValidationRetry",
			fmt.Sprintf("Validation failed (attempt %d/%d, %s error), will retry: %s",
				attempts, maxAttempts, errorType, firstErrorMsg))
	case mcpv1.ValidationStateFailed:
		// Validation failed permanently
		isPermanent := r.isPermanentError(result, hasMismatch)
		maxAttempts := maxValidationAttempts
		if isPermanent {
			maxAttempts = maxPermanentErrorAttempts
		}
		firstErrorMsg := getValidationFailureReason(validationStatus)
		r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "ValidationFailed",
			fmt.Sprintf("Validation failed after %d/%d attempts: %s",
				attempts, maxAttempts, firstErrorMsg))
	}

	// Handle strict mode enforcement for failed validations
	if r.isStrictModeEnabled(mcpServer) && state == mcpv1.ValidationStateFailed {
		// Determine appropriate threshold based on error type
		isPermanent := r.isPermanentError(result, hasMismatch)
		minAttempts := maxPermanentErrorAttempts
		if !isPermanent {
			minAttempts = maxValidationAttempts
		}

		if attempts >= minAttempts {
			log.Info("Strict mode: deleting deployment after validation failure",
				"attempts", attempts,
				"minAttempts", minAttempts,
				"isPermanentError", isPermanent,
				"reason", getValidationFailureReason(validationStatus))

			// Delete the deployment
			if err := r.deleteDeploymentForValidationFailure(ctx, mcpServer); err != nil {
				log.Error(err, "Failed to delete deployment in strict mode")
				// Don't return error - we want to update status even if delete fails
			} else {
				// Clear replica counts since deployment is deleted
				mcpServer.Status.Replicas = 0
				mcpServer.Status.ReadyReplicas = 0
				mcpServer.Status.AvailableReplicas = 0

				// Update phase to ValidationFailed
				mcpServer.Status.Phase = mcpv1.MCPServerPhaseValidationFailed
				mcpServer.Status.Message = fmt.Sprintf("Deployment deleted after %d validation attempts: %s",
					attempts, getValidationFailureReason(validationStatus))

				// Update conditions to reflect that deployment has been deleted
				now := metav1.Now()

				// Set Ready condition to False
				readyCondition := mcpv1.MCPServerCondition{
					Type:               mcpv1.MCPServerConditionReady,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "DeploymentDeleted",
					Message:            "Deployment deleted after validation failure in strict mode",
				}
				r.setCondition(mcpServer, readyCondition)

				// Set Available condition to False
				availableCondition := mcpv1.MCPServerCondition{
					Type:               mcpv1.MCPServerConditionAvailable,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "DeploymentDeleted",
					Message:            "Deployment deleted after validation failure in strict mode",
				}
				r.setCondition(mcpServer, availableCondition)

				// Set Progressing condition to False
				progressingCondition := mcpv1.MCPServerCondition{
					Type:               mcpv1.MCPServerConditionProgressing,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: now,
					Reason:             "DeploymentDeleted",
					Message:            "Deployment deleted after validation failure in strict mode",
				}
				r.setCondition(mcpServer, progressingCondition)

				// The Degraded condition is already set by updateValidationConditions() at line 1304

				// Update status again with new phase, replica counts, and conditions
				if err := r.updateStatus(ctx, mcpServer); err != nil {
					log.Error(err, "Failed to update status after deployment deletion")
					return err
				}

				r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "DeploymentDeleted",
					fmt.Sprintf("Deleted deployment after %d validation attempts in strict mode: %s",
						attempts, getValidationFailureReason(validationStatus)))
			}
		}
	}

	return nil
}

// checkProtocolMismatch detects if the configured protocol doesn't match the detected protocol
// Returns true if there's a mismatch, false otherwise
func (r *MCPServerReconciler) checkProtocolMismatch(ctx context.Context, mcpServer *mcpv1.MCPServer, result *validator.ValidationResult) bool {
	log := logf.FromContext(ctx)

	// No mismatch possible if protocol is auto (auto-detection by definition can't mismatch)
	if mcpServer.Spec.Transport == nil || mcpServer.Spec.Transport.Protocol == "" || mcpServer.Spec.Transport.Protocol == mcpv1.MCPProtocolAuto {
		return false
	}

	// No mismatch if detection failed (unknown protocol)
	if result.DetectedTransport == "" || result.DetectedTransport == validator.TransportUnknown {
		return false
	}

	// Get configured protocol
	configuredProtocol := mcpServer.Spec.Transport.Protocol
	detectedProtocol := string(result.DetectedTransport)

	// Map configured protocol to expected detected transport
	var expectedTransport string
	switch configuredProtocol {
	case mcpv1.MCPProtocolStreamableHTTP:
		expectedTransport = string(validator.TransportStreamableHTTP)
	case mcpv1.MCPProtocolSSE:
		expectedTransport = string(validator.TransportSSE)
	default:
		// Unknown configured protocol, no mismatch detection
		return false
	}

	// Check for mismatch
	if detectedProtocol != expectedTransport {
		log.Info("Protocol mismatch detected",
			"configured", configuredProtocol,
			"detected", detectedProtocol,
			"expected", expectedTransport)

		// Add mismatch issue to validation status
		mismatchIssue := mcpv1.ValidationIssue{
			Level: validator.LevelError,
			Message: fmt.Sprintf("Protocol mismatch: configured '%s' but server uses '%s'. "+
				"Update spec.transport.protocol to '%s' or use 'auto' for automatic detection.",
				configuredProtocol, detectedProtocol, detectedProtocol),
			Code: validator.CodeProtocolMismatch,
		}

		// Add to issues if not already present
		if mcpServer.Status.Validation == nil {
			mcpServer.Status.Validation = &mcpv1.ValidationStatus{
				Issues: []mcpv1.ValidationIssue{mismatchIssue},
			}
		} else {
			// Check if mismatch issue already exists
			found := false
			for _, issue := range mcpServer.Status.Validation.Issues {
				if issue.Code == validator.CodeProtocolMismatch {
					found = true
					break
				}
			}
			if !found {
				mcpServer.Status.Validation.Issues = append(mcpServer.Status.Validation.Issues, mismatchIssue)
			}
		}

		// Emit warning event
		r.Recorder.Event(mcpServer, corev1.EventTypeWarning, "ProtocolMismatch",
			fmt.Sprintf("Protocol mismatch: configured '%s' but detected '%s'. Update spec.transport.protocol to match or use 'auto'.",
				configuredProtocol, detectedProtocol))

		return true
	}

	return false
}

// updateValidationConditions updates conditions based on validation results and protocol mismatch
func (r *MCPServerReconciler) updateValidationConditions(mcpServer *mcpv1.MCPServer, result *validator.ValidationResult, hasMismatch bool) {
	now := metav1.Now()

	// In strict mode with mismatch or validation failure, set Degraded condition
	if hasMismatch || !result.IsCompliant() {
		if r.isStrictModeEnabled(mcpServer) {
			// Strict mode: mark as Failed phase (current behavior)
			mcpServer.Status.Phase = mcpv1.MCPServerPhaseFailed
			mcpServer.Status.Message = "Protocol validation failed in strict mode"

			// Set Degraded condition
			degradedCondition := mcpv1.MCPServerCondition{
				Type:               mcpv1.MCPServerConditionDegraded,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             "ValidationFailedStrict",
				Message:            "MCP protocol validation failed in strict mode",
			}
			r.setCondition(mcpServer, degradedCondition)
		} else {
			// Non-strict mode: set Degraded condition but keep Running phase
			var reason, message string
			if hasMismatch {
				reason = "ProtocolMismatch"
				message = "Configured protocol does not match detected protocol"
			} else {
				reason = "ValidationFailed"
				message = "MCP protocol validation failed"
			}

			degradedCondition := mcpv1.MCPServerCondition{
				Type:               mcpv1.MCPServerConditionDegraded,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: now,
				Reason:             reason,
				Message:            message,
			}
			r.setCondition(mcpServer, degradedCondition)
		}
	} else {
		// Validation passed and no mismatch - clear Degraded condition
		degradedCondition := mcpv1.MCPServerCondition{
			Type:               mcpv1.MCPServerConditionDegraded,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: now,
			Reason:             "ValidationPassed",
			Message:            "MCP protocol validation succeeded",
		}
		r.setCondition(mcpServer, degradedCondition)
	}
}

// isStrictModeEnabled checks if strict mode validation is enabled
func (r *MCPServerReconciler) isStrictModeEnabled(mcpServer *mcpv1.MCPServer) bool {
	if mcpServer.Spec.Validation == nil {
		return false
	}
	if mcpServer.Spec.Validation.StrictMode == nil {
		return false
	}
	return *mcpServer.Spec.Validation.StrictMode
}

// getValidationRetryInterval calculates retry interval for failed validations using progressive backoff
// Returns 0 if validation should not be retried (terminal states: Passed or Failed)
func (r *MCPServerReconciler) getValidationRetryInterval(mcpServer *mcpv1.MCPServer) time.Duration {
	// If validation passed, don't retry (no periodic validation)
	if mcpServer.Status.Validation != nil &&
		mcpServer.Status.Validation.State == mcpv1.ValidationStatePassed {
		return 0
	}

	// If validation failed permanently, don't retry
	// This happens when attempts >= maxValidationAttempts (5 for transient, 2 for permanent errors)
	if mcpServer.Status.Validation != nil &&
		mcpServer.Status.Validation.State == mcpv1.ValidationStateFailed {
		return 0
	}

	// For validating state (transient errors or first permanent error attempt), use progressive backoff
	// Note: updateValidationStatus() enforces max attempts and transitions to Failed state
	if mcpServer.Status.Validation == nil {
		// First validation attempt - retry quickly (pods might still be starting)
		return 30 * time.Second
	}

	attempts := mcpServer.Status.Validation.Attempts

	// Progressive backoff based on attempt count:
	// - Attempt 1-2: retry in 30 seconds (initial validation, might be starting up)
	// - Attempt 3-4: retry in 1 minute (transient issues)
	// - Attempt 5+: retry in 2 minutes (persistent transient issues, will stop at maxValidationAttempts)
	// Note: Max attempts enforcement happens in updateValidationStatus(), not here
	switch {
	case attempts <= 2:
		return 30 * time.Second
	case attempts <= 4:
		return 1 * time.Minute
	default:
		// For remaining attempts up to maxValidationAttempts
		return 2 * time.Minute
	}
}

// getEnvAsInt32 retrieves an environment variable as int32 with a default value
func getEnvAsInt32(key string, defaultValue int32) int32 {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseInt(val, 10, 32); err == nil {
			return int32(parsed)
		}
	}
	return defaultValue
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Filter out status-only updates to prevent reconciliation loops
		// Only reconcile when spec changes (generation increment) or owned resources change
		For(&mcpv1.MCPServer{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
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
