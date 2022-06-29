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
	"net/http"
	"reflect"

	"github.com/go-logr/logr"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/av0de/urlshortener-controller/api/v1alpha1"
	urlshortenerv1alpha1 "github.com/av0de/urlshortener-controller/api/v1alpha1"
)

// RedirectReconciler reconciles a Redirect object
type RedirectReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=urlshortener.av0.de,resources=redirects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=urlshortener.av0.de,resources=redirects/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=urlshortener.av0.de,resources=redirects/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Redirect object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.1/pkg/reconcile
func (r *RedirectReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	log := r.Log.WithValues("redirect", req.NamespacedName)

	// fetch redirect object
	redirect := &v1alpha1.Redirect{}
	err := r.Get(ctx, req.NamespacedName, redirect)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("Redirect resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}

		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get Redirect")
		return ctrl.Result{}, err
	}

	// Check if the ingress already exists, if not create a new one
	found := &networkingv1.Ingress{}
	err = r.Get(ctx, types.NamespacedName{Name: redirect.Name, Namespace: redirect.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		// Define a new ingress
		ing := r.ingressForRedirect(redirect)
		log.Info("Creating a new Ingress", "Ingress.Namespace", ing.Namespace, "Ingress.Name", ing.Name)
		err = r.Create(ctx, ing)
		if err != nil {
			log.Error(err, "Failed to create new Ingress", "Ingress.Namespace", ing.Namespace, "Ingress.Name", ing.Name)
			return ctrl.Result{}, err
		}
		// Ingress created successfully - return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		log.Error(err, "Failed to get Ingress")
		return ctrl.Result{}, err
	}

	log = log.WithValues("Ingress.Namespace", found.Namespace, "Ingress.Name", found.Name)

	// Ensure the target is correct
	newTarget := fmt.Sprintf("http://%s", redirect.Spec.Target)
	if val, ok := found.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/permanent-redirect"]; ok && val != newTarget {
		oldTarget := found.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/permanent-redirect"]
		log.Info("Update Ingress redirection target", "oldTarget", oldTarget, "newTarget", newTarget)

		found.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/permanent-redirect"] = newTarget
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Ingress redirection target")
			return ctrl.Result{}, err
		}
		// Spec updated - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	// Ensure the source is correct
	if len(found.Spec.Rules) == 1 && found.Spec.Rules[0].Host != redirect.Spec.Source {
		oldHost := found.Spec.Rules[0].Host
		newHost := redirect.Spec.Source
		log.Info("Update Ingress redirection target", "oldHost", oldHost, "newHost", newHost)

		found.Spec.Rules[0].Host = newHost
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Ingress redirection source")
			return ctrl.Result{}, err
		}
		// Spec updated - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	// Ensure the redirect code is correct
	if val, ok := found.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/permanent-redirect-code"]; ok && val != fmt.Sprintf("%d", redirect.Spec.Code) {
		oldCode := found.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/permanent-redirect-code"]
		newCode := fmt.Sprintf("%d", redirect.Spec.Code)

		log.Info("Update Ingress redirection code", "oldCode", oldCode, "newCode", newCode)

		found.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/permanent-redirect-code"] = newCode
		err = r.Update(ctx, found)
		if err != nil {
			log.Error(err, "Failed to update Ingress redirection code")
			return ctrl.Result{}, err
		}
		// Spec updated - return and requeue
		return ctrl.Result{Requeue: true}, nil
	}

	// Update the Redirect status with the ingress name and the target
	// List the pods for this memcached's deployment
	ingressList := &networkingv1.IngressList{}
	listOpts := []client.ListOption{
		client.InNamespace(redirect.Namespace),
		client.MatchingLabels(labelsForRedirect(redirect.Name)),
	}
	if err = r.List(ctx, ingressList, listOpts...); err != nil {
		log.Error(err, "Failed to list pods", "Redirect.Namespace", redirect.Namespace, "Redirect.Name", redirect.Name)
		return ctrl.Result{}, err
	}
	ingressNames := getIngressNames(ingressList.Items)

	// Update status.Nodes if needed
	if !reflect.DeepEqual(ingressNames, redirect.Status.IngressName) {
		log.Info("Update Redirect status Ingress name(s)")
		redirect.Status.IngressName = ingressNames
		err := r.Status().Update(ctx, redirect)
		if err != nil {
			log.Error(err, "Failed to update Redirect status Ingress name(s)")
			return ctrl.Result{}, err
		}
	}

	redirect.Status.Target = found.ObjectMeta.Annotations["nginx.ingress.kubernetes.io/permanent-redirect"]
	log.Info("Update Redirect status Ingress redirect code")
	err = r.Status().Update(ctx, redirect)
	if err != nil {
		log.Error(err, "Failed to update Redirect status Ingress redirect code")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RedirectReconciler) ingressForRedirect(redirect *v1alpha1.Redirect) *networkingv1.Ingress {
	pathTypePrefix := networkingv1.PathTypePrefix

	switch redirect.Spec.Code {
	case http.StatusMultipleChoices:
	case http.StatusMovedPermanently:
	case http.StatusFound:
	case http.StatusSeeOther:
	case http.StatusNotModified:
	case http.StatusUseProxy:
	case http.StatusTemporaryRedirect:
	case http.StatusPermanentRedirect:
		break

	default:
		redirect.Spec.Code = http.StatusPermanentRedirect
	}

	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redirect.Name,
			Namespace: redirect.Namespace,
			Labels:    labelsForRedirect(redirect.Name),
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/permanent-redirect":      fmt.Sprintf("http://%s", redirect.Spec.Target),
				"nginx.ingress.kubernetes.io/permanent-redirect-code": fmt.Sprintf("%d", redirect.Spec.Code),
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: redirect.Spec.Source,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathTypePrefix,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "http-svc",
											Port: networkingv1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	// Set Memcached instance as the owner and controller
	ctrl.SetControllerReference(redirect, ing, r.Scheme)
	return ing
}

// labelsForRedirect returns the labels for selecting the resources
// belonging to the given redirect CR name.
func labelsForRedirect(name string) map[string]string {
	return map[string]string{"app": "redirect", "redirect_cr": name}
}

func getIngressNames(ingresses []networkingv1.Ingress) []string {
	var ingressNames []string
	for _, ingress := range ingresses {
		ingressNames = append(ingressNames, ingress.Name)
	}
	return ingressNames
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedirectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&urlshortenerv1alpha1.Redirect{}).
		Owns(&networkingv1.Ingress{}).
		Complete(r)
}
