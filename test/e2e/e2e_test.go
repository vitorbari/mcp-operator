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

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/vitorbari/mcp-operator/test/utils"
)

// operatorNamespace is where the operator is deployed
const operatorNamespace = "mcp-operator-system"

// testNamespace is where test MCPServer resources are created
const testNamespace = "mcp-operator-e2e-test"

// serviceAccountName created for the project
const serviceAccountName = "mcp-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "mcp-operator-controller-manager-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "mcp-operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// Before running the tests, set up the environment by creating the namespace,
	// enforce the restricted security policy to the namespace, installing CRDs,
	// and deploying the controller.
	BeforeAll(func() {
		By("creating operator namespace")
		cmd := exec.Command("kubectl", "create", "ns", operatorNamespace)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create operator namespace")

		By("labeling operator namespace with restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", operatorNamespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label operator namespace")

		By("creating test namespace for MCPServer resources")
		cmd = exec.Command("kubectl", "create", "ns", testNamespace)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test namespace")

		By("labeling test namespace with restricted security policy")
		cmd = exec.Command("kubectl", "label", "--overwrite", "ns", testNamespace,
			"pod-security.kubernetes.io/enforce=restricted")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to label test namespace")

		By("installing CRDs")
		cmd = exec.Command("make", "install")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

		By("configuring controller with reduced retry counts for E2E tests")
		patchJSON := `{"spec":{"template":{"spec":{"containers":[{"name":"manager",` +
			`"env":[{"name":"MCP_MAX_VALIDATION_ATTEMPTS","value":"3"},` +
			`{"name":"MCP_MAX_PERMANENT_ERROR_ATTEMPTS","value":"2"}]}]}}}}`
		cmd = exec.Command("kubectl", "patch", "deployment", "mcp-operator-controller-manager",
			"-n", operatorNamespace, "--type=strategic", "-p", patchJSON)
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to patch controller deployment with retry config")

		By("waiting for controller to restart with new config")
		cmd = exec.Command("kubectl", "rollout", "status", "deployment/mcp-operator-controller-manager",
			"-n", operatorNamespace, "--timeout=2m")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to wait for controller restart")

		By("deploying monitoring resources")
		// Apply monitoring resources since Prometheus Operator is installed in BeforeSuite
		cmd = exec.Command("kubectl", "apply", "-f", "./dist/monitoring.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy monitoring resources")
	})

	// After all tests have been executed, clean up by undeploying the controller, uninstalling CRDs,
	// and deleting the namespace.
	AfterAll(func() {
		By("cleaning up the curl pod for metrics")
		cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", operatorNamespace)
		_, _ = utils.Run(cmd)

		By("removing test namespace (and all MCPServer resources)")
		// Use --wait=false to avoid hanging on finalizers during cleanup
		cmd = exec.Command("kubectl", "delete", "ns", testNamespace, "--timeout=60s", "--wait=false")
		_, _ = utils.Run(cmd)

		By("removing monitoring resources")
		cmd = exec.Command("kubectl", "delete", "-f", "../../dist/monitoring.yaml", "--ignore-not-found", "--timeout=60s", "--wait=false")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		// Use direct kubectl delete with timeout and --wait=false to avoid hanging on finalizers
		cmd = exec.Command("kubectl", "delete", "-f", "../../dist/install.yaml", "--ignore-not-found", "--timeout=60s", "--wait=false")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		// CRD deletion can also hang, add timeout and --wait=false
		cmd = exec.Command("kubectl", "delete", "-f", "../../config/crd/bases", "--ignore-not-found", "--timeout=60s", "--wait=false")
		_, _ = utils.Run(cmd)

		By("removing operator namespace")
		// Use --wait=false to avoid hanging on finalizers during cleanup
		cmd = exec.Command("kubectl", "delete", "ns", operatorNamespace, "--timeout=60s", "--wait=false")
		_, _ = utils.Run(cmd)
	})

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", operatorNamespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", operatorNamespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", operatorNamespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", operatorNamespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("MCPServer CRD Tests", func() {
		It("should create MCPServer and bring it to Running phase", func() {
			mcpServerName := "test-basic-mcpserver"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
`, mcpServerName, testNamespace)

			By("creating MCPServer CR")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create MCPServer")

			By("waiting for MCPServer to reach Running phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Deployment was created with correct specs")
			cmd := exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.replicas}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("1"))

			By("verifying Service was created and has ClusterIP")
			cmd = exec.Command("kubectl", "get", "service", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.clusterIP}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "Service should have ClusterIP")

			By("verifying ServiceAccount was created")
			cmd = exec.Command("kubectl", "get", "serviceaccount", mcpServerName,
				"-n", testNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Deployment pods are running")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-l", "app="+mcpServerName,
					"-n", testNamespace,
					"-o", "jsonpath={.items[0].status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})

		It("should maintain accurate status conditions", func() {
			mcpServerName := "test-status-conditions"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
`, mcpServerName, testNamespace)

			By("creating MCPServer")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Ready condition becomes True when pods are running")
			Eventually(func(g Gomega) {
				result, err := getMCPServerStatus(mcpServerName)
				g.Expect(err).NotTo(HaveOccurred())

				status, ok := result["status"].(map[string]interface{})
				g.Expect(ok).To(BeTrue())

				readyCond := findCondition(status, "Ready")
				g.Expect(readyCond).NotTo(BeNil(), "Ready condition should exist")
				g.Expect(readyCond["status"]).To(Equal("True"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Available condition is set")
			result, err := getMCPServerStatus(mcpServerName)
			Expect(err).NotTo(HaveOccurred())

			status, ok := result["status"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			availableCond := findCondition(status, "Available")
			Expect(availableCond).NotTo(BeNil(), "Available condition should exist")
			Expect(availableCond["status"]).To(Equal("True"))

			By("verifying ObservedGeneration matches current generation")
			metadata, ok := result["metadata"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			generation := metadata["generation"]
			observedGeneration := status["observedGeneration"]
			Expect(observedGeneration).To(Equal(generation), "ObservedGeneration should match Generation")
		})

		It("should set correct owner references on child resources", func() {
			mcpServerName := "test-ownership"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
`, mcpServerName, testNamespace)

			By("creating MCPServer")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for resources to be created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "deployment", mcpServerName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying Deployment has owner reference")
			cmd := exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.metadata.ownerReferences[0].name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(mcpServerName))

			By("verifying owner reference kind is MCPServer")
			cmd = exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.metadata.ownerReferences[0].kind}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("MCPServer"))

			By("verifying Service has owner reference")
			cmd = exec.Command("kubectl", "get", "service", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.metadata.ownerReferences[0].name}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(mcpServerName))

			By("verifying ServiceAccount has owner reference")
			cmd = exec.Command("kubectl", "get", "serviceaccount", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.metadata.ownerReferences[0].name}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(mcpServerName))
		})

		It("should propagate labels correctly to all resources", func() {
			mcpServerName := "test-labels"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
`, mcpServerName, testNamespace)

			By("creating MCPServer")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Deployment to be created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "deployment", mcpServerName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying standard labels on Deployment")
			cmd := exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "json")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var deploymentData map[string]interface{}
			err = json.Unmarshal([]byte(output), &deploymentData)
			Expect(err).NotTo(HaveOccurred())

			metadata, ok := deploymentData["metadata"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			labels, ok := metadata["labels"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			fmt.Println(metadata, labels)

			Expect(labels["app"]).To(Equal(mcpServerName))
			Expect(labels["app.kubernetes.io/name"]).To(Equal("mcpserver"))
			Expect(labels["app.kubernetes.io/instance"]).To(Equal(mcpServerName))
			Expect(labels["app.kubernetes.io/managed-by"]).To(Equal("mcp-operator"))
		})

		It("should configure container settings (command, args, resources, environment)", func() {
			mcpServerName := "test-container-config"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  command: ["/bin/sh", "-c"]
  args: ["echo 'Custom command executed' && nginx -g 'daemon off;'"]
  replicas: 1
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
  environment:
    - name: LOG_LEVEL
      value: "debug"
    - name: MCP_PORT
      value: "8080"
    - name: CUSTOM_VAR
      value: "test-value"
`, mcpServerName, testNamespace)

			By("creating MCPServer with container configuration")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to reach Running phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Deployment has custom command")
			cmd := exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.template.spec.containers[0].command}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("/bin/sh"))
			Expect(output).To(ContainSubstring("-c"))

			By("verifying Deployment has custom args")
			cmd = exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.template.spec.containers[0].args}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Custom command executed"))

			By("verifying Deployment has CPU requests")
			cmd = exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.template.spec.containers[0].resources.requests.cpu}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("100m"))

			By("verifying Deployment has memory requests")
			cmd = exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.template.spec.containers[0].resources.requests.memory}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("128Mi"))

			By("verifying Deployment has CPU limits")
			cmd = exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.template.spec.containers[0].resources.limits.cpu}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("500m"))

			By("verifying Deployment has memory limits")
			cmd = exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.template.spec.containers[0].resources.limits.memory}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("512Mi"))

			By("verifying Deployment has environment variables")
			cmd = exec.Command("kubectl", "get", "deployment", mcpServerName,
				"-n", testNamespace, "-o", "json")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			var deploymentData map[string]interface{}
			err = json.Unmarshal([]byte(output), &deploymentData)
			Expect(err).NotTo(HaveOccurred())

			spec := deploymentData["spec"].(map[string]interface{})
			template := spec["template"].(map[string]interface{})
			podSpec := template["spec"].(map[string]interface{})
			containers := podSpec["containers"].([]interface{})
			container := containers[0].(map[string]interface{})
			env := container["env"].([]interface{})

			logLevelFound := false
			mcpPortFound := false
			customVarFound := false

			for _, e := range env {
				envVar := e.(map[string]interface{})
				if envVar["name"] == "LOG_LEVEL" && envVar["value"] == "debug" {
					logLevelFound = true
				}
				if envVar["name"] == "MCP_PORT" && envVar["value"] == "8080" {
					mcpPortFound = true
				}
				if envVar["name"] == "CUSTOM_VAR" && envVar["value"] == "test-value" {
					customVarFound = true
				}
			}

			Expect(logLevelFound).To(BeTrue(), "LOG_LEVEL environment variable should be set")
			Expect(mcpPortFound).To(BeTrue(), "MCP_PORT environment variable should be set")
			Expect(customVarFound).To(BeTrue(), "CUSTOM_VAR environment variable should be set")
		})

		It("should apply security context settings", func() {
			mcpServerName := "test-security"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
`, mcpServerName, testNamespace)

			By("creating MCPServer with security context")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to reach Running phase")
			waitForMCPServerRunning(mcpServerName)

			By("verifying Deployment has runAsUser")
			Expect(getDeploymentField(mcpServerName,
				".spec.template.spec.containers[0].securityContext.runAsUser")).To(Equal("1000"))

			By("verifying Deployment has runAsGroup")
			Expect(getDeploymentField(mcpServerName,
				".spec.template.spec.containers[0].securityContext.runAsGroup")).To(Equal("1000"))

			By("verifying Deployment has runAsNonRoot")
			Expect(getDeploymentField(mcpServerName,
				".spec.template.spec.containers[0].securityContext.runAsNonRoot")).To(Equal("true"))

			By("verifying Deployment has allowPrivilegeEscalation set to false")
			Expect(getDeploymentField(mcpServerName,
				".spec.template.spec.containers[0].securityContext.allowPrivilegeEscalation")).To(Equal("false"))
		})

		It("should configure transport and service settings", func() {
			mcpServerName := "test-transport-service"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
  transport:
    type: http
    config:
      http:
        port: 3001
        path: "/mcp"
        sessionManagement: true
  service:
    type: ClusterIP
    port: 3001
    protocol: TCP
    annotations:
      custom-annotation: "test-value"
      prometheus.io/scrape: "true"
`, mcpServerName, testNamespace)

			By("creating MCPServer with transport and service configuration")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to reach Running phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Service has correct port configuration")
			cmd := exec.Command("kubectl", "get", "service", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.ports[0].port}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("3001"))

			By("verifying Service has session affinity for SSE")
			cmd = exec.Command("kubectl", "get", "service", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.sessionAffinity}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("ClientIP"))

			By("verifying status reports correct transport type")
			cmd = exec.Command("kubectl", "get", "mcpserver", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.status.transportType}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("http"))

			By("verifying Service type")
			cmd = exec.Command("kubectl", "get", "service", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.type}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("ClusterIP"))

			By("verifying Service protocol")
			cmd = exec.Command("kubectl", "get", "service", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.ports[0].protocol}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("TCP"))

			By("verifying Service has custom annotations")
			cmd = exec.Command("kubectl", "get", "service", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.metadata.annotations.custom-annotation}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("test-value"))

			cmd = exec.Command("kubectl", "get", "service", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.metadata.annotations.prometheus\\.io/scrape}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("true"))
		})

		It("should update Service sessionAffinity when sessionManagement changes", func() {
			mcpServerName := "test-session-update"

			// First create MCPServer without sessionManagement
			mcpServerYAMLWithoutSession := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
  transport:
    type: http
    config:
      http:
        port: 3001
        path: "/mcp"
        sessionManagement: false
`, mcpServerName, testNamespace)

			By("creating MCPServer without sessionManagement")
			err := applyMCPServerYAML(mcpServerYAMLWithoutSession)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to reach Running phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Service has sessionAffinity None initially")
			cmd := exec.Command("kubectl", "get", "service", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.sessionAffinity}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("None"))

			// Now update to enable sessionManagement
			mcpServerYAMLWithSession := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
  transport:
    type: http
    config:
      http:
        port: 3001
        path: "/mcp"
        sessionManagement: true
`, mcpServerName, testNamespace)

			By("updating MCPServer to enable sessionManagement")
			err = applyMCPServerYAML(mcpServerYAMLWithSession)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Service sessionAffinity updates to ClientIP")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "service", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.spec.sessionAffinity}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("ClientIP"))
			}, 30*time.Second, 2*time.Second).Should(Succeed())
		})

		It("should create and configure HPA when enabled", func() {
			mcpServerName := "test-hpa"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 2
  hpa:
    enabled: true
    minReplicas: 2
    maxReplicas: 5
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "200m"
      memory: "256Mi"
`, mcpServerName, testNamespace)

			By("creating MCPServer with HPA enabled")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to reach Running phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying HPA resource is created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "hpa", mcpServerName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying HPA has correct minReplicas")
			cmd := exec.Command("kubectl", "get", "hpa", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.minReplicas}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("2"))

			By("verifying HPA has correct maxReplicas")
			cmd = exec.Command("kubectl", "get", "hpa", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.maxReplicas}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("5"))

			By("verifying HPA has CPU target")
			cmd = exec.Command("kubectl", "get", "hpa", mcpServerName,
				"-n", testNamespace, "-o", "json")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("cpu"))
			Expect(output).To(ContainSubstring("70"))

			By("verifying HPA has memory target")
			Expect(output).To(ContainSubstring("memory"))
			Expect(output).To(ContainSubstring("80"))
		})

		It("should configure health check probes", func() {
			mcpServerName := "test-healthcheck"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
  healthCheck:
    enabled: true
    path: "/"
    port: 8080
`, mcpServerName, testNamespace)

			By("creating MCPServer with health check configuration")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to reach Running phase")
			waitForMCPServerRunning(mcpServerName)

			By("verifying Deployment has liveness probe")
			Expect(getDeploymentField(mcpServerName,
				".spec.template.spec.containers[0].livenessProbe.httpGet.path")).To(Equal("/"))

			By("verifying liveness probe port")
			Expect(getDeploymentField(mcpServerName,
				".spec.template.spec.containers[0].livenessProbe.httpGet.port")).To(Equal("8080"))

			By("verifying Deployment has readiness probe")
			Expect(getDeploymentField(mcpServerName,
				".spec.template.spec.containers[0].readinessProbe.httpGet.path")).To(Equal("/"))

			By("verifying readiness probe port")
			Expect(getDeploymentField(mcpServerName,
				".spec.template.spec.containers[0].readinessProbe.httpGet.port")).To(Equal("8080"))
		})

		It("should apply pod template customizations", func() {
			mcpServerName := "test-podtemplate"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
  podTemplate:
    labels:
      custom-label: "test-value"
      app-version: "1.0.0"
    annotations:
      prometheus.io/scrape: "true"
      custom-annotation: "test"
    nodeSelector:
      kubernetes.io/os: linux
`, mcpServerName, testNamespace)

			By("creating MCPServer with pod template customizations")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to reach Running phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for Pod to be created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "pods", "-l", "app="+mcpServerName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying Pod has custom labels")
			cmd := exec.Command("kubectl", "get", "pods", "-l", "app="+mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.items[0].metadata.labels.custom-label}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("test-value"))

			cmd = exec.Command("kubectl", "get", "pods", "-l", "app="+mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.items[0].metadata.labels.app-version}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("1.0.0"))

			By("verifying Pod has custom annotations")
			cmd = exec.Command("kubectl", "get", "pods", "-l", "app="+mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.items[0].metadata.annotations.prometheus\\.io/scrape}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("true"))

			cmd = exec.Command("kubectl", "get", "pods", "-l", "app="+mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.items[0].metadata.annotations.custom-annotation}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("test"))

			By("verifying Pod has node selector")
			cmd = exec.Command("kubectl", "get", "pods", "-l", "app="+mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.items[0].spec.nodeSelector.kubernetes\\.io/os}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("linux"))
		})

		It("should create and configure Ingress when enabled", func() {
			mcpServerName := "test-ingress"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
  ingress:
    enabled: true
    className: "nginx"
    host: "mcp.test.local"
    path: "/mcp"
    pathType: "Prefix"
    annotations:
      nginx.ingress.kubernetes.io/rewrite-target: "/"
      custom.annotation/test: "value"
`, mcpServerName, testNamespace)

			By("creating MCPServer with ingress enabled")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to reach Running phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Ingress resource is created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "ingress", mcpServerName, "-n", testNamespace)
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying Ingress has correct className")
			cmd := exec.Command("kubectl", "get", "ingress", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.ingressClassName}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("nginx"))

			By("verifying Ingress has correct host")
			cmd = exec.Command("kubectl", "get", "ingress", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.rules[0].host}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("mcp.test.local"))

			By("verifying Ingress has correct path")
			cmd = exec.Command("kubectl", "get", "ingress", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.rules[0].http.paths[0].path}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("/mcp"))

			By("verifying Ingress has correct pathType")
			cmd = exec.Command("kubectl", "get", "ingress", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.spec.rules[0].http.paths[0].pathType}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Prefix"))

			By("verifying Ingress has custom annotations")
			cmd = exec.Command("kubectl", "get", "ingress", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.metadata.annotations.nginx\\.ingress\\.kubernetes\\.io/rewrite-target}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("/"))

			cmd = exec.Command("kubectl", "get", "ingress", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.metadata.annotations.custom\\.annotation/test}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("value"))
		})

		It("should properly clean up resources on deletion", func() {
			mcpServerName := "test-cleanup"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: nginxinc/nginx-unprivileged:latest
  replicas: 1
`, mcpServerName, testNamespace)

			By("creating MCPServer")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to be ready")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("deleting MCPServer")
			cmd := exec.Command("kubectl", "delete", "mcpserver", mcpServerName,
				"-n", testNamespace, "--timeout=60s")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Deployment is deleted")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "deployment", mcpServerName,
					"-n", testNamespace)
				_, err := utils.Run(cmd)
				return err != nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue())

			By("verifying Service is deleted")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "service", mcpServerName,
					"-n", testNamespace)
				_, err := utils.Run(cmd)
				return err != nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue())

			By("verifying ServiceAccount is deleted")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "serviceaccount", mcpServerName,
					"-n", testNamespace)
				_, err := utils.Run(cmd)
				return err != nil
			}, 30*time.Second, 2*time.Second).Should(BeTrue())
		})
	})

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				// Get the name of the controller-manager pod
				cmd := exec.Command("kubectl", "get",
					"pods", "-l", "control-plane=controller-manager",
					"-o", "go-template={{ range .items }}"+
						"{{ if not .metadata.deletionTimestamp }}"+
						"{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", operatorNamespace,
				)

				podOutput, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve controller-manager pod information")
				podNames := utils.GetNonEmptyLines(podOutput)
				g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = podNames[0]
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))

				// Validate the pod's status
				cmd = exec.Command("kubectl", "get",
					"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
					"-n", operatorNamespace,
				)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"), "Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("creating a ClusterRoleBinding for the service account to allow access to metrics")
			cmd := exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
				"--clusterrole=mcp-operator-metrics-reader",
				fmt.Sprintf("--serviceaccount=%s:%s", operatorNamespace, serviceAccountName),
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create ClusterRoleBinding")

			By("validating that the metrics service is available")
			cmd = exec.Command("kubectl", "get", "service", metricsServiceName, "-n", operatorNamespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Metrics service should exist")

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("waiting for the metrics endpoint to be ready")
			verifyMetricsEndpointReady := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", metricsServiceName, "-n", operatorNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("8443"), "Metrics endpoint is not ready")
			}
			Eventually(verifyMetricsEndpointReady).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", operatorNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("controller-runtime.metrics\tServing metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted).Should(Succeed())

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", operatorNamespace,
				"--image=curlimages/curl:latest",
				"--overrides",
				fmt.Sprintf(`{
					"spec": {
						"containers": [{
							"name": "curl",
							"image": "curlimages/curl:latest",
							"command": ["/bin/sh", "-c"],
							"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
							"securityContext": {
								"readOnlyRootFilesystem": true,
								"allowPrivilegeEscalation": false,
								"capabilities": {
									"drop": ["ALL"]
								},
								"runAsNonRoot": true,
								"runAsUser": 1000,
								"seccompProfile": {
									"type": "RuntimeDefault"
								}
							}
						}],
						"serviceAccountName": "%s"
					}
				}`, token, metricsServiceName, operatorNamespace, serviceAccountName))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods", "curl-metrics",
					"-o", "jsonpath={.status.phase}",
					"-n", operatorNamespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			metricsOutput := getMetricsOutput()
			Expect(metricsOutput).To(ContainSubstring(
				"controller_runtime_reconcile_total",
			))
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks

		// Note: Add more e2e test scenarios here as the project evolves.
		// Consider applying sample MCPServer CRs and verifying their status:
		// metricsOutput := getMetricsOutput()
		// Expect(metricsOutput).To(ContainSubstring(
		//    fmt.Sprintf(`controller_runtime_reconcile_total{controller="%s",result="success"} 1`,
		//    strings.ToLower(<Kind>),
		// ))
	})

	Context("MCP Protocol Validation Tests", func() {
		It("should validate compliant MCP server and record metrics/events", func() {
			mcpServerName := "test-validation-compliant"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true
  validation:
    enabled: true
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
`, mcpServerName, testNamespace)

			By("creating compliant MCP server")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for MCPServer to reach Running phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for Service to have endpoints")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "endpoints", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.subsets[0].addresses[0].ip}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "Service should have at least one endpoint")
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			By("waiting for MCP server application to be ready")
			time.Sleep(15 * time.Second) // Give the Node.js app time to start listening

			By("waiting for validation status to be populated")
			Eventually(func(g Gomega) {
				result, err := getMCPServerStatus(mcpServerName)
				g.Expect(err).NotTo(HaveOccurred())

				status, ok := result["status"].(map[string]interface{})
				g.Expect(ok).To(BeTrue())

				validation, ok := status["validation"].(map[string]interface{})
				g.Expect(ok).To(BeTrue(), "Validation status should exist")

				// Debug: Print validation status if not compliant
				if compliant, ok := validation["compliant"].(bool); ok && !compliant {
					if issues, ok := validation["issues"].([]interface{}); ok && len(issues) > 0 {
						_, _ = fmt.Fprintf(GinkgoWriter, "Validation issues: %+v\n", issues)
					}
					if message, ok := validation["message"].(string); ok {
						_, _ = fmt.Fprintf(GinkgoWriter, "Validation message: %s\n", message)
					}
				}

				compliant, ok := validation["compliant"].(bool)
				g.Expect(ok).To(BeTrue())
				g.Expect(compliant).To(BeTrue(), "Server should be compliant")
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying protocol version is detected")
			result, err := getMCPServerStatus(mcpServerName)
			Expect(err).NotTo(HaveOccurred())

			status := result["status"].(map[string]interface{})
			validation := status["validation"].(map[string]interface{})

			protocolVersion, ok := validation["protocolVersion"].(string)
			Expect(ok).To(BeTrue())
			Expect(protocolVersion).To(Or(Equal("2024-11-05"), Equal("2025-03-26")))

			By("verifying capabilities are discovered")
			capabilities, ok := validation["capabilities"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(capabilities).ToNot(BeEmpty(), "Should have at least one capability")

			By("verifying lastValidated timestamp is set")
			lastValidated, ok := validation["lastValidated"].(string)
			Expect(ok).To(BeTrue())
			Expect(lastValidated).NotTo(BeEmpty())

			By("verifying validation status is accessible via JSONPath")
			cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.status.validation.compliant}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("true"))

			By("verifying capabilities are accessible via JSONPath")
			cmd = exec.Command("kubectl", "get", "mcpserver", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.status.validation.capabilities}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty())

			By("verifying validation metrics are present in Prometheus")
			// Wait for metrics to be recorded and available in Prometheus
			// Metrics may take a moment to appear after validation completes
			// Use fetchFreshMetrics() to get current metrics instead of cached ones
			Eventually(func(g Gomega) {
				metricsOutput := fetchFreshMetrics()
				g.Expect(metricsOutput).To(ContainSubstring("mcpserver_validation_compliant"),
					"Should have validation compliant metric")
				g.Expect(metricsOutput).To(ContainSubstring("mcpserver_validation_duration_seconds"),
					"Should have validation duration metric")
				g.Expect(metricsOutput).To(ContainSubstring("mcpserver_validation_total"),
					"Should have validation total metric")
				g.Expect(metricsOutput).To(ContainSubstring("mcpserver_capabilities"),
					"Should have capabilities metric")
				g.Expect(metricsOutput).To(ContainSubstring("mcpserver_protocol_version"),
					"Should have protocol version metric")
			}, 2*time.Minute, 10*time.Second).Should(Succeed())

			By("verifying ValidationPassed event is emitted")
			Eventually(func(g Gomega) {
				waitForEvent(g, mcpServerName, "ValidationPassed", "Should have ValidationPassed event")
			}, 1*time.Minute, 5*time.Second).Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "mcpserver", mcpServerName,
				"-n", testNamespace, "--timeout=120s")
			_, _ = utils.Run(cmd)
		})

		It("should fail deployment when strict mode is enabled and validation fails", func() {
			mcpServerName := "test-validation-strict-fail"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    config:
      http:
        port: 8080
        path: "/mcp"
  validation:
    enabled: true
    strictMode: true
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
`, mcpServerName, testNamespace)

			By("creating non-MCP server with strict mode")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for validation to fail and phase to become ValidationFailed " +
				"due to strict mode (3 attempts in E2E ~1.5min)")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("ValidationFailed"))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying validation status shows non-compliant")
			result, err := getMCPServerStatus(mcpServerName)
			Expect(err).NotTo(HaveOccurred())

			status := result["status"].(map[string]interface{})
			validation, ok := status["validation"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			compliant, ok := validation["compliant"].(bool)
			Expect(ok).To(BeTrue())
			Expect(compliant).To(BeFalse())

			By("verifying status message indicates validation failure")
			message, ok := status["message"].(string)
			Expect(ok).To(BeTrue())
			Expect(message).To(ContainSubstring("validation"))

			By("cleaning up")
			cmd := exec.Command("kubectl", "delete", "mcpserver", mcpServerName,
				"-n", testNamespace, "--timeout=120s")
			_, _ = utils.Run(cmd)
		})

		It("should not fail deployment when strict mode is disabled and validation fails", func() {
			mcpServerName := "test-validation-non-strict"
			// Wrong port: 8080
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    config:
      http:
        port: 8080
        path: "/sse"
  validation:
    enabled: true
    strictMode: false
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
`, mcpServerName, testNamespace)

			By("creating non-MCP server without strict mode")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for server to reach Running phase")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("waiting for validation to complete (3 attempts in E2E)")
			Eventually(func(g Gomega) {
				result, err := getMCPServerStatus(mcpServerName)
				g.Expect(err).NotTo(HaveOccurred())

				status, ok := result["status"].(map[string]interface{})
				g.Expect(ok).To(BeTrue())

				validation, hasValidation := status["validation"].(map[string]interface{})
				g.Expect(hasValidation).To(BeTrue(), "Validation status should exist")

				// Wait for validation to reach Failed state
				state, ok := validation["state"].(string)
				g.Expect(ok).To(BeTrue())
				g.Expect(state).To(Equal("Failed"))
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying server remains in Running phase despite failed validation")
			cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
				"-n", testNamespace, "-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Running"))

			By("verifying validation status shows non-compliant")
			result, err := getMCPServerStatus(mcpServerName)
			Expect(err).NotTo(HaveOccurred())

			status := result["status"].(map[string]interface{})
			validation, ok := status["validation"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			compliant, ok := validation["compliant"].(bool)
			Expect(ok).To(BeTrue())
			Expect(compliant).To(BeFalse())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "mcpserver", mcpServerName,
				"-n", testNamespace, "--timeout=120s")
			_, _ = utils.Run(cmd)
		})

		It("should pass validation when required capabilities are present", func() {
			mcpServerName := "test-validation-required-caps"
			mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    config:
      http:
        port: 3001
        path: "/sse"
  validation:
    enabled: true
    requiredCapabilities:
      - tools
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
`, mcpServerName, testNamespace)

			By("creating MCP server with required capabilities")
			err := applyMCPServerYAML(mcpServerYAML)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for validation to complete")
			Eventually(func(g Gomega) {
				result, err := getMCPServerStatus(mcpServerName)
				g.Expect(err).NotTo(HaveOccurred())

				status, ok := result["status"].(map[string]interface{})
				g.Expect(ok).To(BeTrue())

				validation, ok := status["validation"].(map[string]interface{})
				g.Expect(ok).To(BeTrue())

				compliant, ok := validation["compliant"].(bool)
				g.Expect(ok).To(BeTrue())
				g.Expect(compliant).To(BeTrue())
			}, 3*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying required capability is present")
			result, err := getMCPServerStatus(mcpServerName)
			Expect(err).NotTo(HaveOccurred())

			status := result["status"].(map[string]interface{})
			validation := status["validation"].(map[string]interface{})
			capabilities, ok := validation["capabilities"].([]interface{})
			Expect(ok).To(BeTrue())

			hasTools := false
			for _, cap := range capabilities {
				if cap == "tools" {
					hasTools = true
					break
				}
			}
			Expect(hasTools).To(BeTrue(), "Server should have tools capability")

			By("cleaning up")
			cmd := exec.Command("kubectl", "delete", "mcpserver", mcpServerName,
				"-n", testNamespace, "--timeout=120s")
			_, _ = utils.Run(cmd)
		})

		Context("Protocol Mismatch Detection", func() {
			It("should detect protocol mismatch with strictMode: false and keep running", func() {
				mcpServerName := "test-protocol-mismatch-nonstrict"
				mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true
  validation:
    enabled: true
    strictMode: false
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
`, mcpServerName, testNamespace)

				By("creating MCP server with protocol mismatch")
				err := applyMCPServerYAML(mcpServerYAML)
				Expect(err).NotTo(HaveOccurred())

				By("waiting for deployment to be created")
				Eventually(func() error {
					cmd := exec.Command("kubectl", "get", "deployment", mcpServerName, "-n", testNamespace)
					_, err := utils.Run(cmd)
					return err
				}, 1*time.Minute, 2*time.Second).Should(Succeed())

				By("waiting for pods to be ready")
				Eventually(func(g Gomega) {
					result, err := getMCPServerStatus(mcpServerName)
					g.Expect(err).NotTo(HaveOccurred())

					status, ok := result["status"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					readyReplicas, ok := status["readyReplicas"].(float64)
					if ok {
						g.Expect(readyReplicas).To(BeNumerically(">", 0))
					}
				}, 3*time.Minute, 5*time.Second).Should(Succeed())

				By("waiting for validation to detect mismatch")
				Eventually(func(g Gomega) {
					result, err := getMCPServerStatus(mcpServerName)
					g.Expect(err).NotTo(HaveOccurred())

					status, ok := result["status"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					validation, ok := status["validation"].(map[string]interface{})
					g.Expect(ok).To(BeTrue(), "Validation status should exist")

					state, ok := validation["state"].(string)
					g.Expect(ok).To(BeTrue())
					g.Expect(state).To(Equal("Failed"))
				}, 3*time.Minute, 5*time.Second).Should(Succeed())

				By("verifying phase is Running despite validation failure")
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal("Running"))

				By("verifying validation status shows non-compliant")
				result, err := getMCPServerStatus(mcpServerName)
				Expect(err).NotTo(HaveOccurred())

				status := result["status"].(map[string]interface{})
				validation := status["validation"].(map[string]interface{})

				compliant, ok := validation["compliant"].(bool)
				Expect(ok).To(BeTrue())
				Expect(compliant).To(BeFalse())

				By("verifying protocol mismatch issue is recorded")
				issues, ok := validation["issues"].([]interface{})
				Expect(ok).To(BeTrue())
				Expect(issues).NotTo(BeEmpty())

				foundMismatch := false
				for _, issue := range issues {
					issueMap := issue.(map[string]interface{})
					if code, ok := issueMap["code"].(string); ok && code == "PROTOCOL_MISMATCH" {
						foundMismatch = true
						Expect(issueMap["level"]).To(Equal("error"))
						message, ok := issueMap["message"].(string)
						Expect(ok).To(BeTrue())
						Expect(message).To(ContainSubstring("streamable-http"))
						Expect(message).To(ContainSubstring("sse"))
					}
				}
				Expect(foundMismatch).To(BeTrue(), "Should have PROTOCOL_MISMATCH issue")

				By("verifying detected protocol")
				transport, ok := status["transport"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				detectedProtocol, ok := transport["detectedProtocol"].(string)
				Expect(ok).To(BeTrue())
				Expect(detectedProtocol).To(Equal("sse"))

				By("verifying deployment still exists in non-strict mode")
				cmd = exec.Command("kubectl", "get", "deployment", mcpServerName, "-n", testNamespace)
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "Deployment should exist in non-strict mode")

				By("verifying pods still running")
				readyReplicas, ok := status["readyReplicas"].(float64)
				Expect(ok).To(BeTrue())
				Expect(readyReplicas).To(BeNumerically(">", 0))

				By("cleaning up")
				cmd = exec.Command("kubectl", "delete", "mcpserver", mcpServerName,
					"-n", testNamespace, "--timeout=120s")
				_, _ = utils.Run(cmd)
			})

			It("should delete deployment with protocol mismatch when strictMode: true", func() {
				mcpServerName := "test-protocol-mismatch-strict"
				mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true
  validation:
    enabled: true
    strictMode: true
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
`, mcpServerName, testNamespace)

				By("creating MCP server with protocol mismatch and strict mode")
				err := applyMCPServerYAML(mcpServerYAML)
				Expect(err).NotTo(HaveOccurred())

				By("waiting for validation to fail and phase to become ValidationFailed")
				Eventually(func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
						"-n", testNamespace, "-o", "jsonpath={.status.phase}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal("ValidationFailed"))
				}, 5*time.Minute, 5*time.Second).Should(Succeed())

				By("verifying validation status shows non-compliant")
				result, err := getMCPServerStatus(mcpServerName)
				Expect(err).NotTo(HaveOccurred())

				status := result["status"].(map[string]interface{})
				validation, ok := status["validation"].(map[string]interface{})
				Expect(ok).To(BeTrue())

				compliant, ok := validation["compliant"].(bool)
				Expect(ok).To(BeTrue())
				Expect(compliant).To(BeFalse())

				state, ok := validation["state"].(string)
				Expect(ok).To(BeTrue())
				Expect(state).To(Equal("Failed"))

				By("verifying status message indicates protocol mismatch")
				message, ok := status["message"].(string)
				Expect(ok).To(BeTrue())
				Expect(message).To(Or(
					ContainSubstring("Protocol mismatch"),
					ContainSubstring("protocol mismatch"),
				))

				By("verifying deployment was deleted in strict mode")
				Eventually(func() bool {
					cmd := exec.Command("kubectl", "get", "deployment", mcpServerName,
						"-n", testNamespace, "-o", "jsonpath={.spec.replicas}")
					output, err := utils.Run(cmd)
					// Deployment deleted or scaled to 0
					return err != nil || output == "0"
				}, 1*time.Minute, 2*time.Second).Should(BeTrue(), "Deployment should be deleted or scaled to 0 in strict mode")

				By("verifying status replica counts are zero after deployment deletion")
				result, err = getMCPServerStatus(mcpServerName)
				Expect(err).NotTo(HaveOccurred())
				status = result["status"].(map[string]interface{})

				// Replica fields have omitempty tags, so 0 values may be omitted from JSON
				// If omitted, the value is semantically 0
				replicas := float64(0)
				if val, ok := status["replicas"].(float64); ok {
					replicas = val
				}
				Expect(replicas).To(Equal(float64(0)), "Status replicas should be 0 after deployment deletion")

				readyReplicas := float64(0)
				if val, ok := status["readyReplicas"].(float64); ok {
					readyReplicas = val
				}
				Expect(readyReplicas).To(Equal(float64(0)), "Status readyReplicas should be 0 after deployment deletion")

				availableReplicas := float64(0)
				if val, ok := status["availableReplicas"].(float64); ok {
					availableReplicas = val
				}
				Expect(availableReplicas).To(Equal(float64(0)), "Status availableReplicas should be 0 after deployment deletion")

				By("verifying conditions reflect deployment deletion")
				conditions, ok := status["conditions"].([]interface{})
				Expect(ok).To(BeTrue())

				// Check Ready condition is False
				var readyCondition map[string]interface{}
				for _, cond := range conditions {
					condMap := cond.(map[string]interface{})
					if condMap["type"].(string) == "Ready" {
						readyCondition = condMap
						break
					}
				}
				Expect(readyCondition).NotTo(BeNil(), "Ready condition should exist")
				Expect(readyCondition["status"]).To(Equal("False"), "Ready condition should be False")
				Expect(readyCondition["reason"]).To(Equal("DeploymentDeleted"))

				// Check Available condition is False
				var availableCondition map[string]interface{}
				for _, cond := range conditions {
					condMap := cond.(map[string]interface{})
					if condMap["type"].(string) == "Available" {
						availableCondition = condMap
						break
					}
				}
				Expect(availableCondition).NotTo(BeNil(), "Available condition should exist")
				Expect(availableCondition["status"]).To(Equal("False"), "Available condition should be False")
				Expect(availableCondition["reason"]).To(Equal("DeploymentDeleted"))

				// Check Progressing condition is False
				var progressingCondition map[string]interface{}
				for _, cond := range conditions {
					condMap := cond.(map[string]interface{})
					if condMap["type"].(string) == "Progressing" {
						progressingCondition = condMap
						break
					}
				}
				Expect(progressingCondition).NotTo(BeNil(), "Progressing condition should exist")
				Expect(progressingCondition["status"]).To(Equal("False"), "Progressing condition should be False")
				Expect(progressingCondition["reason"]).To(Equal("DeploymentDeleted"))

				// Check Degraded condition is True
				var degradedCondition map[string]interface{}
				for _, cond := range conditions {
					condMap := cond.(map[string]interface{})
					if condMap["type"].(string) == "Degraded" {
						degradedCondition = condMap
						break
					}
				}
				Expect(degradedCondition).NotTo(BeNil(), "Degraded condition should exist")
				Expect(degradedCondition["status"]).To(Equal("True"), "Degraded condition should be True")

				By("verifying validation attempts")
				validation, ok = status["validation"].(map[string]interface{})
				Expect(ok).To(BeTrue())
				attempts, ok := validation["attempts"].(float64)
				Expect(ok).To(BeTrue())
				Expect(attempts).To(BeNumerically(">=", 2))

				By("verifying DeploymentDeleted event was emitted")
				Eventually(func(g Gomega) {
					waitForEvent(g, mcpServerName, "DeploymentDeleted", "Should have DeploymentDeleted event")
				}, 1*time.Minute, 5*time.Second).Should(Succeed())

				By("cleaning up")
				cmd := exec.Command("kubectl", "delete", "mcpserver", mcpServerName,
					"-n", testNamespace, "--timeout=120s")
				_, _ = utils.Run(cmd)
			})
		})

		Context("Recovery from Validation Failure", func() {
			It("should revalidate and become compliant after fixing protocol mismatch", func() {
				mcpServerName := "test-fix-protocol"
				mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true
  validation:
    enabled: true
    strictMode: false
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
`, mcpServerName, testNamespace)

				By("creating MCP server with wrong protocol")
				err := applyMCPServerYAML(mcpServerYAML)
				Expect(err).NotTo(HaveOccurred())

				By("waiting for validation to fail")
				Eventually(func(g Gomega) {
					result, err := getMCPServerStatus(mcpServerName)
					g.Expect(err).NotTo(HaveOccurred())

					status, ok := result["status"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					validation, ok := status["validation"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					state, ok := validation["state"].(string)
					g.Expect(ok).To(BeTrue())
					g.Expect(state).To(Equal("Failed"))
				}, 3*time.Minute, 5*time.Second).Should(Succeed())

				By("capturing initial generation")
				result, err := getMCPServerStatus(mcpServerName)
				Expect(err).NotTo(HaveOccurred())
				metadata := result["metadata"].(map[string]interface{})
				initialGeneration := metadata["generation"].(float64)

				By("fixing the protocol by updating the spec")
				mcpServerYAMLFixed := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    protocol: sse
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true
  validation:
    enabled: true
    strictMode: false
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
`, mcpServerName, testNamespace)

				err = applyMCPServerYAML(mcpServerYAMLFixed)
				Expect(err).NotTo(HaveOccurred())

				By("verifying generation was incremented")
				Eventually(func(g Gomega) {
					result, err := getMCPServerStatus(mcpServerName)
					g.Expect(err).NotTo(HaveOccurred())

					metadata := result["metadata"].(map[string]interface{})
					currentGeneration := metadata["generation"].(float64)
					g.Expect(currentGeneration).To(BeNumerically(">", initialGeneration))
				}, 30*time.Second, 2*time.Second).Should(Succeed())

				By("waiting for validation to reset and become compliant")
				Eventually(func(g Gomega) {
					result, err := getMCPServerStatus(mcpServerName)
					g.Expect(err).NotTo(HaveOccurred())

					status, ok := result["status"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					phase, ok := status["phase"].(string)
					g.Expect(ok).To(BeTrue())
					g.Expect(phase).To(Equal("Running"))

					validation, ok := status["validation"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					compliant, ok := validation["compliant"].(bool)
					g.Expect(ok).To(BeTrue())
					g.Expect(compliant).To(BeTrue())
				}, 4*time.Minute, 5*time.Second).Should(Succeed())

				By("verifying validation state is Passed")
				result, err = getMCPServerStatus(mcpServerName)
				Expect(err).NotTo(HaveOccurred())

				status := result["status"].(map[string]interface{})
				validation := status["validation"].(map[string]interface{})

				state, ok := validation["state"].(string)
				Expect(ok).To(BeTrue())
				Expect(state).To(Equal("Passed"))

				By("verifying no validation issues")
				issues, ok := validation["issues"].([]interface{})
				if ok {
					Expect(issues).To(BeEmpty())
				}

				By("verifying validation was re-attempted after fix")
				// After spec change (generation increment), validation attempts reset to 0
				// and then increment with each new validation attempt on the updated spec
				currentAttempts := validation["attempts"].(float64)
				Expect(currentAttempts).To(BeNumerically(">=", 1), "Should have attempted validation at least once after fix")

				By("verifying detected protocol matches spec")
				transport := status["transport"].(map[string]interface{})
				detectedProtocol, ok := transport["detectedProtocol"].(string)
				Expect(ok).To(BeTrue())
				Expect(detectedProtocol).To(Equal("sse"))

				By("verifying ValidationRecovery event was emitted")
				Eventually(func(g Gomega) {
					waitForEvent(g, mcpServerName, "ValidationRecovery", "Should have ValidationRecovery event")
				}, 1*time.Minute, 5*time.Second).Should(Succeed())

				By("cleaning up")
				cmd := exec.Command("kubectl", "delete", "mcpserver", mcpServerName,
					"-n", testNamespace, "--timeout=120s")
				_, _ = utils.Run(cmd)
			})

			It("should recover by switching to auto-detection", func() {
				mcpServerName := "test-fix-auto-detection"
				mcpServerYAML := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    protocol: streamable-http
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true
  validation:
    enabled: true
    strictMode: false
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
`, mcpServerName, testNamespace)

				By("creating MCP server with wrong protocol")
				err := applyMCPServerYAML(mcpServerYAML)
				Expect(err).NotTo(HaveOccurred())

				By("waiting for validation to fail")
				Eventually(func(g Gomega) {
					result, err := getMCPServerStatus(mcpServerName)
					g.Expect(err).NotTo(HaveOccurred())

					status, ok := result["status"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					validation, ok := status["validation"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					compliant, ok := validation["compliant"].(bool)
					g.Expect(ok).To(BeTrue())
					g.Expect(compliant).To(BeFalse())
				}, 3*time.Minute, 5*time.Second).Should(Succeed())

				By("switching to auto protocol detection")
				mcpServerYAMLAuto := fmt.Sprintf(`
apiVersion: mcp.mcp-operator.io/v1
kind: MCPServer
metadata:
  name: %s
  namespace: %s
spec:
  image: tzolov/mcp-everything-server:v3
  command: ["node", "dist/index.js", "sse"]
  replicas: 1
  transport:
    type: http
    protocol: auto
    config:
      http:
        port: 3001
        path: "/sse"
        sessionManagement: true
  validation:
    enabled: true
    strictMode: false
  security:
    runAsUser: 1000
    runAsGroup: 1000
    runAsNonRoot: true
    allowPrivilegeEscalation: false
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "512Mi"
`, mcpServerName, testNamespace)

				err = applyMCPServerYAML(mcpServerYAMLAuto)
				Expect(err).NotTo(HaveOccurred())

				By("waiting for validation to become compliant")
				Eventually(func(g Gomega) {
					result, err := getMCPServerStatus(mcpServerName)
					g.Expect(err).NotTo(HaveOccurred())

					status, ok := result["status"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					validation, ok := status["validation"].(map[string]interface{})
					g.Expect(ok).To(BeTrue())

					compliant, ok := validation["compliant"].(bool)
					g.Expect(ok).To(BeTrue())
					g.Expect(compliant).To(BeTrue())

					state, ok := validation["state"].(string)
					g.Expect(ok).To(BeTrue())
					g.Expect(state).To(Equal("Passed"))
				}, 4*time.Minute, 5*time.Second).Should(Succeed())

				By("verifying server is running")
				cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
					"-n", testNamespace, "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
				Expect(output).To(Equal("Running"))

				By("cleaning up")
				cmd = exec.Command("kubectl", "delete", "mcpserver", mcpServerName,
					"-n", testNamespace, "--timeout=120s")
				_, _ = utils.Run(cmd)
			})
		})
	})
})

// serviceAccountToken returns a token for the specified service account in the given namespace.
// It uses the Kubernetes TokenRequest API to generate a token by directly sending a request
// and parsing the resulting token from the API response.
func serviceAccountToken() (string, error) {
	const tokenRequestRawString = `{
		"apiVersion": "authentication.k8s.io/v1",
		"kind": "TokenRequest"
	}`

	// Temporary file to store the token request
	secretName := fmt.Sprintf("%s-token-request", serviceAccountName)
	tokenRequestFile := filepath.Join("/tmp", secretName)
	err := os.WriteFile(tokenRequestFile, []byte(tokenRequestRawString), os.FileMode(0o644))
	if err != nil {
		return "", err
	}

	var out string
	verifyTokenCreation := func(g Gomega) {
		// Execute kubectl command to create the token
		cmd := exec.Command("kubectl", "create", "--raw", fmt.Sprintf(
			"/api/v1/namespaces/%s/serviceaccounts/%s/token",
			operatorNamespace,
			serviceAccountName,
		), "-f", tokenRequestFile)

		output, err := cmd.CombinedOutput()
		g.Expect(err).NotTo(HaveOccurred())

		// Parse the JSON output to extract the token
		var token tokenRequest
		err = json.Unmarshal(output, &token)
		g.Expect(err).NotTo(HaveOccurred())

		out = token.Status.Token
	}
	Eventually(verifyTokenCreation).Should(Succeed())

	return out, err
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() string {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", operatorNamespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
	return metricsOutput
}

// fetchFreshMetrics creates a temporary pod to fetch current metrics.
// This is useful when you need fresh metrics after an operation (like validation).
// Unlike getMetricsOutput(), this creates a new pod each time to ensure fresh data.
func fetchFreshMetrics() string {
	// Generate unique pod name
	podName := fmt.Sprintf("metrics-fetch-%d", time.Now().Unix())

	// Get token for authentication
	token, err := serviceAccountToken()
	Expect(err).NotTo(HaveOccurred(), "Failed to get service account token")

	By(fmt.Sprintf("creating temporary pod %s to fetch fresh metrics", podName))
	cmd := exec.Command("kubectl", "run", podName, "--restart=Never",
		"--namespace", operatorNamespace,
		"--image=curlimages/curl:latest",
		"--overrides",
		fmt.Sprintf(`{
			"spec": {
				"containers": [{
					"name": "curl",
					"image": "curlimages/curl:latest",
					"command": ["/bin/sh", "-c"],
					"args": ["curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics"],
					"securityContext": {
						"readOnlyRootFilesystem": true,
						"allowPrivilegeEscalation": false,
						"capabilities": {
							"drop": ["ALL"]
						},
						"runAsNonRoot": true,
						"runAsUser": 1000,
						"seccompProfile": {
							"type": "RuntimeDefault"
						}
					}
				}],
				"serviceAccountName": "%s"
			}
		}`, token, metricsServiceName, operatorNamespace, serviceAccountName))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to create %s pod", podName))

	By(fmt.Sprintf("waiting for %s pod to complete", podName))
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pods", podName,
			"-o", "jsonpath={.status.phase}",
			"-n", operatorNamespace)
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("Succeeded"), "curl pod in wrong status")
	}, 1*time.Minute, 2*time.Second).Should(Succeed())

	By(fmt.Sprintf("getting metrics from %s logs", podName))
	cmd = exec.Command("kubectl", "logs", podName, "-n", operatorNamespace)
	metricsOutput, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from metrics fetch pod")
	Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))

	// Clean up the temporary pod
	By(fmt.Sprintf("deleting temporary pod %s", podName))
	cmd = exec.Command("kubectl", "delete", "pod", podName, "-n", operatorNamespace, "--wait=false")
	_, _ = utils.Run(cmd) // Ignore errors on cleanup

	return metricsOutput
}

// tokenRequest is a simplified representation of the Kubernetes TokenRequest API response,
// containing only the token field that we need to extract.
type tokenRequest struct {
	Status struct {
		Token string `json:"token"`
	} `json:"status"`
}

// Helper functions for MCPServer tests

// applyMCPServerYAML creates an MCPServer from YAML string
func applyMCPServerYAML(yamlContent string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = os.NewFile(0, "")
	// Write YAML to stdin
	tmpFile, err := os.CreateTemp("", "mcpserver-*.yaml")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()

	if _, err := tmpFile.WriteString(yamlContent); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	cmd = exec.Command("kubectl", "apply", "-f", tmpFile.Name())
	_, err = utils.Run(cmd)
	return err
}

// getMCPServerStatus fetches MCPServer status
func getMCPServerStatus(name string) (map[string]interface{}, error) {
	cmd := exec.Command("kubectl", "get", "mcpserver", name,
		"-n", testNamespace, "-o", "json")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, err
	}

	return result, nil
}

// findCondition finds a condition by type in status
func findCondition(status map[string]interface{}, condType string) map[string]interface{} {
	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return nil
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == condType {
			return cond
		}
	}
	return nil
}

// waitForEvent waits for a specific event reason to appear for an MCPServer
func waitForEvent(g Gomega, mcpServerName, eventReason, errorMsg string) {
	cmd := exec.Command("kubectl", "get", "events",
		"-n", testNamespace,
		"--field-selector", fmt.Sprintf("involvedObject.name=%s", mcpServerName),
		"-o", "json")
	output, err := utils.Run(cmd)
	g.Expect(err).NotTo(HaveOccurred())

	var events map[string]interface{}
	err = json.Unmarshal([]byte(output), &events)
	g.Expect(err).NotTo(HaveOccurred())

	items, ok := events["items"].([]interface{})
	g.Expect(ok).To(BeTrue())

	foundEvent := false
	for _, item := range items {
		event := item.(map[string]interface{})
		reason, ok := event["reason"].(string)
		if ok && reason == eventReason {
			foundEvent = true
			break
		}
	}
	g.Expect(foundEvent).To(BeTrue(), errorMsg)
}

// waitForMCPServerRunning waits for MCPServer to reach Running phase
func waitForMCPServerRunning(mcpServerName string) {
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "mcpserver", mcpServerName,
			"-n", testNamespace, "-o", "jsonpath={.status.phase}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(output).To(Equal("Running"))
	}, 2*time.Minute, 5*time.Second).Should(Succeed())
}

// getDeploymentField gets a specific field from a deployment using jsonpath
func getDeploymentField(deploymentName, jsonPath string) string {
	cmd := exec.Command("kubectl", "get", "deployment", deploymentName,
		"-n", testNamespace, "-o", fmt.Sprintf("jsonpath={%s}", jsonPath))
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	return output
}
