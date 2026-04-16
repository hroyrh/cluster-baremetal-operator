package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestTLSConfigWatcher_AdherenceChange(t *testing.T) {
	scheme := setUpSchemeForReconciler()

	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
		},
	}

	changed := false
	watcher := &TLSConfigWatcher{
		Client:           fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build(),
		InitialAdherence: configv1.TLSAdherencePolicyNoOpinion,
		OnChange:         func() { changed = true },
	}

	_, err := watcher.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "cluster"},
	})
	assert.NoError(t, err)
	assert.True(t, changed, "OnChange should fire when tlsAdherence changes")
	assert.Equal(t, configv1.TLSAdherencePolicyStrictAllComponents, watcher.InitialAdherence,
		"InitialAdherence should be updated after change")
}

func TestTLSConfigWatcher_ProfileChange(t *testing.T) {
	scheme := setUpSchemeForReconciler()

	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
		},
	}

	changed := false
	watcher := &TLSConfigWatcher{
		Client:             fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build(),
		InitialAdherence:   configv1.TLSAdherencePolicyStrictAllComponents,
		InitialProfileSpec: *configv1.TLSProfiles[configv1.TLSProfileIntermediateType],
		OnChange:           func() { changed = true },
	}

	_, err := watcher.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "cluster"},
	})
	assert.NoError(t, err)
	assert.True(t, changed, "OnChange should fire when TLS profile changes")
}

func TestTLSConfigWatcher_NoChange(t *testing.T) {
	scheme := setUpSchemeForReconciler()

	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSAdherence: configv1.TLSAdherencePolicyStrictAllComponents,
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
		},
	}

	changed := false
	watcher := &TLSConfigWatcher{
		Client:             fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build(),
		InitialAdherence:   configv1.TLSAdherencePolicyStrictAllComponents,
		InitialProfileSpec: *configv1.TLSProfiles[configv1.TLSProfileIntermediateType],
		OnChange:           func() { changed = true },
	}

	_, err := watcher.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "cluster"},
	})
	assert.NoError(t, err)
	assert.False(t, changed, "OnChange should not fire when nothing changed")
}

func TestTLSConfigWatcher_ProfileChangeIgnoredWhenNotEnforcing(t *testing.T) {
	scheme := setUpSchemeForReconciler()

	// tlsAdherence is NoOpinion, so profile changes should not trigger OnChange
	apiServer := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSAdherence: configv1.TLSAdherencePolicyNoOpinion,
			TLSSecurityProfile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
		},
	}

	changed := false
	watcher := &TLSConfigWatcher{
		Client:             fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(apiServer).Build(),
		InitialAdherence:   configv1.TLSAdherencePolicyNoOpinion,
		InitialProfileSpec: *configv1.TLSProfiles[configv1.TLSProfileIntermediateType],
		OnChange:           func() { changed = true },
	}

	_, err := watcher.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "cluster"},
	})
	assert.NoError(t, err)
	assert.False(t, changed, "profile changes should be ignored when not enforcing")
}

func TestTLSConfigWatcher_NotFoundIsNoop(t *testing.T) {
	scheme := setUpSchemeForReconciler()

	changed := false
	watcher := &TLSConfigWatcher{
		Client:           fakeclient.NewClientBuilder().WithScheme(scheme).Build(),
		InitialAdherence: configv1.TLSAdherencePolicyNoOpinion,
		OnChange:         func() { changed = true },
	}

	_, err := watcher.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "cluster"},
	})
	assert.NoError(t, err)
	assert.False(t, changed, "OnChange should not fire when APIServer not found")
}
