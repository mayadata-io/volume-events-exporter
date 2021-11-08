package tests

import (
	"io/fs"
	"io/ioutil"

	"github.com/ghodss/yaml"
	"github.com/mayadata-io/volume-events-exporter/pkg/encrypt/rsa"
	"github.com/mayadata-io/volume-events-exporter/pkg/env"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

// addOrUpdateEventControllerSidecar will add volume-event-controller side car only
// if container doesn't exist else updates the CALLBACK_URL and CALLBACK_TOKEN
func addEventControllerSideCar(deploymentNamespace, deploymentName string) error {
	deployObj, err := Client.getDeployment(deploymentNamespace, deploymentName)
	if err != nil {
		return err
	}
	var isVolumeEventsCollectorExist bool
	volumeEventsCollector := corev1.Container{
		Name:  "volume-events-collector",
		Image: "mayadataio/volume-events-exporter:ci",
		Args: []string{
			"--leader-election=false",
			"--generate-k8s-events=true",
		},
		Env: []corev1.EnvVar{
			{
				Name:  "OPENEBS_IO_NFS_SERVER_NS",
				Value: OpenEBSNamespace,
			},
			{
				Name:  "CALLBACK_URL",
				Value: serverIface.GetEventsReceiverEndpoint(),
			},
			{
				Name:  "CALLBACK_TOKEN",
				Value: serverIface.GetToken(),
			},
		},
	}

	for idx, container := range deployObj.Spec.Template.Spec.Containers {
		if container.Name == volumeEventsCollector.Name {
			deployObj.Spec.Template.Spec.Containers[idx] = volumeEventsCollector
			isVolumeEventsCollectorExist = true
			break
		}
	}
	if !isVolumeEventsCollectorExist {
		deployObj.Spec.Template.Spec.Containers = append(deployObj.Spec.Template.Spec.Containers, volumeEventsCollector)
	}
	updatedDeployObj, err := Client.updateDeployment(deployObj)
	if err != nil {
		return err
	}
	return Client.waitForDeploymentRollout(updatedDeployObj.Namespace, updatedDeployObj.Name)
}

func removeEventsCollectorSidecar(deploymentNamespace, deploymentName string) error {
	var isVolumeEventsCollectorExist bool
	var index int

	deployObj, err := Client.getDeployment(deploymentNamespace, deploymentName)
	if err != nil {
		return err
	}

	for idx, container := range deployObj.Spec.Template.Spec.Containers {
		if container.Name == "volume-events-collector" {
			index = idx
			isVolumeEventsCollectorExist = true
		}
	}
	// Remove volume events collector sidecar
	if !isVolumeEventsCollectorExist {
		return nil
	}

	deployObj.Spec.Template.Spec.Containers = append(deployObj.Spec.Template.Spec.Containers[:index], deployObj.Spec.Template.Spec.Containers[index+1:]...)
	updatedDeployObj, err := Client.updateDeployment(deployObj)
	if err != nil {
		return err
	}
	return Client.waitForDeploymentRollout(updatedDeployObj.Namespace, updatedDeployObj.Name)
}

// updateNFSHookConfig will update the NFS hook configuration as
// per test details
func updateNFSHookConfig(namespace, name string) error {
	hookConfigMap, err := Client.getConfigMap(namespace, name)
	if err != nil {
		return errors.Wrapf(err, "failed to get configmap %s/%s", namespace, name)
	}
	var hook Hook
	hookData, isConfigExist := hookConfigMap.Data[nfsHookConfigDataKey]
	if !isConfigExist {
		return errors.Errorf("hook configmap=%s/%s doesn't have data field=%s", namespace, name, nfsHookConfigDataKey)
	}

	err = yaml.Unmarshal([]byte(hookData), &hook)
	if err != nil {
		return err
	}
	addHookConfig, isAddExist := hook.Config[ActionAddOnCreateVolumeEvent]
	if !isAddExist {
		return errors.Errorf("%s configuration doesn't exist in hook %s/%s", ActionAddOnCreateVolumeEvent, namespace, name)
	}
	addHookConfig.BackendPVCConfig.Finalizers = append(addHookConfig.BackendPVCConfig.Finalizers, integrationTestFinalizer)
	hook.Config[ActionAddOnCreateVolumeEvent] = addHookConfig

	updatedHookConfigInBytes, err := yaml.Marshal(hook)
	if err != nil {
		return err
	}

	hookConfigMap.Data[nfsHookConfigDataKey] = string(updatedHookConfigInBytes)
	_, err = Client.updateConfigMap(hookConfigMap)
	return err
}

// addSigningKeyPath will does following changes to deploymant:
// - Add hostpath volume to deployment
// - Add container path(volume mount) to volume-events-collector container
// - Add Env of signing key path to private key
func addSigningKeyPath(deploymentNamespace, deploymentName string, volumeMount corev1.VolumeMount, containerSigningKeyPath string) error {
	deployObj, err := Client.getDeployment(deploymentNamespace, deploymentName)
	if err != nil {
		return err
	}

	var isEnvExist, isVolumeExist, isVolumeMountExist bool
	var volumeEventContainerIndex int
	for cIdx, container := range deployObj.Spec.Template.Spec.Containers {
		if container.Name == "volume-events-collector" {
			for eIdx, envVal := range container.Env {
				if envVal.Name == env.SigningKeyPathKey {
					deployObj.Spec.Template.Spec.Containers[cIdx].Env[eIdx].Value = containerSigningKeyPath
					isEnvExist = true
				}
			}
			volumeEventContainerIndex = cIdx
			for vIdx, cvMount := range container.VolumeMounts {
				if cvMount.Name == volumeMount.Name {
					deployObj.Spec.Template.Spec.Containers[cIdx].VolumeMounts[vIdx] = volumeMount
					isVolumeMountExist = true
				}
			}
		}
	}

	if !isEnvExist {
		deployObj.Spec.Template.Spec.Containers[volumeEventContainerIndex].Env = append(deployObj.Spec.Template.Spec.Containers[volumeEventContainerIndex].Env, corev1.EnvVar{Name: env.SigningKeyPathKey, Value: containerSigningKeyPath})
	}

	if !isVolumeMountExist {
		deployObj.Spec.Template.Spec.Containers[volumeEventContainerIndex].VolumeMounts = append(deployObj.Spec.Template.Spec.Containers[volumeEventContainerIndex].VolumeMounts, volumeMount)
	}

	hostPathDirectory := corev1.HostPathDirectory
	for vIdx, cVolume := range deployObj.Spec.Template.Spec.Volumes {
		if cVolume.Name == volumeMount.Name {
			deployObj.Spec.Template.Spec.Volumes[vIdx].HostPath = &corev1.HostPathVolumeSource{
				Path: signingKeyDir,
				Type: &hostPathDirectory,
			}
			isVolumeExist = true
		}
	}

	if !isVolumeExist {
		deployObj.Spec.Template.Spec.Volumes = append(deployObj.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: volumeMount.Name,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: signingKeyDir,
					Type: &hostPathDirectory,
				},
			},
		})
	}

	updatedDeployObj, err := Client.updateDeployment(deployObj)
	if err != nil {
		return err
	}
	return Client.waitForDeploymentRollout(updatedDeployObj.Namespace, updatedDeployObj.Name)
}

