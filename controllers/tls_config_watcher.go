/*

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
	"reflect"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/cluster-baremetal-operator/provisioning"
	utiltls "github.com/openshift/controller-runtime-common/pkg/tls"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// TLSConfigWatcher watches the APIServer CR for changes to both the TLS
// security profile and the tlsAdherence field. When either changes from
// the values seen at startup, it calls OnChange (typically to cancel the
// manager context and trigger a graceful restart).
//
// This replaces the upstream SecurityProfileWatcher which only watches
// TLSProfileSpec changes and must be conditionally registered.
// TLSConfigWatcher is always registered so it can detect transitions in
// either direction (e.g. tlsAdherence going from "" to
// "StrictAllComponents" or vice versa).
type TLSConfigWatcher struct {
	client.Client

	// InitialProfileSpec is the resolved TLS profile spec at startup.
	// Zero-value when TLS enforcement is not active.
	InitialProfileSpec configv1.TLSProfileSpec

	// InitialAdherence is the tlsAdherence value read at startup.
	InitialAdherence configv1.TLSAdherencePolicy

	// OnChange is called when either the TLS profile or tlsAdherence
	// changes from the initial values. Typically calls cancel() to
	// trigger a graceful shutdown.
	OnChange func()
}

// SetupWithManager registers the controller with the manager.
func (w *TLSConfigWatcher) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("tlsconfigwatcher").
		For(&configv1.APIServer{}, builder.WithPredicates(
			predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					return e.Object.GetName() == utiltls.APIServerName
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					return e.ObjectNew.GetName() == utiltls.APIServerName
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					return e.Object.GetName() == utiltls.APIServerName
				},
				GenericFunc: func(e event.GenericEvent) bool {
					return e.Object.GetName() == utiltls.APIServerName
				},
			},
		)).
		WithLogConstructor(func(_ *reconcile.Request) klog.Logger {
			return mgr.GetLogger().WithValues(
				"controller", "tlsconfigwatcher",
			)
		}).
		Complete(w)
}

// Reconcile checks for changes to tlsAdherence or the TLS profile and
// triggers OnChange when either has diverged from the startup value.
func (w *TLSConfigWatcher) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	apiServer := &configv1.APIServer{}
	if err := w.Get(ctx, req.NamespacedName, apiServer); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("failed to get APIServer %s: %w", req.NamespacedName, err)
	}

	// Check tlsAdherence change.
	if apiServer.Spec.TLSAdherence != w.InitialAdherence {
		klog.Infof("TLS adherence changed from %q to %q, initiating shutdown",
			w.InitialAdherence, apiServer.Spec.TLSAdherence)
		if w.OnChange != nil {
			w.OnChange()
		}
		w.InitialAdherence = apiServer.Spec.TLSAdherence
		return ctrl.Result{}, nil
	}

	// Check TLS profile change only when enforcement is active.
	// When not enforcing, the profile is not applied so changes to it
	// don't require a restart.
	if provisioning.ShouldHonorClusterTLSProfile(w.InitialAdherence) {
		currentSpec, err := utiltls.GetTLSProfileSpec(apiServer.Spec.TLSSecurityProfile)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to get TLS profile: %w", err)
		}
		if !reflect.DeepEqual(w.InitialProfileSpec, currentSpec) {
			klog.Infof("TLS profile changed, initiating shutdown. old: %+v, new: %+v",
				w.InitialProfileSpec, currentSpec)
			if w.OnChange != nil {
				w.OnChange()
			}
			w.InitialProfileSpec = currentSpec
		}
	}

	return ctrl.Result{}, nil
}
