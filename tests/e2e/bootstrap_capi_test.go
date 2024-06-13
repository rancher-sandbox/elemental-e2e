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
	"os/exec"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/rancher"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	"github.com/rancher/elemental/tests/e2e/helpers/elemental"
	"github.com/rancher/elemental/tests/e2e/helpers/misc"
	"github.com/rancher/elemental/tests/e2e/helpers/network"
)

var _ = Describe("E2E - Bootstrapping node", Label("bootstrap"), func() {
	var (
		bootstrappedNodes int
		wg                sync.WaitGroup
	)

	It("Provision the node", func() {
		// Report to Qase
		testCaseID = 9

		if !isoBoot {
			By("Downloading MachineRegistration file", func() {
				// Download the new YAML installation config file
				machineRegName := "machine-registration-master-" + clusterName
				tokenURL, err := kubectl.RunWithoutErr("get", "MachineRegistration",
					"--namespace", clusterNS, machineRegName,
					"-o", "jsonpath={.status.registrationURL}")
				Expect(err).To(Not(HaveOccurred()))

				Eventually(func() error {
					return tools.GetFileFromURL(tokenURL, installConfigYaml, false)
				}, tools.SetTimeout(2*time.Minute), 10*time.Second).ShouldNot(HaveOccurred())
			})

			By("Configuring iPXE boot script for network installation", func() {
				numberOfFile, err := network.ConfigureiPXE(httpSrv)
				Expect(err).To(Not(HaveOccurred()))
				Expect(numberOfFile).To(BeNumerically(">=", 1))
			})
		}

		// Loop on node provisionning
		// NOTE: if numberOfVMs == vmIndex then only one node will be provisionned
		bootstrappedNodes = 0
		for index := vmIndex; index <= numberOfVMs; index++ {
			// Set node hostname
			hostName := elemental.SetHostname(vmNameRoot, index)
			Expect(hostName).To(Not(BeEmpty()))

			// Add node in network configuration
			err := rancher.AddNode(netDefaultFileName, hostName, index)
			Expect(err).To(Not(HaveOccurred()))

			// Get generated MAC address
			_, macAdrs := GetNodeInfo(hostName)
			Expect(macAdrs).To(Not(BeEmpty()))

			wg.Add(1)
			go func(s, h, m string, i int) {
				defer wg.Done()
				defer GinkgoRecover()

				By("Installing node "+h, func() {
					// Execute node deployment in parallel
					err := exec.Command(s, h, m).Run()
					Expect(err).To(Not(HaveOccurred()))
				})
			}(installVMScript, hostName, macAdrs, index)

			// Wait a bit before starting more nodes to reduce CPU and I/O load
			bootstrappedNodes = misc.WaitNodesBoot(index, vmIndex, bootstrappedNodes, numberOfNodesMax)
		}

		// Wait for all parallel jobs
		wg.Wait()
	})

	It("Add the nodes in the cluster", func() {
		bootstrappedNodes = 0
		for index := vmIndex; index <= numberOfVMs; index++ {
			// Set node hostname
			hostName := elemental.SetHostname(vmNameRoot, index)
			Expect(hostName).To(Not(BeEmpty()))

			// Get node information
			client, _ := GetNodeInfo(hostName)
			Expect(client).To(Not(BeNil()))

			// Execute in parallel
			wg.Add(1)
			go func(c, h string, i int, t bool, cl *tools.Client) {
				defer wg.Done()
				defer GinkgoRecover()

				// Restart the node(s)
				By("Restarting "+h+" to add it in the cluster", func() {

					err := exec.Command("sudo", "virsh", "start", h).Run()
					Expect(err).To(Not(HaveOccurred()))
				})

				By("Checking "+h+" SSH connection", func() {
					CheckSSH(cl)
				})

				By("Checking that TPM is correctly configured on "+h, func() {
					testValue := "-c"
					if t == true {
						testValue = "! -e"
					}
					_ = RunSSHWithRetry(cl, "[[ "+testValue+" /dev/tpm0 ]]")
				})

				By("Checking OS version on "+h, func() {
					out := RunSSHWithRetry(cl, "cat /etc/os-release")
					GinkgoWriter.Printf("OS Version on %s:\n%s\n", h, out)
				})
			}(clusterNS, hostName, index, emulateTPM, client)

			// Wait a bit before starting more nodes to reduce CPU and I/O load
			bootstrappedNodes = misc.WaitNodesBoot(index, vmIndex, bootstrappedNodes, numberOfNodesMax)
		}

		// Wait for all parallel jobs
		wg.Wait()

		// TODO: check if elemental hosts are
		/*
			kubectl get elementalhost -A
			NAMESPACE   NAME                                     CLUSTER   MACHINE                    ELEMENTALMACHINE           PHASE     READY   AGE
			default     m-b9de3abc-378d-46d4-989c-5594215f6c7f   rke2      rke2-control-plane-jz896   rke2-control-plane-5wp6x   Running   True    7m34s
			default     m-2b2d48a9-4401-4b59-9553-3aaa4856e5e0                                                                   Running   True    6m2s
		*/

		// TODO: check if machines are Running
		/*
			management-host:~ # kubectl get machines
			NAME                       CLUSTER   NODENAME                                 PROVIDERID                                                   PHASE     AGE   VERSION
			rke2-control-plane-jz896   rke2      m-b9de3abc-378d-46d4-989c-5594215f6c7f   elemental://default/m-b9de3abc-378d-46d4-989c-5594215f6c7f   Running   22m   v1.30.1+rke2r1
			rke2-md-0-tmq6b-4xpxz      rke2      m-2b2d48a9-4401-4b59-9553-3aaa4856e5e0   elemental://default/m-2b2d48a9-4401-4b59-9553-3aaa4856e5e0   Running   22m   v1.30.1+rke2r1
		*/

		By("Checking cluster state", func() {
			WaitCAPICluster("default", clusterName)
		})
	})
})
