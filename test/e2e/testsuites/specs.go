/*
Copyright 2019 The Kubernetes Authors.

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
	"fmt"

	"sigs.k8s.io/azuredisk-csi-driver/test/e2e/driver"

	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	clientset "k8s.io/client-go/kubernetes"
	restclientset "k8s.io/client-go/rest"
)

type PodDetails struct {
	Cmd     string
	Volumes []VolumeDetails
}

type VolumeDetails struct {
	VolumeType            string
	FSType                string
	Encrypted             bool
	MountOptions          []string
	ClaimSize             string
	ReclaimPolicy         *v1.PersistentVolumeReclaimPolicy
	VolumeBindingMode     *storagev1.VolumeBindingMode
	AllowedTopologyValues []string
	VolumeMode            VolumeMode
	VolumeMount           VolumeMountDetails
	VolumeDevice          VolumeDeviceDetails
	// Optional, used with pre-provisioned volumes
	VolumeID string
	// Optional, used with PVCs created from snapshots or pvc
	DataSource *DataSource
	// Optional, used with specified StorageClass
	StorageClass *storagev1.StorageClass
}

type VolumeMode int

const (
	FileSystem VolumeMode = iota
	Block
)

const (
	VolumeSnapshotKind = "VolumeSnapshot"
	VolumePVCKind      = "PersistentVolumeClaim"
	SnapshotAPIVersion = "snapshot.storage.k8s.io/v1alpha1"
	APIVersionv1alpha1 = "v1alpha1"
)

var (
	SnapshotAPIGroup = "snapshot.storage.k8s.io"
)

type VolumeMountDetails struct {
	NameGenerate      string
	MountPathGenerate string
	ReadOnly          bool
}

type VolumeDeviceDetails struct {
	NameGenerate string
	DevicePath   string
}

type DataSource struct {
	Kind string
	Name string
}

func (pod *PodDetails) SetupWithDynamicVolumes(client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver) (*TestPod, []func()) {
	tpod := NewTestPod(client, namespace, pod.Cmd)
	cleanupFuncs := make([]func(), 0)
	for n, v := range pod.Volumes {
		tpvc, funcs := v.SetupDynamicPersistentVolumeClaim(client, namespace, csiDriver)
		tpod.pvcs = append(tpod.pvcs, tpvc)
		cleanupFuncs = append(cleanupFuncs, funcs...)
		ginkgo.By("setting up the pod")
		if v.VolumeMode == Block {
			tpod.SetupRawBlockVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeDevice.NameGenerate, n+1), v.VolumeDevice.DevicePath)
		} else {
			tpod.SetupVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeMount.NameGenerate, n+1), fmt.Sprintf("%s%d", v.VolumeMount.MountPathGenerate, n+1), v.VolumeMount.ReadOnly)
		}
	}
	return tpod, cleanupFuncs
}

func (pod *PodDetails) SetupWithPreProvisionedVolumes(client clientset.Interface, namespace *v1.Namespace, csiDriver driver.PreProvisionedVolumeTestDriver) (*TestPod, []func()) {
	tpod := NewTestPod(client, namespace, pod.Cmd)
	cleanupFuncs := make([]func(), 0)
	for n, v := range pod.Volumes {
		tpvc, funcs := v.SetupPreProvisionedPersistentVolumeClaim(client, namespace, csiDriver)
		tpod.pvcs = append(tpod.pvcs, tpvc)
		cleanupFuncs = append(cleanupFuncs, funcs...)
		ginkgo.By("setting up the pod")
		if v.VolumeMode == Block {
			tpod.SetupRawBlockVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeDevice.NameGenerate, n+1), v.VolumeDevice.DevicePath)
		} else {
			tpod.SetupVolume(tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeMount.NameGenerate, n+1), fmt.Sprintf("%s%d", v.VolumeMount.MountPathGenerate, n+1), v.VolumeMount.ReadOnly)
		}
	}
	return tpod, cleanupFuncs
}

func (pod *PodDetails) SetupDeployment(client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver) (*TestDeployment, []func()) {
	cleanupFuncs := make([]func(), 0)
	volume := pod.Volumes[0]
	ginkgo.By("setting up the StorageClass")
	storageClass := csiDriver.GetDynamicProvisionStorageClass(driver.GetParameters(), volume.MountOptions, volume.ReclaimPolicy, volume.VolumeBindingMode, volume.AllowedTopologyValues, namespace.Name)
	tsc := NewTestStorageClass(client, namespace, storageClass)
	createdStorageClass := tsc.Create()
	cleanupFuncs = append(cleanupFuncs, tsc.Cleanup)
	ginkgo.By("setting up the PVC")
	tpvc := NewTestPersistentVolumeClaim(client, namespace, volume.ClaimSize, volume.VolumeMode, &createdStorageClass)
	tpvc.Create()
	if volume.VolumeBindingMode == nil || *volume.VolumeBindingMode == storagev1.VolumeBindingImmediate {
		tpvc.WaitForBound()
		tpvc.ValidateProvisionedPersistentVolume()
	}
	cleanupFuncs = append(cleanupFuncs, tpvc.Cleanup)
	ginkgo.By("setting up the Deployment")
	tDeployment := NewTestDeployment(client, namespace, pod.Cmd, tpvc.persistentVolumeClaim, fmt.Sprintf("%s%d", volume.VolumeMount.NameGenerate, 1), fmt.Sprintf("%s%d", volume.VolumeMount.MountPathGenerate, 1), volume.VolumeMount.ReadOnly)

	cleanupFuncs = append(cleanupFuncs, tDeployment.Cleanup)
	return tDeployment, cleanupFuncs
}

func (volume *VolumeDetails) SetupDynamicPersistentVolumeClaim(client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver) (*TestPersistentVolumeClaim, []func()) {
	cleanupFuncs := make([]func(), 0)
	storageClass := volume.StorageClass
	if storageClass == nil {
		tsc, tscCleanup := volume.CreateStorageClass(client, namespace, csiDriver)
		cleanupFuncs = append(cleanupFuncs, tscCleanup)
		storageClass = tsc.storageClass
	}
	ginkgo.By("setting up the PVC and PV")
	var tpvc *TestPersistentVolumeClaim
	if volume.DataSource != nil {
		dataSource := &v1.TypedLocalObjectReference{
			Name: volume.DataSource.Name,
			Kind: volume.DataSource.Kind,
		}
		if volume.DataSource.Kind == VolumeSnapshotKind {
			dataSource.APIGroup = &SnapshotAPIGroup
		}
		tpvc = NewTestPersistentVolumeClaimWithDataSource(client, namespace, volume.ClaimSize, volume.VolumeMode, storageClass, dataSource)
	} else {
		tpvc = NewTestPersistentVolumeClaim(client, namespace, volume.ClaimSize, volume.VolumeMode, storageClass)
	}
	tpvc.Create()
	cleanupFuncs = append(cleanupFuncs, tpvc.Cleanup)
	// PV will not be ready until PVC is used in a pod when volumeBindingMode: WaitForFirstConsumer
	if volume.VolumeBindingMode == nil || *volume.VolumeBindingMode == storagev1.VolumeBindingImmediate {
		tpvc.WaitForBound()
		tpvc.ValidateProvisionedPersistentVolume()
	}

	return tpvc, cleanupFuncs
}

func (volume *VolumeDetails) SetupPreProvisionedPersistentVolumeClaim(client clientset.Interface, namespace *v1.Namespace, csiDriver driver.PreProvisionedVolumeTestDriver) (*TestPersistentVolumeClaim, []func()) {
	cleanupFuncs := make([]func(), 0)
	ginkgo.By("setting up the PV")
	pv := csiDriver.GetPersistentVolume(volume.VolumeID, volume.FSType, volume.ClaimSize, volume.ReclaimPolicy, namespace.Name)
	tpv := NewTestPreProvisionedPersistentVolume(client, pv)
	tpv.Create()
	ginkgo.By("setting up the PVC")
	tpvc := NewTestPersistentVolumeClaim(client, namespace, volume.ClaimSize, volume.VolumeMode, nil)
	tpvc.Create()
	cleanupFuncs = append(cleanupFuncs, tpvc.DeleteBoundPersistentVolume)
	cleanupFuncs = append(cleanupFuncs, tpvc.Cleanup)
	tpvc.WaitForBound()
	tpvc.ValidateProvisionedPersistentVolume()

	return tpvc, cleanupFuncs
}

func (volume *VolumeDetails) CreateStorageClass(client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver) (*TestStorageClass, func()) {
	ginkgo.By("setting up the StorageClass")
	storageClass := csiDriver.GetDynamicProvisionStorageClass(driver.GetParameters(), volume.MountOptions, volume.ReclaimPolicy, volume.VolumeBindingMode, volume.AllowedTopologyValues, namespace.Name)
	allowVolumeExpansion := true
	storageClass.AllowVolumeExpansion = &allowVolumeExpansion
	tsc := NewTestStorageClass(client, namespace, storageClass)
	tsc.Create()
	return tsc, tsc.Cleanup
}

func CreateVolumeSnapshotClass(client restclientset.Interface, namespace *v1.Namespace, csiDriver driver.VolumeSnapshotTestDriver) (*TestVolumeSnapshotClass, func()) {
	ginkgo.By("setting up the VolumeSnapshotClass")
	volumeSnapshotClass := csiDriver.GetVolumeSnapshotClass(namespace.Name)
	tvsc := NewTestVolumeSnapshotClass(client, namespace, volumeSnapshotClass)
	tvsc.Create()

	return tvsc, tvsc.Cleanup
}

func (tpvc *TestPersistentVolumeClaim) UpdateDynamicVolumes(client clientset.Interface, namespace *v1.Namespace, csiDriver driver.DynamicPVTestDriver) {
	ginkgo.By("updating the PVC")
	tpvc.Update()
}
