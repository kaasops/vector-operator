package configcheck

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/kaasops/vector-operator/controllers/factory/helper"
	"github.com/kaasops/vector-operator/controllers/factory/k8sutils"
	"github.com/kaasops/vector-operator/controllers/factory/label"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	ErrConfigCheck = errors.New("Config Check finish with errors")
)

func Run(
	cfg []byte,
	c client.Client,
	name,
	namespace,
	image string,
) error {
	log := log.FromContext(context.TODO()).WithValues("Vector ConfigCheck", name)

	log.Info("start ConfigCheck Vector")

	err := ensureVectorConfigCheckRBAC(c, namespace)
	if err != nil {
		return err
	}

	hash := randStringRunes()

	err = ensureVectorConfigCheckConfig(c, cfg, name, namespace, hash)
	if err != nil {
		return err
	}

	err = ensureVectorConfigCheckPod(c, name, namespace, image, hash)
	if err != nil {
		return err
	}

	return nil
}

func ensureVectorConfigCheckRBAC(c client.Client, ns string) error {
	// ctx := context.Background()
	// log := log.FromContext(ctx).WithValues("vector-config-check-rbac", "ConfigCheck")

	// log.Info("start Reconcile Vector Config Check RBAC")

	if done, _, err := ensureVectorConfigCheckServiceAccount(c, ns); done {
		return err
	}

	return nil
}

func ensureVectorConfigCheckServiceAccount(c client.Client, ns string) (bool, ctrl.Result, error) {
	vectorAgentServiceAccount := createVectorConfigCheckServiceAccount(ns)

	_, err := k8sutils.CreateOrUpdateServiceAccount(vectorAgentServiceAccount, c)

	return helper.ReconcileResult(err)
}
func ensureVectorConfigCheckConfig(c client.Client, cfg []byte, name, ns, hash string) error {
	// ctx := context.Background()
	// log := log.FromContext(ctx).WithValues("vector-config-check-secret", "ConfigCheck")

	// log.Info("start Create Config Check Secret")

	vectorConfigCheckSecret, err := createVectorConfigCheckConfig(cfg, name, ns, hash)
	if err != nil {
		return err
	}

	_, err = k8sutils.CreateOrUpdateSecret(vectorConfigCheckSecret, c)

	return err
}

func ensureVectorConfigCheckPod(c client.Client, name, ns, image, hash string) error {
	// ctx := context.Background()
	// log := log.FromContext(ctx).WithValues("vector-config-check-pod", "ConfigCheck")

	// log.Info("start Vector Config Check Pod")

	vectorConfigCheckPod := createVectorConfigCheckPod(name, ns, image, hash)

	err := k8sutils.CreatePod(vectorConfigCheckPod, c)
	if err != nil {
		return err
	}

	err = getCheckResult(vectorConfigCheckPod, c)
	if err != nil {
		return err
	}

	return nil
}

func labelsForVectorConfigCheck() map[string]string {
	return map[string]string{
		label.ManagedByLabelKey:  "vector-operator",
		label.NameLabelKey:       "vector-configcheck",
		label.ComponentLabelKey:  "ConfigCheck",
		label.VectorExcludeLabel: "true",
	}
}

func getNameVectorConfigCheck(name, hash string) string {
	n := "configcheck-" + name + "-" + hash
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

func getCheckResult(pod *corev1.Pod, c client.Client) error {
	log := log.FromContext(context.TODO()).WithValues("Vector ConfigCheck", pod.Name)

	for {
		existing, err := k8sutils.GetPod(pod, c)
		if err != nil {
			return err
		}

		switch existing.Status.Phase {
		case "Pending":
			log.Info("wait Validate Vector Config Result")
			time.Sleep(5 * time.Second)
		case "Failed":
			return ErrConfigCheck
		case "Succeeded":
			log.Info("Config Check completed successfully")
			return nil
		}
	}
}