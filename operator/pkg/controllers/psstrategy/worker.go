// Copyright 2022 The EasyDL Authors. All rights reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package psstrategy

import (
	"context"
	"fmt"
	elasticv1alpha1 "github.com/intelligent-machine-learning/easydl/operator/api/v1alpha1"
	controllers "github.com/intelligent-machine-learning/easydl/operator/pkg/controllers"
	logger "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

const (
	workerServicePort int32 = 2222
)

// WorkerManager generates a master pod object.
type WorkerManager struct {
	controllers.PodManager
}

func init() {
	logger.Infof("init worker manager")
	controllers.ReplicaManagers[ReplicaTypeWorker] = newWorkerManager()
}

func newWorkerManager() *WorkerManager {
	return &WorkerManager{}
}

func insertCommonWorkerLabels(labels map[string]string, workerIndex int32) {
	labels[controllers.LabelReplicaTypeKey] = string(ReplicaTypeWorker)
	labels[controllers.LabelReplicaIndexKey] = fmt.Sprintf("%d", workerIndex)
}

func (m *WorkerManager) newWorker(job *elasticv1alpha1.ElasticJob, workerIndex int32) *corev1.Pod {
	spec, ok := job.Spec.ReplicaSpecs[ReplicaTypeWorker]
	if !ok {
		return nil
	}
	name := newWorkerName(job.Name, workerIndex)
	pod := m.NewPod(job, &spec.Template, name)
	pod.Labels[LabelRestartCount] = fmt.Sprintf("%d", spec.RestartCount)
	insertCommonWorkerLabels(pod.Labels, workerIndex)
	return pod
}

// ReconcilePods creates a Pod on a K8s cluster
func (m *WorkerManager) ReconcilePods(
	r *controllers.ElasticJobReconciler,
	job *elasticv1alpha1.ElasticJob,
	resourceSpec *elasticv1alpha1.ReplicaResourceSpec,
) error {
	for i := 0; i < resourceSpec.Replicas; i++ {
		workerIndex := int32(i)
		worker := m.newWorker(job, workerIndex)
		err := r.Create(context.Background(), worker)
		if err != nil {
			r.Recorder.Eventf(job, corev1.EventTypeWarning, string(corev1.PodFailed), "worker pod %s created failed: %v", worker.Name, err)
			return err
		}
		service := m.newWorkerService(job, workerIndex)
		err = r.Create(context.Background(), service)
		if err != nil {
			r.Recorder.Eventf(job, corev1.EventTypeWarning, string(corev1.PodFailed), "worker service %s created failed: %v", service.Name, err)
			return err
		}
	}
	return nil
}

// SyncJobState synchronize the job status by replicas
func (m *WorkerManager) SyncJobState(
	r *controllers.ElasticJobReconciler,
	job *elasticv1alpha1.ElasticJob,
) error {
	workers, err := m.GetReplicaTypePods(r, job, ReplicaTypeWorker)
	if errors.IsNotFound(err) {
		logger.Warningf("No any worker found: %v", err)
		return nil
	}
	workerStatus := m.GetReplicaStatus(workers)
	job.Status.ReplicaStatuses[ReplicaTypeWorker] = workerStatus
	return nil
}

func (m *WorkerManager) newWorkerService(job *elasticv1alpha1.ElasticJob, workerIndex int32) *corev1.Service {
	name := newWorkerName(job.Name, workerIndex)
	selector := make(map[string]string)
	insertCommonWorkerLabels(selector, workerIndex)
	service := m.NewService(job, name, workerServicePort, selector)
	return service
}

func newWorkerName(jobName string, workerIndex int32) string {
	return fmt.Sprintf("%s-%s-%d", jobName, string(ReplicaTypeWorker), workerIndex)
}