// removeSigningKeyPath will remove SIGNING_KEY_PATH env from deployment
func removeSigningKeyPath(deploymentNamespace, deploymentName string) error {
	deployObj, err := Client.getDeployment(deploymentNamespace, deploymentName)
	if err != nil {
		return err
	}
	for cIdx, container := range deployObj.Spec.Template.Spec.Containers {
		if container.Name == "volume-events-collector" {
			var updatedEnvs []corev1.EnvVar
			for _, envVar := range container.Env {
				if envVar.Name == env.SigningKeyPathKey {
					continue
				}
				updatedEnvs = append(updatedEnvs, envVar)
			}
			deployObj.Spec.Template.Spec.Containers[cIdx].Env = updatedEnvs
		}
	}
	updatedDeployObj, err := Client.updateDeployment(deployObj)
	if err != nil {
		return err
	}
	return Client.waitForDeploymentRollout(updatedDeployObj.Namespace, updatedDeployObj.Name)
}

func generateRSAKeys() error {
	// following private-public key pair is used to encrypt the data
	privateKey, publicKey, err := rsa.GenerateKeyPair(2048)
	if err != nil {
		return err
	}
	err = writeToFile(privateKey, signingPrivateKeyPath, 0600)
	if err != nil {
		return errors.Wrapf(err, "failed to dump private key into file")
	}
	err = writeToFile(publicKey, signingPublicKeyPath, 0600)
	if err != nil {
		return errors.Wrapf(err, "failed to dump public key into file")
	}

	// We are generating another public private key pair, which will be
	// used for integration testing
	fakePrivateKey, _, err := rsa.GenerateKeyPair(4096)
	if err != nil {
		return nil
	}
	return writeToFile(fakePrivateKey, fakeSigningPrivateKeyPath, 0600)
}

func writeToFile(data []byte, filePath string, perm fs.FileMode) error {
	return ioutil.WriteFile(filePath, data, perm)
}
