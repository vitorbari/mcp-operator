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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// MCPServerSpec defines the desired state of MCPServer
type MCPServerSpec struct {
	// Image specifies the container image for the MCP server
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*(?::[a-zA-Z0-9][a-zA-Z0-9._-]*)?$`
	Image string `json:"image"`

	// Replicas specifies the number of MCP server instances to run
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Capabilities defines the MCP capabilities this server provides
	// +kubebuilder:validation:MinItems=1
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`

	// Resources defines the resource requirements for the MCP server containers
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Security defines security-related configuration for the MCP server
	// +optional
	Security *MCPServerSecurity `json:"security,omitempty"`

	// Service defines the service configuration for the MCP server
	// +optional
	Service *MCPServerService `json:"service,omitempty"`

	// HealthCheck defines health checking parameters
	// +optional
	HealthCheck *MCPServerHealthCheck `json:"healthCheck,omitempty"`

	// Environment defines environment variables for the MCP server
	// +optional
	Environment []corev1.EnvVar `json:"environment,omitempty"`

	// PodTemplate defines additional pod template specifications
	// +optional
	PodTemplate *MCPServerPodTemplate `json:"podTemplate,omitempty"`

	// Configuration defines MCP-specific configuration
	// +optional
	Configuration *MCPServerConfiguration `json:"configuration,omitempty"`

	// HPA defines Horizontal Pod Autoscaler configuration
	// +optional
	HPA *MCPServerHPA `json:"hpa,omitempty"`
}

// MCPServerSecurity defines security settings for the MCP server
type MCPServerSecurity struct {
	// AllowedUsers specifies which users or service accounts can access this MCP server
	// +optional
	AllowedUsers []string `json:"allowedUsers,omitempty"`

	// AllowedGroups specifies which groups can access this MCP server
	// +optional
	AllowedGroups []string `json:"allowedGroups,omitempty"`

	// TLS defines TLS configuration for secure communication
	// +optional
	TLS *MCPServerTLS `json:"tls,omitempty"`

	// RunAsUser specifies the user ID to run the MCP server process
	// +optional
	RunAsUser *int64 `json:"runAsUser,omitempty"`

	// RunAsGroup specifies the group ID to run the MCP server process
	// +optional
	RunAsGroup *int64 `json:"runAsGroup,omitempty"`

	// ReadOnlyRootFilesystem specifies if the container should have a read-only root filesystem
	// +optional
	ReadOnlyRootFilesystem *bool `json:"readOnlyRootFilesystem,omitempty"`
}

// MCPServerTLS defines TLS configuration
type MCPServerTLS struct {
	// Enabled indicates if TLS should be enabled
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// SecretName refers to a Secret containing TLS certificates
	// +optional
	SecretName string `json:"secretName,omitempty"`

	// CertFile specifies the path to the certificate file within the container
	// +optional
	CertFile string `json:"certFile,omitempty"`

	// KeyFile specifies the path to the private key file within the container
	// +optional
	KeyFile string `json:"keyFile,omitempty"`
}

// MCPServerService defines service configuration
type MCPServerService struct {
	// Type specifies the type of Kubernetes service to create
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +kubebuilder:default=ClusterIP
	// +optional
	Type corev1.ServiceType `json:"type,omitempty"`

	// Port specifies the port on which the MCP server listens
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default=8080
	// +optional
	Port int32 `json:"port,omitempty"`

	// TargetPort specifies the container port to target
	// +optional
	TargetPort *intstr.IntOrString `json:"targetPort,omitempty"`

	// Protocol specifies the network protocol
	// +kubebuilder:validation:Enum=TCP;UDP
	// +kubebuilder:default=TCP
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`

	// Annotations to add to the service
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// MCPServerHealthCheck defines health checking parameters
type MCPServerHealthCheck struct {
	// Enabled indicates if health checks should be performed
	// +kubebuilder:default=true
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// Path specifies the HTTP path for health checks
	// +kubebuilder:default="/health"
	// +optional
	Path string `json:"path,omitempty"`

	// Port specifies the port for health checks
	// +optional
	Port *intstr.IntOrString `json:"port,omitempty"`

	// InitialDelaySeconds specifies the delay before the first health check
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=30
	// +optional
	InitialDelaySeconds *int32 `json:"initialDelaySeconds,omitempty"`

	// PeriodSeconds specifies the interval between health checks
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	// +optional
	PeriodSeconds *int32 `json:"periodSeconds,omitempty"`

	// TimeoutSeconds specifies the timeout for health checks
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=5
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`

	// FailureThreshold specifies the number of failed checks before marking as unhealthy
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	// +optional
	FailureThreshold *int32 `json:"failureThreshold,omitempty"`

	// SuccessThreshold specifies the number of successful checks before marking as healthy
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	SuccessThreshold *int32 `json:"successThreshold,omitempty"`
}

