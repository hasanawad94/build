// Copyright The Shipwright Contributors
//
// SPDX-License-Identifier: Apache-2.0

package buildrun

import (
	"context"

	pipelineapi "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"knative.dev/pkg/apis"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	buildv1beta1 "github.com/shipwright-io/build/pkg/apis/build/v1beta1"
	"github.com/shipwright-io/build/pkg/config"
)

// ImageBuildRunner defines an interface for building a container image.
type ImageBuildRunner interface {
	// GetName returns the name of the build runner.
	GetName() string
	// GetLabels returns the labels of the build runner.
	GetLabels() map[string]string
	// GetCreationTimestamp returns the creation timestamp of the build runner.
	GetCreationTimestamp() metav1.Time
	// GetResults returns the results of the build runner.
	GetResults() []pipelineapi.TaskRunResult
	// GetCondition returns the condition of the build runner.
	GetCondition(conditionType apis.ConditionType) *apis.Condition
	// GetStartTime returns the start time of the build runner.
	GetStartTime() *metav1.Time
	// GetCompletionTime returns the completion time of the build runner.
	GetCompletionTime() *metav1.Time
	// GetPodName returns the pod name of the build runner.
	GetPodName(ctx context.Context, client client.Client) (string, error)
	// IsCancelled returns true if the build runner is cancelled.
	IsCancelled() bool
	// Cancel cancels the execution of the build runner.
	Cancel(ctx context.Context, client client.Client) error
	// GetObject returns the underlying client.Object for owner reference operations.
	GetObject() client.Object
	// GetKind returns the Kind of the build runner.
	GetKind() string
}

// ImageBuildRunnerFactory defines methods for creating and manipulating ImageBuildRunners.
type ImageBuildRunnerFactory interface {
	// NewImageBuildRunner creates a new empty ImageBuildRunner.
	NewImageBuildRunner() ImageBuildRunner

	// CreateImageBuildRunner creates an ImageBuildRunner instance from build configuration. It does not create the ImageBuildRunner in the API server.
	CreateImageBuildRunner(cfg *config.Config, serviceAccount *corev1.ServiceAccount, strategy buildv1beta1.BuilderStrategy, build *buildv1beta1.Build, buildRun *buildv1beta1.BuildRun, scheme *runtime.Scheme, setOwnerRef setOwnerReferenceFunc) (ImageBuildRunner, error)

	// GetImageBuildRunner retrieves an ImageBuildRunner from the API server.
	GetImageBuildRunner(ctx context.Context, client client.Client, namespacedName types.NamespacedName) (ImageBuildRunner, error)

	// CreateImageBuildRunnerInCluster creates the ImageBuildRunner in the API server.
	CreateImageBuildRunnerInCluster(ctx context.Context, client client.Client, taskRunner ImageBuildRunner) error
}
