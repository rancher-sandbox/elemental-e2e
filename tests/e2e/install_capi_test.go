/*
Copyright © 2022 - 2024 SUSE LLC

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

package e2e_test

import (
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/rancher"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
)

var _ = Describe("E2E - Install CAPI", Label("install-capi"), func() {
	// Create kubectl context
	// Default timeout is too small, so New() cannot be used
	k := &kubectl.Kubectl{
		Namespace:    "",
		PollTimeout:  tools.SetTimeout(300 * time.Second),
		PollInterval: 500 * time.Millisecond,
	}

	// Define local Kubeconfig file
	localKubeconfig := os.Getenv("HOME") + "/.kube/config"

	It("Install CAPI components", func() {
		password := "root"
		userName := "root"
		archiveName := "cluster-api-provider-elemental"

		// For ssh access
		client := &tools.Client{
			Host:     "192.168.122.100:22",
			Username: userName,
			Password: password,
		}

		err := os.Setenv("KUBECONFIG", localKubeconfig)
		Expect(err).To(Not(HaveOccurred()))

		By("Installing and configuring clusterctl", func() {
			err := exec.Command("curl", "-sLO", "https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.5.3/clusterctl-linux-amd64").Run()
			Expect(err).To(Not(HaveOccurred()))
			err = exec.Command("sudo", "install", "-o", "root", "-g", "root", "-m", "0755", "clusterctl-linux-amd64", "/usr/local/bin/clusterctl").Run()
			Expect(err).To(Not(HaveOccurred()))
			err = exec.Command("bash", "-c", "mkdir -p $HOME/.cluster-api").Run()
			Expect(err).To(Not(HaveOccurred()))
			err = exec.Command("bash", "-c", "cp "+clusterctlYaml+" $HOME/.cluster-api").Run()
			Expect(err).To(Not(HaveOccurred()))
		})

		By("Compiling latest elemental CAPI provider", func() {
			err := os.Chdir("../../cluster-api-provider-elemental")
			Expect(err).To(Not(HaveOccurred()))
			err = exec.Command("make", "docker-build").Run()
			Expect(err).To(Not(HaveOccurred()))
			err = exec.Command("bash", "-c", "docker save ghcr.io/rancher-sandbox/cluster-api-provider-elemental -o "+archiveName).Run()
			Expect(err).To(Not(HaveOccurred()))
			err = client.SendFile(archiveName, "/tmp/"+archiveName, "0644")
			Expect(err).To(Not(HaveOccurred()))
			_, err = client.RunSSH("/usr/local/bin/k3s ctr images import /tmp/" + archiveName)
			Expect(err).To(Not(HaveOccurred()))
		})

		By("Installing CAPI core, control plane and bootstrap providers", func() {
			out, err := exec.Command("/usr/local/bin/clusterctl", "--v", "4", "init", "--bootstrap", "rke2", "--control-plane", "rke2", "--infrastructure", "elemental:v0.0.0").CombinedOutput()
			// Show command output, easier to debug
			GinkgoWriter.Printf("%s\n", string(out))
			Expect(err).To(Not(HaveOccurred()))
			// Wait for all pods to be started
			checkList := [][]string{
				{"cert-manager", "app.kubernetes.io/component=controller"},
				{"cert-manager", "app.kubernetes.io/component=webhook"},
				{"cert-manager", "app.kubernetes.io/component=cainjector"},
				{"capi-system", "control-plane=controller-manager"},
				{"rke2-bootstrap-system", "cluster.x-k8s.io/provider=bootstrap-rke2"},
				{"rke2-control-plane-system", "cluster.x-k8s.io/provider=control-plane-rke2"},
				{"elemental-system", "control-plane=controller-manager"},
			}
			Eventually(func() error {
				return rancher.CheckPod(k, checkList)
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})

		By("Exposing Elemental API server", func() {
			err := os.Chdir("../tests/e2e")
			Expect(err).To(Not(HaveOccurred()))
			err = kubectl.Apply("elemental-system", elementalAPIYaml)
			Expect(err).To(Not(HaveOccurred()))
			// TODO: not needed but can be usefull
			// check if service is ready
		})

		By("Creating Elemental cluster", func() {
			err := exec.Command("bash", "-c", "clusterctl generate cluster --control-plane-machine-count=1 --worker-machine-count=2 --infrastructure elemental:v0.0.0 --flavor rke2 "+clusterName+" --kubernetes-version="+k8sDownstreamVersion+"> ~/rke2-cluster-manifest.yaml").Run()
			Expect(err).To(Not(HaveOccurred()))
			err = kubectl.Apply("", "/home/gh-runner/rke2-cluster-manifest.yaml")
			Expect(err).To(Not(HaveOccurred()))
			// Check elementalmachine resources?
		})

		By("Creating Elemental Machine registration", func() {
			// Set temporary file
			registrationTmp, err := tools.CreateTemp("machineRegistration")
			Expect(err).To(Not(HaveOccurred()))
			//defer os.Remove(registrationTmp)

			// Remove quotes from the url
			url := strings.Trim(elementalAPIEndpoint, "\"\"")

			patterns := []YamlPattern{
				{
					key:   "%CLUSTER_NAME%",
					value: clusterName,
				},
				{
					key:   "%PASSWORD%",
					value: userPassword,
				},
				{
					key:   "%ELEMENTAL_API_ENDPOINT%",
					value: url,
				},
				{
					key:   "%USER%",
					value: userName,
				},
				{
					key:   "%VM_NAME%",
					value: vmNameRoot,
				},
			}

			// Save original file as it will have to be modified twice
			err = tools.CopyFile(registrationYaml, registrationTmp)
			Expect(err).To(Not(HaveOccurred()))

			// Create Yaml file
			for _, p := range patterns {
				err := tools.Sed(p.key, p.value, registrationTmp)
				Expect(err).To(Not(HaveOccurred()))
			}

			// Apply to k8s
			err = kubectl.Apply("default", registrationTmp)
			Expect(err).To(Not(HaveOccurred()))

			// Check that the machine registration is correctly created
			CheckCreatedRegistration("default", "machine-registration-master-"+clusterName)

			// Generate the config files
			// TODO: replace sleep with a check
			time.Sleep(2 * time.Minute)
			err = os.Chdir("../../cluster-api-provider-elemental")
			Expect(err).To(Not(HaveOccurred()))
			err = exec.Command("bash", "-c", "./test/scripts/print_agent_config.sh -n default -r machine-registration-master-"+clusterName+" > iso/config/my-config.yaml").Run()
			Expect(err).To(Not(HaveOccurred()))

		})
	})
})
