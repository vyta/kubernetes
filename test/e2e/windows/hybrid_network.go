/*
Copyright 2018 The Kubernetes Authors.

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

package windows

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/test/e2e/framework"
	imageutils "k8s.io/kubernetes/test/utils/image"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const (
	linuxOS    = "linux"
	windowsOS  = "windows"
	iterations = 5
)

var (
	windowsBusyBoximage = imageutils.GetE2EImage(imageutils.TestWebserver)
	linuxBusyBoxImage   = "docker.io/library/nginx:1.15-alpine"
)

var _ = SIGDescribe("Hybrid cluster network", func() {
	f := framework.NewDefaultFramework("hybrid-network")

	BeforeEach(func() {
		framework.SkipUnlessNodeOSDistroIs("windows")
	})

	Context("for all supported CNIs", func() {

		It("should have stable networking for linux and windows pods", func() {
			By("creating linux and windows pods")
			linuxPod := createTestPod(f, linuxBusyBoxImage, linuxOS)
			windowsPod := createTestPod(f, windowsBusyBoximage, windowsOS)

			By("checking connectivity to 8.8.8.8 53 (google.com) from linux")
			checkLinuxConnectivity(f, linuxPod.ObjectMeta.Name, "8.8.8.8", "53")

			By("checking connectivity to www.google.com from windows")
			checkWindowsConnectivity(f, windowsPod.ObjectMeta.Name, "www.google.com")

			By("checking connectivity from linux to windows")
			checkLinuxConnectivity(f, linuxPod.ObjectMeta.Name, windowsPod.Status.PodIP, "80")

			By("checking connectivity from windows to linux")
			checkWindowsConnectivity(f, windowsPod.ObjectMeta.Name, linuxPod.Status.PodIP)

		})

	})
})

func checkContainerOutput(f *framework.Framework, podName string, os string, cmd []string) (string, string, error) {
	By(fmt.Sprintf("checking connectivity of %s-container in %s", os, podName))
	out, stderr, err := f.ExecCommandInContainerWithFullOutput(podName, os+"-container", cmd...)
	msg := fmt.Sprintf("cmd: %v, stdout: %q, stderr: %q", cmd, out, stderr)
	Expect(err).NotTo(HaveOccurred(), msg)
	return out, msg, err
}

func checkLinuxConnectivity(f *framework.Framework, podName string, address string, port string) {
	successes := 0
	for i := 0; i < iterations; i++ {
		nc := fmt.Sprintf("nc -vz %s %s", address, port)
		cmd := []string{"/bin/sh", "-c", nc}
		_, _, err := checkContainerOutput(f, podName, linuxOS, cmd)
		if err != nil {
			break
		}
		successes++
	}
	Expect(successes).To(Equal(iterations))
}

func checkWindowsConnectivity(f *framework.Framework, podName string, address string) {
	successes := 0
	for i := 0; i < iterations; i++ {
		ps := fmt.Sprintf("$r=invoke-webrequest %s -usebasicparsing; echo $r.StatusCode", address)
		cmd := []string{"powershell", "-command", ps}
		out, msg, err := checkContainerOutput(f, podName, windowsOS, cmd)
		if err != nil || out != "200" {
			framework.Logf(msg)
			break
		}
		successes++
	}
	Expect(successes).To(Equal(iterations))
}

func createTestPod(f *framework.Framework, image string, os string) *v1.Pod {
	containerName := fmt.Sprintf("%s-container", os)
	podName := "pod-" + string(uuid.NewUUID())
	pod := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  containerName,
					Image: image,
					Ports: []v1.ContainerPort{{ContainerPort: 80}},
				},
			},
			NodeSelector: map[string]string{
				"beta.kubernetes.io/os": os,
			},
		},
	}
	if os == linuxOS {
		pod.Spec.Tolerations = []v1.Toleration{
			{
				Key:      "key",
				Operator: v1.TolerationOpExists,
				Effect:   v1.TaintEffectNoSchedule,
			},
		}
	}
	return f.PodClient().CreateSync(pod)
}
