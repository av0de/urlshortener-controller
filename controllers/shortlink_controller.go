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

package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	openfaasv1 "github.com/openfaas/faas-netes/pkg/apis/openfaas/v1"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/av0de/urlshortener-controller/api/v1alpha1"
	urlshortenerv1alpha1 "github.com/av0de/urlshortener-controller/api/v1alpha1"
	pkg_redirect "github.com/av0de/urlshortener-controller/pkg/redirect"
)

// ShortLinkReconciler reconciles a ShortLink object
type ShortLinkReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	Log logr.Logger
}

//+kubebuilder:rbac:groups=urlshortener.av0.de,resources=shortlinks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=urlshortener.av0.de,resources=shortlinks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=urlshortener.av0.de,resources=shortlinks/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ShortLink object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *ShortLinkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	log := r.Log.WithValues("shortlink", req.NamespacedName)

	// fetch shortlink object
	shortlink := &v1alpha1.ShortLink{}
	err := r.Get(ctx, req.NamespacedName, shortlink)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("ShortLink resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get ShortLink")
		return ctrl.Result{}, err
	}

	//result, err := r.deployOrUpdateFunction(log, ctx, shortlink)
	//if err != nil {
	//	return result, err
	//}

	result, err := r.deployEtcdSts(log, ctx, shortlink)
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *ShortLinkReconciler) deployEtcdSts(log logr.Logger, ctx context.Context, shortlink *v1alpha1.ShortLink) (ctrl.Result, error) {
	found := &appsv1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: shortlink.Name, Namespace: shortlink.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		etcdSts, err := pkg_redirect.GetEtcdSts(fmt.Sprintf("etcd-%s", shortlink.Name), shortlink.Namespace, labelsForRedirect(shortlink.Name))
		if err != nil {
			log.Error(err, "Failed to create new etcd stateful set")
			return ctrl.Result{}, err
		}

		log.Info("Creating a new etcd stateful set")
		err = r.Create(ctx, etcdSts)
		if err != nil {
			log.Error(err, "Failed to create new etcd stateful set")
			return ctrl.Result{}, err
		}
		// Ingress created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

func (r *ShortLinkReconciler) deployOrUpdateFunction(log logr.Logger, ctx context.Context, shortlink *v1alpha1.ShortLink) (ctrl.Result, error) {
	// Check if the function already exists, if not create a new one
	found := &openfaasv1.Function{}
	err := r.Get(ctx, types.NamespacedName{Name: shortlink.Name, Namespace: shortlink.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		log = log.WithValues("Ingress.Namespace", found.Namespace, "Ingress.Name", found.Name)

		// Define a new ingress
		ing := r.functionForShortLink(shortlink)
		log.Info("Creating a new function")
		err = r.Create(ctx, ing)
		if err != nil {
			log.Error(err, "Failed to create new Function")
			return ctrl.Result{}, err
		}
		// Ingress created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Ingress")
		return ctrl.Result{}, err
	}

	log = log.WithValues("Function.Namespace", found.Namespace, "Function.Name", found.Name)

	// Ensure the target is correct
	if val, ok := (*found.Spec.Environment)["Target"]; ok && val != shortlink.Spec.Target {
		oldTarget := (*found.Spec.Environment)["Target"]
		log.Info("Update Function shortlink target", "oldTarget", oldTarget, "newTarget", shortlink.Spec.Target)

		(*found.Spec.Environment)["Target"] = shortlink.Spec.Target
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Function shortlink target")
			return ctrl.Result{}, err
		}
		// Spec updated - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	// Ensure alias is up to date
	if val, ok := (*found.Spec.Environment)["Alias"]; ok && val != shortlink.Spec.Alias {
		oldAlias := (*found.Spec.Environment)["Alias"]
		log.Info("Update Function shortlink alias", "oldAlias", oldAlias, "newTarget", shortlink.Spec.Alias)

		(*found.Spec.Environment)["Alias"] = shortlink.Spec.Alias
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Function shortlink alias")
			return ctrl.Result{}, err
		}
		// Spec updated - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

func (r *ShortLinkReconciler) functionForShortLink(shortLink *v1alpha1.ShortLink) *openfaasv1.Function {
	fn := &openfaasv1.Function{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shortLink.Name,
			Namespace: shortLink.Namespace,
			Labels:    labelsForShortLink(shortLink.Name),
		},
		Spec: openfaasv1.FunctionSpec{
			Environment: &map[string]string{
				"Alias":  shortLink.Spec.Alias,
				"Target": shortLink.Spec.Target,
			},
			Handler:                "env",
			Image:                  "ghcr.io/openfaas/alpine:latest",
			Name:                   shortLink.Name,
			ReadOnlyRootFilesystem: true,
		},
	}

	// Set Memcached instance as the owner and controller
	ctrl.SetControllerReference(shortLink, fn, r.Scheme)

	return fn
}

// labelsForShortLink returns the labels for selecting the resources
// belonging to the given redirect CR name.
func labelsForShortLink(name string) map[string]string {
	return map[string]string{"app": "shortlink", "redirect_cr": name}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ShortLinkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&urlshortenerv1alpha1.ShortLink{}).
		Owns(&openfaasv1.Function{}).
		Complete(r)
}