// MCPServerPodTemplate defines additional pod template specifications
type MCPServerPodTemplate struct {
	// Labels to add to the pod
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to add to the pod
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// NodeSelector specifies node selection constraints
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations specifies pod tolerations
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity specifies pod affinity rules
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// ServiceAccountName specifies the service account to use for the pod
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// ImagePullSecrets specifies secrets for pulling container images
	// +optional
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Volumes specifies additional volumes to mount
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// VolumeMounts specifies additional volume mounts for the container
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`
}

// MCPServerConfiguration defines MCP-specific configuration
type MCPServerConfiguration struct {
	// MaxConnections specifies the maximum number of concurrent connections
	// +kubebuilder:validation:Minimum=1
	// +optional
	MaxConnections *int32 `json:"maxConnections,omitempty"`

	// ConnectionTimeout specifies the connection timeout duration
	// +kubebuilder:validation:Pattern=`^\d+[smh]$`
	// +optional
	ConnectionTimeout string `json:"connectionTimeout,omitempty"`

	// LogLevel specifies the logging level
	// +kubebuilder:validation:Enum=debug;info;warn;error
	// +kubebuilder:default=info
	// +optional
	LogLevel string `json:"logLevel,omitempty"`

	// MetricsEnabled indicates if metrics collection should be enabled
	// +kubebuilder:default=true
	// +optional
	MetricsEnabled *bool `json:"metricsEnabled,omitempty"`

	// MetricsPath specifies the HTTP path for metrics endpoint
	// +kubebuilder:default="/metrics"
	// +optional
	MetricsPath string `json:"metricsPath,omitempty"`

	// CustomConfig allows for additional custom configuration as key-value pairs
	// +optional
	CustomConfig map[string]string `json:"customConfig,omitempty"`
}

// MCPServerStatus defines the observed state of MCPServer
type MCPServerStatus struct {
	// Phase represents the current phase of the MCP server deployment
	// +optional
	Phase MCPServerPhase `json:"phase,omitempty"`

	// Message provides additional information about the current phase
	// +optional
	Message string `json:"message,omitempty"`

	// Replicas represents the current number of running replicas
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas represents the number of ready replicas
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// AvailableReplicas represents the number of available replicas
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`

	// Conditions represents the current conditions of the MCP server
	// +optional
	Conditions []MCPServerCondition `json:"conditions,omitempty"`

	// ServiceEndpoint represents the endpoint where the MCP server is accessible
	// +optional
	ServiceEndpoint string `json:"serviceEndpoint,omitempty"`

	// LastReconcileTime represents the last time the MCP server was reconciled
	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// ObservedGeneration represents the most recent generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Metrics contains current metrics about the MCP server
	// +optional
	Metrics *MCPServerMetrics `json:"metrics,omitempty"`
}

// MCPServerPhase represents the current phase of an MCP server deployment
// +kubebuilder:validation:Enum=Pending;Creating;Running;Scaling;Updating;Failed;Terminating
type MCPServerPhase string

const (
	// MCPServerPhasePending indicates the MCP server is pending creation
	MCPServerPhasePending MCPServerPhase = "Pending"
	// MCPServerPhaseCreating indicates the MCP server is being created
	MCPServerPhaseCreating MCPServerPhase = "Creating"
	// MCPServerPhaseRunning indicates the MCP server is running normally
	MCPServerPhaseRunning MCPServerPhase = "Running"
	// MCPServerPhaseScaling indicates the MCP server is scaling up or down
	MCPServerPhaseScaling MCPServerPhase = "Scaling"
	// MCPServerPhaseUpdating indicates the MCP server is being updated
	MCPServerPhaseUpdating MCPServerPhase = "Updating"
	// MCPServerPhaseFailed indicates the MCP server deployment failed
	MCPServerPhaseFailed MCPServerPhase = "Failed"
	// MCPServerPhaseTerminating indicates the MCP server is being terminated
	MCPServerPhaseTerminating MCPServerPhase = "Terminating"
)

