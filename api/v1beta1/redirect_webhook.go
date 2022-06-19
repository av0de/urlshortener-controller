/*
Copyright 2022 Cedric Kienzler.

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

package v1beta1

import (
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var redirectlog = logf.Log.WithName("redirect-resource")

func (r *Redirect) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-urlredirector-av0-de-v1beta1-redirect,mutating=true,failurePolicy=fail,sideEffects=None,groups=urlredirector.av0.de,resources=redirects,verbs=create;update,versions=v1beta1,name=mredirect.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Redirect{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Redirect) Default() {
	redirectlog.Info("webhook defaulter", "name", r.ObjectMeta.Name)

	if r.Spec.HttpRedirect == nil {
		redirectlog.Info("defaulting to http 301 redirection", "name", r.ObjectMeta.Name)
		httpCode := http.StatusMovedPermanently
		r.Spec.HttpRedirect = &httpCode
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-urlredirector-av0-de-v1beta1-redirect,mutating=false,failurePolicy=fail,sideEffects=None,groups=urlredirector.av0.de,resources=redirects,verbs=create;update,versions=v1beta1,name=vredirect.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Redirect{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Redirect) ValidateCreate() error {
	redirectlog.Info("validate create", "name", r.ObjectMeta.Name)

	if r.Spec.HttpRedirect == nil {
		return fmt.Errorf("Currently only HttpRedirect is supported and must be specified")
	}

	return validateHttp(*r.Spec.HttpRedirect)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Redirect) ValidateUpdate(old runtime.Object) error {
	redirectlog.Info("validate update", "name", r.ObjectMeta.Name)

	if r.Spec.HttpRedirect == nil {
		return fmt.Errorf("Currently only HttpRedirect is supported and must be specified")
	}

	return validateHttp(*r.Spec.HttpRedirect)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Redirect) ValidateDelete() error {
	redirectlog.Info("validate delete", "name", r.ObjectMeta.Name)

	if r.Spec.HttpRedirect == nil {
		return fmt.Errorf("Currently only HttpRedirect is supported and must be specified")
	}

	return validateHttp(*r.Spec.HttpRedirect)
}

func validateHttp(httpCode int) error {
	if httpCode < 300 && httpCode > 399 {
		return fmt.Errorf("HTTP response code for redirect is specified as '%d' but http must be a valid 3xx redirect code", httpCode)
	}

	switch httpCode {
	case http.StatusUseProxy:
	case http.StatusSeeOther:
	case http.StatusNotModified:
	case http.StatusMultipleChoices:
		return fmt.Errorf("HTTP '%d' ('%s') is not supported", http.StatusMultipleChoices, http.StatusText(httpCode))

	case http.StatusMovedPermanently:
	case http.StatusFound:
	case http.StatusTemporaryRedirect:
	case http.StatusPermanentRedirect:
		return nil
	}

	return fmt.Errorf("Unknown http response code found")
}
