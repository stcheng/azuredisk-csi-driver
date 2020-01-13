/*
Copyright 2020 The Kubernetes Authors.

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

package testsuites

import (
	"sigs.k8s.io/azuredisk-csi-driver/test/e2e/driver"

	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	clientset "k8s.io/client-go/kubernetes"
)

// DynamicallyProvisionedVolumeExpansionTest !!!
type DynamicallyProvisionedVolumeExpansionTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Pod       PodDetails
}

// Run !!!
func (t *DynamicallyProvisionedVolumeExpansionTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	// create the storageClass
	tsc, _ := t.Pod.Volumes[0].CreateStorageClass(client, namespace, t.CSIDriver)
	// defer tscCleanup()

	// create the pod
	t.Pod.Volumes[0].StorageClass = tsc.storageClass
	tpod, _ := t.Pod.SetupWithDynamicVolumes(client, namespace, t.CSIDriver)
	// for i := range cleanups {
	// 	defer cleanups[i]()
	// }

	// update the size
	t.Pod.Volumes[0].ClaimSize = "15Gi"
	tpod.pvcs[0].requestedPersistentVolumeClaim.ObjectMeta.Name = tpod.pod.Spec.Volumes[0].PersistentVolumeClaim.ClaimName
	tpod.pvcs[0].requestedPersistentVolumeClaim.Spec = v1.PersistentVolumeClaimSpec{
		AccessModes: []v1.PersistentVolumeAccessMode{
			v1.ReadWriteOnce,
		},
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): resource.MustParse("15Gi"),
			},
		},
	}
	// tpod.pvcs[0] = &v1.PersistentVolumeClaim{
	// 	ObjectMeta: metav1.ObjectMeta{

	// 	},
	// }
	// }

	// call  updagte API
	tpod.pvcs[0].UpdateDynamicVolumes(client, namespace, t.CSIDriver)

	ginkgo.By("deploying the pod")
	tpod.Create()
	// defer tpod.Cleanup()
	ginkgo.By("checking that the pod's command exits with no error")
	tpod.WaitForSuccess()
}
