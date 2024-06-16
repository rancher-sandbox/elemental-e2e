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
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/rancher"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/elemental/tests/e2e/helpers/elemental"
)

const (
	capiRegistrationYaml = "../assets/capi_elementalRegistration.yaml"
	clusterctlYaml       = "../assets/clusterctl.yaml"
	ciTokenYaml          = "../assets/local-kubeconfig-token-skel.yaml"
	elementalAPIYaml     = "../assets/elemental_capi_api.yaml"
	emulateTPMYaml       = "../assets/emulateTPM.yaml"
	httpSrv              = "http://192.168.122.1:8000"
	installConfigYaml    = "../../install-config.yaml"
	installVMScript      = "../scripts/install-vm"
	numberOfNodesMax     = 30
	userName             = "root"
	userPassword         = "r0s@pwd1"
	vmNameRoot           = "node"
)

var (
	clusterName          string
	clusterNS            string
	clusterType          string
	clusterYaml          string
	elementalAPIEndpoint string
	elementalSupport     string
	emulateTPM           bool
	isoBoot              bool
	k8sUpstreamVersion   string
	k8sDownstreamVersion string
	netDefaultFileName   string
	numberOfVMs          int
	operatorRepo         string
	operatorType         string
	registrationYaml     string
	testCaseID           int64
	testType             string
	usedNodes            int
	vmIndex              int
	vmName               string
)

/*
Wait for cluster to be in a stable state
  - @param ns Namespace where the cluster is deployed
  - @param cn Cluster resource name
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func WaitCAPICluster(ns, cn string) {
	type state struct {
		conditionStatus string
		conditionType   string
	}

	// List of conditions to check
	states := []state{
		{
			conditionStatus: "true",
			conditionType:   "controlPlaneReady",
		},
		{
			conditionStatus: "true",
			conditionType:   "infrastructureReady",
		},
	}

	// Check that all needed conditions are in the good state
	for _, s := range states {
		counter := 0

		Eventually(func() string {
			status, _ := kubectl.RunWithoutErr("get", "cluster",
				"--namespace", ns, cn,
				"-o", "jsonpath={.status."+s.conditionType+"}")

			if status != s.conditionStatus {
				// Show the status in case of issue, easier to debug (but log after 10 different issues)
				// NOTE: it's not perfect but it's mainly a way to inform that the cluster took time to came up
				counter++
				if counter > 10 {
					GinkgoWriter.Printf("!! Cluster status issue !! %s is %s instead of %s\n",
						s.conditionType, status, s.conditionStatus)

					// Reset counter
					counter = 0
				}
			}

			return status
		}, tools.SetTimeout(2*time.Duration(usedNodes)*time.Minute), 10*time.Second).Should(Equal(s.conditionStatus))
	}
}

/*
Wait for elemental resource to be in a ready state
  - @param ns Namespace where the cluster is deployed
  - @param er Elemental resource type
  - @param rs Elemental resource name
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func WaitElementalResources(ns, er, rs string) {
	type state struct {
		conditionStatus string
		conditionType   string
	}

	// List of conditions to check
	elementalhostStates := []state{
		{
			conditionStatus: "True",
			conditionType:   "RegistrationReady",
		},
		{
			conditionStatus: "True",
			conditionType:   "InstallationReady",
		},
		{
			conditionStatus: "True",
			conditionType:   "BootstrapReady",
		},
		{
			conditionStatus: "True",
			conditionType:   "Ready",
		},
	}

	// List of conditions to check
	elementalmachineStates := []state{
		{
			conditionStatus: "True",
			conditionType:   "AssociationReady",
		},
		{
			conditionStatus: "True",
			conditionType:   "HostReady",
		},
		{
			conditionStatus: "True",
			conditionType:   "ProviderIDReady",
		},
		{
			conditionStatus: "True",
			conditionType:   "Ready",
		},
	}

	// Check that all needed conditions are in the good state
	if er == "elementalhost" {
		for _, s := range elementalhostStates {
			CheckCondition(ns, er, rs, s.conditionType, s.conditionStatus)
		}
	} else {
		for _, s := range elementalmachineStates {
			CheckCondition(ns, er, rs, s.conditionType, s.conditionStatus)
		}
	}
}

/*
Check that condition is in the expected state
  - @param ns Namespace where the cluster is deployed
  - @param er Elemental resource type
  - @param rs Elemental resource name
  - @param ct Condition type
  - @param cs Condition status
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func CheckCondition(ns, er, rs, ct, cs string) {
	Eventually(func() string {
		status, _ := kubectl.RunWithoutErr("get", er,
			"--namespace", ns, rs,
			"-o", "jsonpath={.status.conditions[?(@.type==\""+ct+"\")].status}")
		if status != cs {
			// Show the status in case of issue, easier to debug
			GinkgoWriter.Printf("!! %s status issue !! %s is %s instead of %s\n",
				er, ct, status, cs)
		}
		return status
	}, tools.SetTimeout(2*time.Duration(usedNodes)*time.Minute), 20*time.Second).Should(Equal(cs))
}

/*
Check that Registration resource has been correctly created
  - @param ns Namespace where the cluster is deployed
  - @param rn Registration resource name
  - @param op Operator type (capi or vanilla)
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func CheckCreatedRegistration(ns, rn string) {
	Eventually(func() string {
		registration := "MachineRegistration"
		if operatorType == "capi" {
			registration = "ElementalRegistration"
		}
		out, _ := kubectl.RunWithoutErr("get", registration,
			"--namespace", ns,
			"-o", "jsonpath={.items[*].metadata.name}")
		return out
	}, tools.SetTimeout(3*time.Minute), 5*time.Second).Should(ContainSubstring(rn))
}

/*
Check SSH connection
  - @param cl Client (node) informations
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func CheckSSH(cl *tools.Client) {
	Eventually(func() string {
		out, _ := cl.RunSSH("echo SSH_OK")
		return strings.Trim(out, "\n")
	}, tools.SetTimeout(10*time.Minute), 5*time.Second).Should(Equal("SSH_OK"))
}

/*
Get Elemental node information
  - @param hn Node hostname
  - @returns Client structure and MAC address
*/
func GetNodeInfo(hn string) (*tools.Client, string) {
	// Get network data
	data, err := rancher.GetHostNetConfig(".*name=\""+hn+"\".*", netDefaultFileName)
	Expect(err).To(Not(HaveOccurred()))

	// Set 'client' to be able to access the node through SSH
	c := &tools.Client{
		Host:     string(data.IP) + ":22",
		Username: userName,
		Password: userPassword,
	}

	return c, data.Mac
}

