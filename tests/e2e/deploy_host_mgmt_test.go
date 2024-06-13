/*
Copyright © 2022 - 2023 SUSE LLC

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/rancher"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
)

var _ = Describe("E2E - Deploy management host with K3S", Label("install-mgmt-host"), func() {
	It("Create the management host machine", func() {
		By("Updating the default network configuration", func() {
			// Don't check return code, as the default network could be already removed
			for _, c := range []string{"net-destroy", "net-undefine"} {
				_ = exec.Command("sudo", "virsh", c, "default").Run()
			}

			// Wait a bit between virsh commands
			time.Sleep(30 * time.Second)
			err := exec.Command("sudo", "virsh", "net-create", netDefaultFileName).Run()
			Expect(err).To(Not(HaveOccurred()))
		})

		By("Creating the host management VM", func() {
			err := exec.Command("sudo", "virt-install",
				"--name", "management-host",
				"--memory", "16384",
				"--vcpus", "4",
				"--disk", "path="+os.Getenv("HOME")+"/rancher-image.qcow2,bus=sata",
				"--import",
				"--os-variant", "opensuse-unknown",
				"--network=default,mac=52:54:00:00:00:10",
				"--noautoconsole").Run()
			Expect(err).To(Not(HaveOccurred()))
		})
	})

	It("Install K3S in the management-host machine", func() {
		password := "root"
		userName := "root"

		// For ssh access
		client := &tools.Client{
			Host:     "192.168.122.100:22",
			Username: userName,
			Password: password,
		}

		// Create kubectl context
		// Default timeout is too small, so New() cannot be used
		k := &kubectl.Kubectl{
			Namespace:    "",
			PollTimeout:  tools.SetTimeout(300 * time.Second),
			PollInterval: 500 * time.Millisecond,
		}

		By("Installing K3S", func() {
			// Make sure SSH is available
			CheckSSH(client)

			// Create the destination repository
			_, err := client.RunSSH("INSTALL_K3S_VERSION=" + k8sUpstreamVersion + " bash -c 'curl -sfL https://get.k3s.io | sh -'")
			Expect(err).To(Not(HaveOccurred()))
		})

		By("Getting the kubeconfig file of the airgap cluster", func() {
			// Define local Kubeconfig file
			localKubeconfig := os.Getenv("HOME") + "/.kube/config"

			err := os.Mkdir(os.Getenv("HOME")+"/.kube", 0755)
			Expect(err).To(Not(HaveOccurred()))

			err = client.GetFile(localKubeconfig, "/etc/rancher/k3s/k3s.yaml", 0644)
			Expect(err).To(Not(HaveOccurred()))
			// NOTE: not sure that this is need because we have the config file in ~/.kube/

			err = os.Setenv("KUBECONFIG", localKubeconfig)
			Expect(err).To(Not(HaveOccurred()))

			// Replace localhost with the IP of the VM
			err = tools.Sed("127.0.0.1", "192.168.122.100", localKubeconfig)
			Expect(err).To(Not(HaveOccurred()))
		})

		By("Installing kubectl", func() {
			// TODO: Variable for kubectl version
			err := exec.Command("curl", "-sLO", "https://dl.k8s.io/release/v1.28.2/bin/linux/amd64/kubectl").Run()
			Expect(err).To(Not(HaveOccurred()))
			err = exec.Command("chmod", "+x", "kubectl").Run()
			Expect(err).To(Not(HaveOccurred()))
			err = exec.Command("sudo", "mv", "kubectl", "/usr/local/bin/").Run()
			Expect(err).To(Not(HaveOccurred()))
		})
		By("Waiting for K3s to be started", func() {
			// Wait for all pods to be started
			checkList := [][]string{
				{"kube-system", "app=local-path-provisioner"},
				{"kube-system", "k8s-app=kube-dns"},
				{"kube-system", "app.kubernetes.io/name=traefik"},
				{"kube-system", "svccontroller.k3s.cattle.io/svcname=traefik"},
			}
			Eventually(func() error {
				return rancher.CheckPod(k, checkList)
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})
	})
})
