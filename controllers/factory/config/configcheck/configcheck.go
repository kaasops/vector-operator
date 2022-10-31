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
	"time"

	"github.com/kaasops/vector-operator/controllers/factory/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ConfigCheck struct {
	Ctx context.Context

	Config []byte

	Client    client.Client
	ClientSet *kubernetes.Clientset

	Name      string
	Namespace string
	Image     string
	Hash      string
}

func New(ctx context.Context, config []byte, c client.Client, cs *kubernetes.Clientset, name, namespace, image string) *ConfigCheck {
	return &ConfigCheck{
		Ctx:       ctx,
		Config:    config,
		Client:    c,
		ClientSet: cs,
		Name:      name,
		Namespace: namespace,
		Image:     image,
	}
}

func (cc *ConfigCheck) Run() error {
	log := log.FromContext(context.TODO()).WithValues("Vector ConfigCheck", cc.Name)

	log.Info("start ConfigCheck")

	if err := cc.ensureVectorConfigCheckRBAC(); err != nil {
		return err
	}

	cc.Hash = randStringRunes()

	if err := cc.ensureVectorConfigCheckConfig(); err != nil {
		return err
	}

	if err := cc.checkVectorConfigCheckPod(); err != nil {
		return err
	}

	return nil
}

func (cc *ConfigCheck) ensureVectorConfigCheckRBAC() error {
	return cc.ensureVectorConfigCheckServiceAccount()
}

func (cc *ConfigCheck) ensureVectorConfigCheckServiceAccount() error {
	vectorAgentServiceAccount := cc.createVectorConfigCheckServiceAccount()

	return k8s.CreateOrUpdateServiceAccount(vectorAgentServiceAccount, cc.Client)
}
func (cc *ConfigCheck) ensureVectorConfigCheckConfig() error {
	vectorConfigCheckSecret, err := cc.createVectorConfigCheckConfig()
	if err != nil {
		return err
	}

	return k8s.CreateOrUpdateSecret(vectorConfigCheckSecret, cc.Client)
}

func (cc *ConfigCheck) checkVectorConfigCheckPod() error {
	vectorConfigCheckPod := cc.createVectorConfigCheckPod()

	err := k8s.CreatePod(vectorConfigCheckPod, cc.Client)
	if err != nil {
		return err
	}

	err = cc.getCheckResult(vectorConfigCheckPod)
	if err != nil {
		return err
	}

	err = cc.cleanup()
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
	n := "configcheck-" + "-" + cc.Name + "-" + cc.Hash

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

func (cc *ConfigCheck) getCheckResult(pod *corev1.Pod) error {
	log := log.FromContext(context.TODO()).WithValues("Vector ConfigCheck", pod.Name)

	for {
		existing, err := k8s.GetPod(pod, cc.Client)
		if err != nil {
			return err
		}

		switch existing.Status.Phase {
		case "Pending":
			log.Info("wait Validate Vector Config Result")
			time.Sleep(5 * time.Second)
		case "Failed":
			reason, err := k8s.GetPodLogs(pod, cc.ClientSet)
			if err != nil {
				return err
			}
			return &ConfigCheckError{
				Reason: reason,
			}
		case "Succeeded":
			log.Info("Config Check completed successfully")
			return nil
		}
	}
}

func (cc *ConfigCheck) cleanup() error {
	listOpts, err := cc.gcRListOptions()
	if err != nil {
		return err
	}

	podlist := corev1.PodList{}
	err = cc.Client.List(cc.Ctx, &podlist, &listOpts)
	if err != nil {
		return err
	}
	for _, pod := range podlist.Items {
		if pod.Status.Phase == "Succeeded" {
			for _, v := range pod.Spec.Volumes {
				if v.Name == "config" {
					secret := &corev1.Secret{}
					secretName := v.Secret.SecretName
					if err := cc.Client.Get(cc.Ctx, types.NamespacedName{Name: secretName, Namespace: pod.Namespace}, secret); err != nil {
						return err
					}
					if err := cc.Client.Delete(cc.Ctx, secret); err != nil {
						return err
					}
				}
			}
			if err := cc.Client.Delete(cc.Ctx, &pod); err != nil {
				return err
			}
		}
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