/*
Get Elemental node IP address
  - @param hn Node hostname
  - @returns IP address
*/
func GetNodeIP(hn string) string {
	// Get network data
	data, err := rancher.GetHostNetConfig(".*name=\""+hn+"\".*", netDefaultFileName)
	Expect(err).To(Not(HaveOccurred()))

	return data.IP
}

/*
Execute RunHelmBinaryWithCustomErr within a loop with timeout
  - @param s options to pass to RunHelmBinaryWithCustomErr command
  - @returns Nothing, the function will fail through Ginkgo in case of issue
*/
func RunHelmCmdWithRetry(s ...string) {
	Eventually(func() error {
		return kubectl.RunHelmBinaryWithCustomErr(s...)
	}, tools.SetTimeout(2*time.Minute), 20*time.Second).Should(Not(HaveOccurred()))
}

/*
Execute SSH command with retry
  - @param cl Client (node) informations
  - @param cmd Command to execute
  - @returns result of the executed command
*/
func RunSSHWithRetry(cl *tools.Client, cmd string) string {
	var err error
	var out string

	Eventually(func() error {
		out, err = cl.RunSSH(cmd)
		return err
	}, tools.SetTimeout(2*time.Minute), 20*time.Second).Should(Not(HaveOccurred()))

	return out
}

func FailWithReport(message string, callerSkip ...int) {
	// Ensures the correct line numbers are reported
	Fail(message, callerSkip[0]+1)
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Elemental End-To-End Test Suite")
}

// Use to modify yaml templates
type YamlPattern struct {
	key   string
	value string
}

var _ = BeforeSuite(func() {
	bootTypeString := os.Getenv("BOOT_TYPE")
	clusterName = os.Getenv("CLUSTER_NAME")
	clusterNS = os.Getenv("CLUSTER_NS")
	clusterType = os.Getenv("CLUSTER_TYPE")
	elementalAPIEndpoint = os.Getenv("ELEMENTAL_API_ENDPOINT")
	elementalSupport = os.Getenv("ELEMENTAL_SUPPORT")
	eTPM := os.Getenv("EMULATE_TPM")
	index := os.Getenv("VM_INDEX")
	k8sDownstreamVersion = os.Getenv("K8S_DOWNSTREAM_VERSION")
	k8sUpstreamVersion = os.Getenv("K8S_UPSTREAM_VERSION")
	number := os.Getenv("VM_NUMBERS")
	operatorRepo = os.Getenv("OPERATOR_REPO")
	operatorType = os.Getenv("OPERATOR_TYPE")
	testType = os.Getenv("TEST_TYPE")

	// Only if VM_INDEX is set
	if index != "" {
		var err error
		vmIndex, err = strconv.Atoi(index)
		Expect(err).To(Not(HaveOccurred()))

		// Set default hostname
		vmName = elemental.SetHostname(vmNameRoot, vmIndex)
	} else {
		// Default value for vmIndex
		vmIndex = 0
	}

	// Only if VM_NUMBER is set
	if number != "" {
		var err error
		numberOfVMs, err = strconv.Atoi(number)
		Expect(err).To(Not(HaveOccurred()))
	} else {
		// By default set to vmIndex
		numberOfVMs = vmIndex
	}

	// Set number of "used" nodes
	// NOTE: could be the number added nodes or the number of nodes to use/upgrade
	usedNodes = (numberOfVMs - vmIndex) + 1

	// Force correct value for emulateTPM
	switch eTPM {
	case "true":
		emulateTPM = true
	default:
		emulateTPM = false
	}

	// Define boot type
	switch bootTypeString {
	case "iso":
		isoBoot = true
	}

	switch testType {
	default:
		// Default cluster support
		clusterYaml = "../assets/cluster.yaml"
		netDefaultFileName = "../assets/net-default-capi.xml"
		registrationYaml = "../assets/capi_elementalRegistration.yaml"
	}

	// Start HTTP server
	tools.HTTPShare("../..", ":8000")
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})