// MCPServerCondition describes the current condition of an MCP server
type MCPServerCondition struct {
	// Type of the condition
	Type MCPServerConditionType `json:"type"`

	// Status of the condition (True, False, Unknown)
	Status corev1.ConditionStatus `json:"status"`

	// LastTransitionTime is the last time the condition transitioned
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a unique, one-word, CamelCase reason for the condition's last transition
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable message indicating details about last transition
	// +optional
	Message string `json:"message,omitempty"`
}

// MCPServerConditionType represents the type of condition
// +kubebuilder:validation:Enum=Ready;Available;Progressing;Degraded;Reconciled
type MCPServerConditionType string

const (
	// MCPServerConditionReady indicates the MCP server is ready to serve traffic
	MCPServerConditionReady MCPServerConditionType = "Ready"
	// MCPServerConditionAvailable indicates the MCP server has minimum required replicas available
	MCPServerConditionAvailable MCPServerConditionType = "Available"
	// MCPServerConditionProgressing indicates the MCP server is progressing towards desired state
	MCPServerConditionProgressing MCPServerConditionType = "Progressing"
	// MCPServerConditionDegraded indicates the MCP server is in a degraded state
	MCPServerConditionDegraded MCPServerConditionType = "Degraded"
	// MCPServerConditionReconciled indicates the MCP server has been successfully reconciled
	MCPServerConditionReconciled MCPServerConditionType = "Reconciled"
)

// MCPServerMetrics contains runtime metrics for the MCP server
type MCPServerMetrics struct {
	// CurrentConnections represents the current number of active connections
	// +optional
	CurrentConnections *int32 `json:"currentConnections,omitempty"`

	// TotalRequests represents the total number of requests processed
	// +optional
	TotalRequests *int64 `json:"totalRequests,omitempty"`

	// ErrorRate represents the current error rate as a percentage string (e.g., "2.5%")
	// +optional
	ErrorRate *string `json:"errorRate,omitempty"`

	// AverageResponseTime represents the average response time in milliseconds
	// +optional
	AverageResponseTime *int64 `json:"averageResponseTime,omitempty"`

	// ResourceUsage contains current resource usage information
	// +optional
	ResourceUsage *MCPServerResourceUsage `json:"resourceUsage,omitempty"`
}

// MCPServerResourceUsage contains resource usage information
type MCPServerResourceUsage struct {
	// CPU represents current CPU usage
	// +optional
	CPU *resource.Quantity `json:"cpu,omitempty"`

	// Memory represents current memory usage
	// +optional
	Memory *resource.Quantity `json:"memory,omitempty"`
}

// MCPServerHPA defines Horizontal Pod Autoscaler configuration
type MCPServerHPA struct {
	// Enabled indicates if HPA should be created
	// +kubebuilder:default=false
	// +optional
	Enabled *bool `json:"enabled,omitempty"`

	// MinReplicas is the lower limit for the number of replicas
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the upper limit for the number of replicas
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// TargetCPUUtilizationPercentage is the target average CPU utilization
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`

	// TargetMemoryUtilizationPercentage is the target average memory utilization
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +optional
	TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`

	// ScaleUpBehavior configures scaling up behavior
	// +optional
	ScaleUpBehavior *MCPServerHPABehavior `json:"scaleUpBehavior,omitempty"`

	// ScaleDownBehavior configures scaling down behavior
	// +optional
	ScaleDownBehavior *MCPServerHPABehavior `json:"scaleDownBehavior,omitempty"`
}

// MCPServerHPABehavior defines scaling behavior policies
type MCPServerHPABehavior struct {
	// StabilizationWindowSeconds is the number of seconds for which past recommendations should be considered
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	// +optional
	StabilizationWindowSeconds *int32 `json:"stabilizationWindowSeconds,omitempty"`

	// Policies is a list of potential scaling policies
	// +optional
	Policies []MCPServerHPAPolicy `json:"policies,omitempty"`
}

// MCPServerHPAPolicy defines a single scaling policy
type MCPServerHPAPolicy struct {
	// Type is the type of the policy (Percent or Pods)
	// +kubebuilder:validation:Enum=Percent;Pods
	Type string `json:"type"`

	// Value contains the amount of change which is permitted by the policy
	// +kubebuilder:validation:Minimum=1
	Value int32 `json:"value"`

	// PeriodSeconds specifies how long the policy should be held
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1800
	PeriodSeconds int32 `json:"periodSeconds"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MCPServer is the Schema for the mcpservers API
type MCPServer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of MCPServer
	// +required
	Spec MCPServerSpec `json:"spec"`

	// status defines the observed state of MCPServer
	// +optional
	Status MCPServerStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// MCPServerList contains a list of MCPServer
type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MCPServer{}, &MCPServerList{})
}
