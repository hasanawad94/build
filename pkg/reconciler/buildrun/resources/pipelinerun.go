// Copyright The Shipwright Contributors
//
// SPDX-License-Identifier: Apache-2.0

package resources

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	buildv1beta1 "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/shipwright-io/build/pkg/config"
	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// GeneratePipelineRun creates a Tekton PipelineRun object from a Build and BuildRun.
// It generates a TaskRun, and then embeds the TaskSpec into a PipelineRun.
func GeneratePipelineRun(cfg *config.Config, build *buildv1beta1.Build, buildRun *buildv1beta1.BuildRun, serviceAccountName string, strategy buildv1beta1.BuilderStrategy) (*pipelineapi.PipelineRun, error) {
	// Generate a TaskRun object using the existing logic. This gives us a fully-formed
	// object with all steps, parameters, workspaces, and pod settings correctly configured.
	taskRun, err := GenerateTaskRun(cfg, build, buildRun, serviceAccountName, strategy)
	if err != nil {
		return nil, err
	}

	// Start building the PipelineRun, using the generated TaskRun as a template.
	pipelineRun := &pipelineapi.PipelineRun{
		ObjectMeta: taskRun.ObjectMeta,
		Spec: pipelineapi.PipelineRunSpec{
			PipelineSpec: &pipelineapi.PipelineSpec{
				Tasks: []pipelineapi.PipelineTask{},
				Workspaces: []pipelineapi.PipelineWorkspaceDeclaration{
					{Name: workspaceSource},
				},
				Params: []pipelineapi.ParamSpec{},
			},
			TaskRunTemplate: pipelineapi.PipelineTaskRunTemplate{
				ServiceAccountName: taskRun.Spec.ServiceAccountName,
				PodTemplate:        taskRun.Spec.PodTemplate,
			},
			// Bind the declared workspace to a new PVC for this PipelineRun. This is the key
			// difference from a TaskRun, which uses an emptyDir.
			Workspaces: []pipelineapi.WorkspaceBinding{
				{
					Name: workspaceSource,
					VolumeClaimTemplate: &corev1.PersistentVolumeClaim{
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: resource.MustParse("1Gi"),
								},
							},
						},
					},
				},
			},
			// Use the same final parameters as the TaskRun, with one modification below.
			Params: taskRun.Spec.Params,
			Timeouts: &pipelineapi.TimeoutFields{
				Pipeline: taskRun.Spec.Timeout,
			},
		},
	}

	// Create a single Task from the generated TaskRun's spec
	taskSpec := taskRun.Spec.TaskSpec.DeepCopy()

	// When using a PVC workspace, the emptyDir volume for the output is no longer needed.
	taskSpec.Volumes = filterOutVolume(taskSpec.Volumes, "shp-output-directory")
	for i, step := range taskSpec.Steps {
		taskSpec.Steps[i].VolumeMounts = filterOutVolumeMount(step.VolumeMounts, "shp-output-directory")
	}

	// Embed the TaskSpec into a single PipelineTask.
	embedTaskSpecInPipeline(pipelineRun, taskSpec)

	// Remap the output directory parameter to point to a subdirectory in the shared workspace.
	remapOutputParameter(pipelineRun)

	// Propagate all parameters from the TaskSpec to the PipelineSpec and the PipelineTask.
	propagateParameters(pipelineRun, taskRun.Spec.TaskSpec)

	return pipelineRun, nil
}

// embedTaskSpecInPipeline wraps a given TaskSpec into a PipelineTask and embeds it
// into the PipelineRun's PipelineSpec.
func embedTaskSpecInPipeline(pipelineRun *pipelineapi.PipelineRun, taskSpec *pipelineapi.TaskSpec) {
	pipelineRun.Spec.PipelineSpec.Tasks = []pipelineapi.PipelineTask{
		{
			Name: "build-task",
			TaskSpec: &pipelineapi.EmbeddedTask{
				TaskSpec: *taskSpec,
			},
			Workspaces: []pipelineapi.WorkspacePipelineTaskBinding{
				{Name: workspaceSource, Workspace: workspaceSource},
			},
		},
	}
}

// remapOutputParameter finds the 'shp-output-directory' parameter in the PipelineRun's
// spec and changes its value to a path inside the shared workspace.
func remapOutputParameter(pipelineRun *pipelineapi.PipelineRun) {
	for i, param := range pipelineRun.Spec.Params {
		if param.Name == "shp-output-directory" {
			pipelineRun.Spec.Params[i].Value = *pipelineapi.NewStructuredValues(path.Join("/workspace/source", "shp-output-directory"))
			return
		}
	}
}

// propagateParameters takes all parameter definitions from a TaskSpec, declares them in the
// PipelineSpec, and then configures the PipelineTask to consume them.
func propagateParameters(pipelineRun *pipelineapi.PipelineRun, taskSpec *pipelineapi.TaskSpec) {
	if taskSpec == nil {
		return
	}

	for _, paramSpec := range taskSpec.Params {
		pipelineRun.Spec.PipelineSpec.Params = append(pipelineRun.Spec.PipelineSpec.Params, paramSpec)
		for i := range pipelineRun.Spec.PipelineSpec.Tasks {
			var paramValue pipelineapi.ParamValue
			if paramSpec.Type == pipelineapi.ParamTypeArray {
				paramValue = pipelineapi.ParamValue{
					Type:     pipelineapi.ParamTypeArray,
					ArrayVal: []string{"$(params." + paramSpec.Name + "[*])"},
				}
			} else {
				paramValue = pipelineapi.ParamValue{
					Type:      pipelineapi.ParamTypeString,
					StringVal: "$(params." + paramSpec.Name + ")",
				}
			}
			pipelineRun.Spec.PipelineSpec.Tasks[i].Params = append(
				pipelineRun.Spec.PipelineSpec.Tasks[i].Params,
				pipelineapi.Param{
					Name:  paramSpec.Name,
					Value: paramValue,
				},
			)
		}
	}
}

func filterOutVolume(volumes []corev1.Volume, name string) []corev1.Volume {
	var result []corev1.Volume
	for _, vol := range volumes {
		if vol.Name != name {
			result = append(result, vol)
		}
	}
	return result
}

func filterOutVolumeMount(mounts []corev1.VolumeMount, name string) []corev1.VolumeMount {
	var result []corev1.VolumeMount
	for _, mount := range mounts {
		if mount.Name != name {
			result = append(result, mount)
		}
	}
	return result
}
