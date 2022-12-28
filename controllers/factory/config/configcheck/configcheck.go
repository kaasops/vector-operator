/*
Copyright 2022.

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

package configcheck

import (
	"context"
	"math/rand"

	"github.com/kaasops/vector-operator/controllers/factory/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ConfigCheck struct {
	Config []byte

	Client    client.Client
	ClientSet *kubernetes.Clientset

	Name        string
	Namespace   string
	Initiator   string
	Image       string
	Envs        []corev1.EnvVar
	Hash        string
	Tolerations []corev1.Toleration
}

func New(
	config []byte,
	c client.Client,
	cs *kubernetes.Clientset,
	name, namespace, image string,
	envs []corev1.EnvVar,
	tolerations []corev1.Toleration,
) *ConfigCheck {
	return &ConfigCheck{
		Config:      config,
		Client:      c,
		ClientSet:   cs,
		Name:        name,
		Namespace:   namespace,
		Image:       image,
		Envs:        envs,
		Tolerations: tolerations,
	}
}

func (cc *ConfigCheck) Run(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("Vector ConfigCheck", cc.Initiator)
	log.Info("================= Started ConfigCheck =======================")

	if err := cc.ensureVectorConfigCheckRBAC(ctx); err != nil {
		return err
	}

	cc.Hash = randStringRunes()

	if err := cc.ensureVectorConfigCheckConfig(ctx); err != nil {
		return err
	}

	if err := cc.checkVectorConfigCheckPod(ctx); err != nil {
		return err
	}

	return nil
}

func (cc *ConfigCheck) ensureVectorConfigCheckRBAC(ctx context.Context) error {
	return cc.ensureVectorConfigCheckServiceAccount(ctx)
}

func (cc *ConfigCheck) ensureVectorConfigCheckServiceAccount(ctx context.Context) error {
	vectorAgentServiceAccount := cc.createVectorConfigCheckServiceAccount()

	return k8s.CreateOrUpdateResource(ctx, vectorAgentServiceAccount, cc.Client)
}
func (cc *ConfigCheck) ensureVectorConfigCheckConfig(ctx context.Context) error {
	vectorConfigCheckSecret, err := cc.createVectorConfigCheckConfig()
	if err != nil {
		return err
	}

	return k8s.CreateOrUpdateResource(ctx, vectorConfigCheckSecret, cc.Client)
}

func (cc *ConfigCheck) checkVectorConfigCheckPod(ctx context.Context) error {
	vectorConfigCheckPod := cc.createVectorConfigCheckPod()

	err := k8s.CreatePod(ctx, vectorConfigCheckPod, cc.Client)
	if err != nil {
		return err
	}

	err = cc.getCheckResult(ctx, vectorConfigCheckPod)
	if err != nil {
		return err
	}

	err = cc.cleanup(ctx, vectorConfigCheckPod)
	if err != nil {
		return err
	}
	return nil
}

func labelsForVectorConfigCheck() map[string]string {
	return map[string]string{
		k8s.ManagedByLabelKey:  "vector-operator",
		k8s.NameLabelKey:       "vector-configcheck",
		k8s.ComponentLabelKey:  "ConfigCheck",
		k8s.VectorExcludeLabel: "true",
	}
}

func (cc *ConfigCheck) getNameVectorConfigCheck() string {
	n := "configcheck" + "-" + cc.Name + "-" + cc.Hash

	return n
}

func randStringRunes() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")

	b := make([]rune, 5)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func (cc *ConfigCheck) getCheckResult(ctx context.Context, pod *corev1.Pod) (err error) {
	log := log.FromContext(ctx).WithValues("Vector ConfigCheck", pod.Name)
	log.Info("Trying to get configcheck result")

	watcher, err := cc.ClientSet.CoreV1().Pods(cc.Namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(metav1.ObjectNameField, pod.Name).String(),
		// LabelSelector: labelsForVectorConfigCheck(),
	})

	if err != nil {
		log.Error(err, "cannot create Pod event watcher")
		return err
	}

	defer watcher.Stop()

	for {
		select {
		case e := <-watcher.ResultChan():
			if e.Object == nil {
				return nil
			}
			pod, ok := e.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			switch e.Type {
			case watch.Modified:
				if pod.DeletionTimestamp != nil {
					continue
				}
				switch pod.Status.Phase {
				case corev1.PodSucceeded:
					log.Info("Config Check completed successfully")
					return nil
				case corev1.PodFailed:
					log.Info("Config Check Failed")
					reason, err := k8s.GetPodLogs(ctx, pod, cc.ClientSet)
					if err != nil {
						return err
					}
					return newValidationError(reason)
				}
			}
		case <-ctx.Done():
			watcher.Stop()
			return nil
		}
	}
}

func (cc *ConfigCheck) cleanup(ctx context.Context, pod *corev1.Pod) error {
	pod, err := k8s.FetchPod(ctx, pod, cc.Client)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	for _, v := range pod.Spec.Volumes {
		if v.Name == "config" {
			nn := types.NamespacedName{
				Name:      v.Secret.SecretName,
				Namespace: pod.Namespace,
			}
			secret, err := k8s.GetSecret(ctx, nn, cc.Client)
			if err != nil {
				return err
			}
			if err := k8s.DeleteSecret(ctx, secret, cc.Client); err != nil {
				return err
			}
		}
	}
	if err := k8s.DeletePod(ctx, pod, cc.Client); err != nil {
		return err
	}
	return nil
}

func (cc *ConfigCheck) gcRListOptions() (client.ListOptions, error) {
	configCheckLabels := labelsForVectorConfigCheck()
	var requirements []labels.Requirement
	for k, v := range configCheckLabels {
		r, err := labels.NewRequirement(k, "==", []string{v})
		if err != nil {
			return client.ListOptions{}, err
		}
		requirements = append(requirements, *r)
	}
	labelsSelector := labels.NewSelector().Add(requirements...)

	return client.ListOptions{
		LabelSelector: labelsSelector,
		Namespace:     cc.Namespace,
	}, nil
}
