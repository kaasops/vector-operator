/*
Copyright 2024.

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

package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	api_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NamespaceIsTerminating reports whether the named namespace is being deleted or
// already gone. Reconciling resources (secrets, daemonsets, configcheck pods) into
// such a namespace is futile — the API server rejects new content — so callers
// should skip instead of erroring and requeuing forever.
func NamespaceIsTerminating(ctx context.Context, c client.Client, name string) (bool, error) {
	ns := &corev1.Namespace{}
	if err := c.Get(ctx, types.NamespacedName{Name: name}, ns); err != nil {
		if api_errors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return ns.DeletionTimestamp != nil, nil
}
