// Copyright The Shipwright Contributors
//
// SPDX-License-Identifier: Apache-2.0

package buildrun

import (
	"context"
	"encoding/json"
	"fmt"

	buildv1beta1 "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/shipwright-io/build/pkg/config"
	"github.com/shipwright-io/build/pkg/reconciler/buildrun/resources"
	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"knative.dev/pkg/apis"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TektonPipelineRunWrapper wraps pipelineapi.PipelineRun to implement the ImageBuildRunner interface.
type TektonPipelineRunWrapper struct {
	PipelineRun *pipelineapi.PipelineRun
}

// GetName returns the name of the PipelineRun.
func (t *TektonPipelineRunWrapper) GetName() string {
	if t.PipelineRun == nil {
		return ""
	}
	return t.PipelineRun.Name
}

// GetLabels returns the labels of the PipelineRun.
func (t *TektonPipelineRunWrapper) GetLabels() map[string]string {
	if t.PipelineRun == nil {
		return nil
	}
	return t.PipelineRun.Labels
}

// GetCreationTimestamp returns the creation timestamp of the PipelineRun.
func (t *TektonPipelineRunWrapper) GetCreationTimestamp() metav1.Time {
	if t.PipelineRun == nil {
		return metav1.Time{}
	}
	return t.PipelineRun.CreationTimestamp
}

// GetResults returns the PipelineRun results converted to TaskRun results.
func (t *TektonPipelineRunWrapper) GetResults() []pipelineapi.TaskRunResult {
	if t.PipelineRun == nil {
		return nil
	}
	var taskRunResults []pipelineapi.TaskRunResult
	for _, prResult := range t.PipelineRun.Status.Results {
		taskRunResults = append(taskRunResults, pipelineapi.TaskRunResult{
			Name:  prResult.Name,
			Value: prResult.Value,
		})
	}
	return taskRunResults
}

// GetCondition returns the condition with the specified type.
func (t *TektonPipelineRunWrapper) GetCondition(conditionType apis.ConditionType) *apis.Condition {
	if t.PipelineRun == nil {
		return nil
	}
	return t.PipelineRun.Status.GetCondition(conditionType)
}

// GetStartTime returns the start time of the PipelineRun.
func (t *TektonPipelineRunWrapper) GetStartTime() *metav1.Time {
	if t.PipelineRun == nil {
		return nil
	}
	return t.PipelineRun.Status.StartTime
}

// GetCompletionTime returns the completion time of the PipelineRun.
func (t *TektonPipelineRunWrapper) GetCompletionTime() *metav1.Time {
	if t.PipelineRun == nil {
		return nil
	}
	return t.PipelineRun.Status.CompletionTime
}

// GetPodName returns the pod name of the PipelineRun's first TaskRun.
func (t *TektonPipelineRunWrapper) GetPodName(ctx context.Context, client client.Client) (string, error) {
	if t.PipelineRun == nil || len(t.PipelineRun.Status.ChildReferences) == 0 {
		return "", nil
	}

	// In tekton v1, TaskRuns map is deprecated in favor of ChildReferences.
	// To get pod name, one would have to list TaskRuns and find the one from ChildReferences.
	for _, childRef := range t.PipelineRun.Status.ChildReferences {
		if childRef.Kind == "TaskRun" {
			taskRun := &pipelineapi.TaskRun{}
			err := client.Get(ctx, types.NamespacedName{Name: childRef.Name, Namespace: t.PipelineRun.Namespace}, taskRun)
			if err != nil {
				return "", err
			}
			if taskRun.Status.PodName != "" {
				return taskRun.Status.PodName, nil
			}
		}
	}

	return "", nil
}

// IsCancelled returns true if the PipelineRun is cancelled.
func (t *TektonPipelineRunWrapper) IsCancelled() bool {
	if t.PipelineRun == nil {
		return false
	}
	return t.PipelineRun.IsCancelled()
}

// Cancel cancels the PipelineRun by setting its status to cancelled.
func (t *TektonPipelineRunWrapper) Cancel(ctx context.Context, c client.Client) error {
	if t.PipelineRun == nil {
		return fmt.Errorf("underlying PipelineRun does not exist")
	}

	payload := []patchStringValue{{
		Op:    "replace",
		Path:  "/spec/status",
		Value: pipelineapi.PipelineRunSpecStatusCancelled,
	}}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	patch := client.RawPatch(types.JSONPatchType, data)

	trueParam := true
	patchOpt := client.PatchOptions{
		Raw: &metav1.PatchOptions{
			Force: &trueParam,
		},
	}
	return c.Patch(ctx, t.PipelineRun, patch, &patchOpt)
}

// GetObject returns the underlying client.Object for owner reference operations.
func (t *TektonPipelineRunWrapper) GetObject() client.Object {
	return t.PipelineRun
}

// GetKind returns the Tekton a PipelineRun kind
func (t *TektonPipelineRunWrapper) GetKind() string {
	return "PipelineRun"
}

// TektonPipelineRunImageBuildRunnerFactory implements ImageBuildRunnerFactory for Tekton PipelineRuns.
type TektonPipelineRunImageBuildRunnerFactory struct{}

// NewImageBuildRunner creates a new empty ImageBuildRunner for a PipelineRun.
func (f *TektonPipelineRunImageBuildRunnerFactory) NewImageBuildRunner() ImageBuildRunner {
	return &TektonPipelineRunWrapper{
		PipelineRun: &pipelineapi.PipelineRun{},
	}
}

// CreateImageBuildRunner creates an ImageBuildRunner instance from build configuration.
func (f *TektonPipelineRunImageBuildRunnerFactory) CreateImageBuildRunner(cfg *config.Config, serviceAccount *corev1.ServiceAccount, strategy buildv1beta1.BuilderStrategy, build *buildv1beta1.Build, buildRun *buildv1beta1.BuildRun, scheme *runtime.Scheme, setOwnerRef setOwnerReferenceFunc) (ImageBuildRunner, error) {
	generatedPipelineRun, err := resources.GeneratePipelineRun(cfg, build, buildRun, serviceAccount.Name, strategy)
	if err != nil {
		return nil, err
	}

	if err := setOwnerRef(buildRun, generatedPipelineRun, scheme); err != nil {
		return nil, err
	}

	return &TektonPipelineRunWrapper{PipelineRun: generatedPipelineRun}, nil
}

// GetImageBuildRunner retrieves an ImageBuildRunner from the API server.
func (f *TektonPipelineRunImageBuildRunnerFactory) GetImageBuildRunner(ctx context.Context, client client.Client, namespacedName types.NamespacedName) (ImageBuildRunner, error) {
	pipelineRun := &pipelineapi.PipelineRun{}
	err := client.Get(ctx, namespacedName, pipelineRun)
	if err != nil {
		return nil, err
	}
	return &TektonPipelineRunWrapper{PipelineRun: pipelineRun}, nil
}

// CreateImageBuildRunnerInCluster creates the ImageBuildRunner in the API server.
func (f *TektonPipelineRunImageBuildRunnerFactory) CreateImageBuildRunnerInCluster(ctx context.Context, client client.Client, runner ImageBuildRunner) error {
	wrapper, ok := runner.(*TektonPipelineRunWrapper)
	if !ok {
		return fmt.Errorf("unsupported ImageBuildRunner type")
	}
	return client.Create(ctx, wrapper.PipelineRun)
}
