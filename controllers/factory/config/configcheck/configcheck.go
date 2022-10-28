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

	"github.com/kaasops/vector-operator/controllers/factory/utils/helper"
	"github.com/kaasops/vector-operator/controllers/factory/utils/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
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

// func (cfg *Config) StartCheck() error {

// 	log := log.FromContext(context.TODO()).WithValues("ConfigCheck Vector Pipeline", cfg.vaCtrl.Vector.Name)

// 	configCheck := configcheck.New(cfg.Ctx, cfg.ByteConfig, cfg.vaCtrl.Client, cfg.vaCtrl.ClientSet, cfg.Name, cfg.vaCtrl.Vector.Namespace, cfg.vaCtrl.Vector.Spec.Agent.Image)

// 	err := configCheck.Run()
// 	if _, ok := err.(*configcheck.ErrConfigCheck); ok {
// 		if err := cfg.vaCtrl.SetFailedStatus(cfg.Ctx, err); err != nil {
// 			return err
// 		}
// 		log.Error(err, "Vector Config has error")
// 		return nil
// 	}
// 	if err != nil {
// 		return err
// 	}

// 	return cfg.vaCtrl.SetSucceesStatus(cfg.Ctx)
// }

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
	if done, _, err := cc.ensureVectorConfigCheckServiceAccount(); done {
		return err
	}

	return nil
}

func (cc *ConfigCheck) ensureVectorConfigCheckServiceAccount() (bool, ctrl.Result, error) {
	vectorAgentServiceAccount := cc.createVectorConfigCheckServiceAccount()

	_, err := k8s.CreateOrUpdateServiceAccount(vectorAgentServiceAccount, cc.Client)

	return helper.ReconcileResult(err)
}
func (cc *ConfigCheck) ensureVectorConfigCheckConfig() error {
	vectorConfigCheckSecret, err := cc.createVectorConfigCheckConfig()
	if err != nil {
		return err
	}

	_, err = k8s.CreateOrUpdateSecret(vectorConfigCheckSecret, cc.Client)

	return err
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
			return &ErrConfigCheck{
				Reason: reason,
			}
		case "Succeeded":
			log.Info("Config Check completed successfully")
			return nil
		}
	}
}
