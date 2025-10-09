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
		cmd = exec.Command("kubectl", "delete", "ns", testNamespace)
		_, _ = utils.Run(cmd)

		By("removing monitoring resources")
		cmd = exec.Command("kubectl", "delete", "-f", "../../dist/monitoring.yaml", "--ignore-not-found")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("removing operator namespace")
		cmd = exec.Command("kubectl", "delete", "ns", operatorNamespace)
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
				result, err := getMCPServerStatus(mcpServerName, testNamespace)
				g.Expect(err).NotTo(HaveOccurred())

				status, ok := result["status"].(map[string]interface{})
				g.Expect(ok).To(BeTrue())

				readyCond := findCondition(status, "Ready")
				g.Expect(readyCond).NotTo(BeNil(), "Ready condition should exist")
				g.Expect(readyCond["status"]).To(Equal("True"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying Available condition is set")
			result, err := getMCPServerStatus(mcpServerName, testNamespace)
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
func getMCPServerStatus(name, ns string) (map[string]interface{}, error) {
	cmd := exec.Command("kubectl", "get", "mcpserver", name,
		"-n", ns, "-o", "json")
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
