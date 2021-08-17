/*
Copyright 2021 The Kubernetes Authors.

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

package topology

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha4"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileCluster(t *testing.T) {
	cluster1 := newFakeCluster(metav1.NamespaceDefault, "cluster1").Obj()
	cluster1WithReferences := newFakeCluster(metav1.NamespaceDefault, "cluster1").
		WithInfrastructureCluster(newFakeInfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster1").Obj()).
		WithControlPlane(newFakeControlPlane(metav1.NamespaceDefault, "control-plane1").Obj()).
		Obj()
	cluster2WithReferences := cluster1WithReferences.DeepCopy()
	cluster2WithReferences.SetGroupVersionKind(cluster1WithReferences.GroupVersionKind())
	cluster2WithReferences.Name = "cluster2"

	tests := []struct {
		name    string
		current *clusterv1.Cluster
		desired *clusterv1.Cluster
		want    *clusterv1.Cluster
		wantErr bool
	}{
		{
			name:    "Should update the cluster if infrastructure and control plane references are not set",
			current: cluster1,
			desired: cluster1WithReferences,
			want:    cluster1WithReferences,
			wantErr: false,
		},
		{
			name:    "Should be a no op if infrastructure and control plane references are already set",
			current: cluster2WithReferences,
			desired: cluster2WithReferences,
			want:    cluster2WithReferences,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeObjs := make([]client.Object, 0)
			if tt.current != nil {
				fakeObjs = append(fakeObjs, tt.current)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			currentState := &clusterTopologyState{cluster: tt.current}

			// TODO: stop setting ResourceVersion when building objects
			tt.desired.SetResourceVersion("")
			desiredState := &clusterTopologyState{cluster: tt.desired}

			r := ClusterReconciler{
				Client: fakeClient,
			}
			err := r.reconcileCluster(ctx, currentState, desiredState)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			got := tt.want.DeepCopy()
			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(tt.want), got)
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(got.Spec.InfrastructureRef).To(Equal(tt.want.Spec.InfrastructureRef), cmp.Diff(got, tt.want))
			g.Expect(got.Spec.ControlPlaneRef).To(Equal(tt.want.Spec.ControlPlaneRef), cmp.Diff(got, tt.want))
		})
	}
}

func TestReconcileInfrastructureCluster(t *testing.T) {
	g := NewWithT(t)

	clusterInfrastructure1 := newFakeInfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster1").Obj()
	clusterInfrastructure2 := newFakeInfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster2").Obj()
	clusterInfrastructure3 := newFakeInfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster3").Obj()
	clusterInfrastructure3WithInstanceSpecificChanges := clusterInfrastructure3.DeepCopy()
	clusterInfrastructure3WithInstanceSpecificChanges.SetLabels(map[string]string{"foo": "bar"})
	clusterInfrastructure4 := newFakeInfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster4").Obj()
	clusterInfrastructure4WithTemplateOverridingChanges := clusterInfrastructure4.DeepCopy()
	err := unstructured.SetNestedField(clusterInfrastructure4WithTemplateOverridingChanges.UnstructuredContent(), false, "spec", "fakeSetting")
	g.Expect(err).ToNot(HaveOccurred())
	clusterInfrastructure5 := newFakeInfrastructureCluster(metav1.NamespaceDefault, "infrastructure-cluster5").Obj()

	tests := []struct {
		name    string
		current *unstructured.Unstructured
		desired *unstructured.Unstructured
		want    *unstructured.Unstructured
		wantErr bool
	}{
		{
			name:    "Should create desired InfrastructureCluster if the current does not exists yet",
			current: nil,
			desired: clusterInfrastructure1,
			want:    clusterInfrastructure1,
			wantErr: false,
		},
		{
			name:    "No-op if current InfrastructureCluster is equal to desired",
			current: clusterInfrastructure2,
			desired: clusterInfrastructure2,
			want:    clusterInfrastructure2,
			wantErr: false,
		},
		{
			name:    "Should preserve instance specific changes",
			current: clusterInfrastructure3WithInstanceSpecificChanges,
			desired: clusterInfrastructure3,
			want:    clusterInfrastructure3WithInstanceSpecificChanges,
			wantErr: false,
		},
		{
			name:    "Should restore template values if overridden",
			current: clusterInfrastructure4WithTemplateOverridingChanges,
			desired: clusterInfrastructure4,
			want:    clusterInfrastructure4,
			wantErr: false,
		},
		{
			name:    "Fails for incompatible changes",
			current: clusterInfrastructure5,
			desired: clusterInfrastructure1,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeObjs := make([]client.Object, 0)
			if tt.current != nil {
				fakeObjs = append(fakeObjs, tt.current)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			currentState := &clusterTopologyState{infrastructureCluster: tt.current}

			// TODO: stop setting ResourceVersion when building objects
			tt.desired.SetResourceVersion("")
			desiredState := &clusterTopologyState{infrastructureCluster: tt.desired}

			r := ClusterReconciler{
				Client: fakeClient,
			}
			err := r.reconcileInfrastructureCluster(ctx, currentState, desiredState)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			got := tt.want.DeepCopy() // this is required otherwise Get will modify tt.want
			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(tt.want), got)
			g.Expect(err).ToNot(HaveOccurred())

			// Spec
			wantSpec, ok, err := unstructured.NestedMap(tt.want.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())

			gotSpec, ok, err := unstructured.NestedMap(got.UnstructuredContent(), "spec")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ok).To(BeTrue())
			for k, v := range wantSpec {
				g.Expect(gotSpec).To(HaveKeyWithValue(k, v))
			}
		})
	}
}

func TestReconcileMachineDeployments(t *testing.T) {
	infrastructureMachineTemplate1 := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-1").Obj()
	bootstrapTemplate1 := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-1").Obj()
	md1 := newFakeMachineDeploymentTopologyState("md-1", infrastructureMachineTemplate1, bootstrapTemplate1)

	infrastructureMachineTemplate2 := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-2").Obj()
	bootstrapTemplate2 := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-2").Obj()
	md2 := newFakeMachineDeploymentTopologyState("md-2", infrastructureMachineTemplate2, bootstrapTemplate2)
	infrastructureMachineTemplate2WithChanges := infrastructureMachineTemplate2.DeepCopy()
	infrastructureMachineTemplate2WithChanges.SetLabels(map[string]string{"foo": "bar"})
	md2WithRotatedInfrastructureMachineTemplate := newFakeMachineDeploymentTopologyState("md-2", infrastructureMachineTemplate2WithChanges, bootstrapTemplate2)

	infrastructureMachineTemplate3 := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-3").Obj()
	bootstrapTemplate3 := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-3").Obj()
	md3 := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3)
	bootstrapTemplate3WithChanges := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate3WithChanges.SetLabels(map[string]string{"foo": "bar"})
	md3WithRotatedBootstrapTemplate := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3WithChanges)
	bootstrapTemplate3WithChangeKind := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate3WithChangeKind.SetKind("AnotherGenericBootstrapTemplate")
	md3WithRotatedBootstrapTemplateChangedKind := newFakeMachineDeploymentTopologyState("md-3", infrastructureMachineTemplate3, bootstrapTemplate3WithChanges)

	infrastructureMachineTemplate4 := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-4").Obj()
	bootstrapTemplate4 := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-4").Obj()
	md4 := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate4, bootstrapTemplate4)
	infrastructureMachineTemplate4WithChanges := infrastructureMachineTemplate4.DeepCopy()
	infrastructureMachineTemplate4WithChanges.SetLabels(map[string]string{"foo": "bar"})
	bootstrapTemplate4WithChanges := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate4WithChanges.SetLabels(map[string]string{"foo": "bar"})
	md4WithRotatedTemplates := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate4WithChanges, bootstrapTemplate4WithChanges)

	infrastructureMachineTemplate5 := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-5").Obj()
	bootstrapTemplate5 := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-5").Obj()
	md5 := newFakeMachineDeploymentTopologyState("md-5", infrastructureMachineTemplate5, bootstrapTemplate5)
	infrastructureMachineTemplate5WithChangedKind := infrastructureMachineTemplate5.DeepCopy()
	infrastructureMachineTemplate5WithChangedKind.SetKind("ChangedKind")
	md5WithChangedInfrastructureMachineTemplateKind := newFakeMachineDeploymentTopologyState("md-4", infrastructureMachineTemplate5WithChangedKind, bootstrapTemplate5)

	infrastructureMachineTemplate6 := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-6").Obj()
	bootstrapTemplate6 := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-6").Obj()
	md6 := newFakeMachineDeploymentTopologyState("md-6", infrastructureMachineTemplate6, bootstrapTemplate6)
	bootstrapTemplate6WithChangedNamespace := bootstrapTemplate6.DeepCopy()
	bootstrapTemplate6WithChangedNamespace.SetNamespace("ChangedNamespace")
	md6WithChangedBootstrapTemplateNamespace := newFakeMachineDeploymentTopologyState("md-6", infrastructureMachineTemplate6, bootstrapTemplate6WithChangedNamespace)

	infrastructureMachineTemplate7 := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-7").Obj()
	bootstrapTemplate7 := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-7").Obj()
	md7 := newFakeMachineDeploymentTopologyState("md-7", infrastructureMachineTemplate7, bootstrapTemplate7)

	infrastructureMachineTemplate8Create := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-8-create").Obj()
	bootstrapTemplate8Create := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-8-create").Obj()
	md8Create := newFakeMachineDeploymentTopologyState("md-8-create", infrastructureMachineTemplate8Create, bootstrapTemplate8Create)
	infrastructureMachineTemplate8Delete := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-8-delete").Obj()
	bootstrapTemplate8Delete := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-8-delete").Obj()
	md8Delete := newFakeMachineDeploymentTopologyState("md-8-delete", infrastructureMachineTemplate8Delete, bootstrapTemplate8Delete)
	infrastructureMachineTemplate8Update := newFakeInfrastructureMachineTemplate(metav1.NamespaceDefault, "infrastructure-machine-8-update").Obj()
	bootstrapTemplate8Update := newFakeBootstrapTemplate(metav1.NamespaceDefault, "bootstrap-config-8-update").Obj()
	md8Update := newFakeMachineDeploymentTopologyState("md-8-update", infrastructureMachineTemplate8Update, bootstrapTemplate8Update)
	infrastructureMachineTemplate8UpdateWithChanges := infrastructureMachineTemplate8Update.DeepCopy()
	infrastructureMachineTemplate8UpdateWithChanges.SetLabels(map[string]string{"foo": "bar"})
	bootstrapTemplate8UpdateWithChanges := bootstrapTemplate3.DeepCopy()
	bootstrapTemplate8UpdateWithChanges.SetLabels(map[string]string{"foo": "bar"})
	md8UpdateWithRotatedTemplates := newFakeMachineDeploymentTopologyState("md-8-update", infrastructureMachineTemplate8UpdateWithChanges, bootstrapTemplate8UpdateWithChanges)

	tests := []struct {
		name                                      string
		current                                   []machineDeploymentTopologyState
		desired                                   []machineDeploymentTopologyState
		want                                      []machineDeploymentTopologyState
		wantInfrastructureMachineTemplateRotation map[string]bool
		wantBootstrapTemplateRotation             map[string]bool
		wantErr                                   bool
	}{
		{
			name:    "Should create desired MachineDeployment if the current does not exists yet",
			current: nil,
			desired: []machineDeploymentTopologyState{md1},
			want:    []machineDeploymentTopologyState{md1},
			wantErr: false,
		},
		{
			name:    "No-op if current MachineDeployment is equal to desired",
			current: []machineDeploymentTopologyState{md1},
			desired: []machineDeploymentTopologyState{md1},
			want:    []machineDeploymentTopologyState{md1},
			wantErr: false,
		},
		{
			name:    "Should update MachineDeployment with InfrastructureMachineTemplate rotation",
			current: []machineDeploymentTopologyState{md2},
			desired: []machineDeploymentTopologyState{md2WithRotatedInfrastructureMachineTemplate},
			want:    []machineDeploymentTopologyState{md2WithRotatedInfrastructureMachineTemplate},
			wantInfrastructureMachineTemplateRotation: map[string]bool{"md-2": true},
			wantErr: false,
		},
		{
			name:                          "Should update MachineDeployment with BootstrapTemplate rotation",
			current:                       []machineDeploymentTopologyState{md3},
			desired:                       []machineDeploymentTopologyState{md3WithRotatedBootstrapTemplate},
			want:                          []machineDeploymentTopologyState{md3WithRotatedBootstrapTemplate},
			wantBootstrapTemplateRotation: map[string]bool{"md-3": true},
			wantErr:                       false,
		},
		{
			name:                          "Should update MachineDeployment with BootstrapTemplate rotation with changed kind",
			current:                       []machineDeploymentTopologyState{md3},
			desired:                       []machineDeploymentTopologyState{md3WithRotatedBootstrapTemplateChangedKind},
			want:                          []machineDeploymentTopologyState{md3WithRotatedBootstrapTemplateChangedKind},
			wantBootstrapTemplateRotation: map[string]bool{"md-3": true},
			wantErr:                       false,
		},
		{
			name:    "Should update MachineDeployment with InfrastructureMachineTemplate and BootstrapTemplate rotation",
			current: []machineDeploymentTopologyState{md4},
			desired: []machineDeploymentTopologyState{md4WithRotatedTemplates},
			want:    []machineDeploymentTopologyState{md4WithRotatedTemplates},
			wantInfrastructureMachineTemplateRotation: map[string]bool{"md-4": true},
			wantBootstrapTemplateRotation:             map[string]bool{"md-4": true},
			wantErr:                                   false,
		},
		{
			name:    "Should fail update MachineDeployment because of changed InfrastructureMachineTemplate kind",
			current: []machineDeploymentTopologyState{md5},
			desired: []machineDeploymentTopologyState{md5WithChangedInfrastructureMachineTemplateKind},
			wantErr: true,
		},
		{
			name:    "Should fail update MachineDeployment because of changed BootstrapTemplate namespace",
			current: []machineDeploymentTopologyState{md6},
			desired: []machineDeploymentTopologyState{md6WithChangedBootstrapTemplateNamespace},
			wantErr: true,
		},
		{
			name:    "Should delete MachineDeployment",
			current: []machineDeploymentTopologyState{md7},
			desired: []machineDeploymentTopologyState{},
			want:    []machineDeploymentTopologyState{},
			wantErr: false,
		},
		{
			name:    "Should create, update and delete MachineDeployments",
			current: []machineDeploymentTopologyState{md8Update, md8Delete},
			desired: []machineDeploymentTopologyState{md8Create, md8UpdateWithRotatedTemplates},
			want:    []machineDeploymentTopologyState{md8Create, md8UpdateWithRotatedTemplates},
			wantInfrastructureMachineTemplateRotation: map[string]bool{"md-8-update": true},
			wantBootstrapTemplateRotation:             map[string]bool{"md-8-update": true},
			wantErr:                                   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			fakeObjs := make([]client.Object, 0)
			for _, mdts := range tt.current {
				fakeObjs = append(fakeObjs, mdts.object)
				fakeObjs = append(fakeObjs, mdts.infrastructureMachineTemplate)
				fakeObjs = append(fakeObjs, mdts.bootstrapTemplate)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(fakeScheme).
				WithObjects(fakeObjs...).
				Build()

			currentMachineDeploymentTopologyStates := toMachineDeploymentTopologyStateMap(tt.current)
			currentState := &clusterTopologyState{
				cluster:            newFakeCluster(metav1.NamespaceDefault, "cluster-1").Obj(),
				machineDeployments: currentMachineDeploymentTopologyStates,
			}

			// TODO: stop setting ResourceVersion when building objects
			for _, md := range tt.desired {
				md.object.SetResourceVersion("")
				md.bootstrapTemplate.SetResourceVersion("")
				md.infrastructureMachineTemplate.SetResourceVersion("")
			}
			desiredState := &clusterTopologyState{machineDeployments: toMachineDeploymentTopologyStateMap(tt.desired)}

			r := ClusterReconciler{
				Client: fakeClient,
			}
			err := r.reconcileMachineDeployments(ctx, currentState, desiredState)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			var gotMachineDeploymentList clusterv1.MachineDeploymentList
			g.Expect(fakeClient.List(ctx, &gotMachineDeploymentList)).To(Succeed())
			g.Expect(gotMachineDeploymentList.Items).To(HaveLen(len(tt.want)))

			for _, wantMachineDeploymentState := range tt.want {
				for _, gotMachineDeployment := range gotMachineDeploymentList.Items {
					if wantMachineDeploymentState.object.Name != gotMachineDeployment.Name {
						continue
					}
					currentMachineDeploymentTopologyName := wantMachineDeploymentState.object.ObjectMeta.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]
					currentMachineDeploymentTopologyState := currentMachineDeploymentTopologyStates[currentMachineDeploymentTopologyName]

					// Compare MachineDeployment.
					// Note: We're intentionally only comparing Spec as otherwise we would have to account for
					// empty vs. filled out TypeMeta.
					g.Expect(gotMachineDeployment.Spec).To(Equal(wantMachineDeploymentState.object.Spec))

					// Compare BootstrapTemplate.
					gotBootstrapTemplateRef := gotMachineDeployment.Spec.Template.Spec.Bootstrap.ConfigRef
					gotBootstrapTemplate := unstructured.Unstructured{}
					gotBootstrapTemplate.SetKind(gotBootstrapTemplateRef.Kind)
					gotBootstrapTemplate.SetAPIVersion(gotBootstrapTemplateRef.APIVersion)

					err = fakeClient.Get(ctx, client.ObjectKey{
						Namespace: gotBootstrapTemplateRef.Namespace,
						Name:      gotBootstrapTemplateRef.Name,
					}, &gotBootstrapTemplate)
					g.Expect(err).ToNot(HaveOccurred())
					// We don't want to compare resourceVersions as they are slightly different between the test cases
					// and it's not worth the effort.
					gotBootstrapTemplate.SetResourceVersion("")
					g.Expect(gotBootstrapTemplate).To(Equal(*wantMachineDeploymentState.bootstrapTemplate))

					// Check BootstrapTemplate rotation if there was a previous MachineDeployment/Template.
					if currentMachineDeploymentTopologyState.bootstrapTemplate != nil {
						if tt.wantBootstrapTemplateRotation[gotMachineDeployment.Name] {
							g.Expect(currentMachineDeploymentTopologyState.bootstrapTemplate.GetName()).ToNot(Equal(gotBootstrapTemplate.GetName()))
						} else {
							g.Expect(currentMachineDeploymentTopologyState.bootstrapTemplate.GetName()).To(Equal(gotBootstrapTemplate.GetName()))
						}
					}

					// Compare InfrastructureMachineTemplate.
					gotInfrastructureMachineTemplateRef := gotMachineDeployment.Spec.Template.Spec.InfrastructureRef
					gotInfrastructureMachineTemplate := unstructured.Unstructured{}
					gotInfrastructureMachineTemplate.SetKind(gotInfrastructureMachineTemplateRef.Kind)
					gotInfrastructureMachineTemplate.SetAPIVersion(gotInfrastructureMachineTemplateRef.APIVersion)

					err = fakeClient.Get(ctx, client.ObjectKey{
						Namespace: gotInfrastructureMachineTemplateRef.Namespace,
						Name:      gotInfrastructureMachineTemplateRef.Name,
					}, &gotInfrastructureMachineTemplate)
					g.Expect(err).ToNot(HaveOccurred())
					// We don't want to compare resourceVersions as they are slightly different between the test cases
					// and it's not worth the effort.
					gotInfrastructureMachineTemplate.SetResourceVersion("")
					g.Expect(gotInfrastructureMachineTemplate).To(Equal(*wantMachineDeploymentState.infrastructureMachineTemplate))

					// Check InfrastructureMachineTemplate rotation if there was a previous MachineDeployment/Template.
					if currentMachineDeploymentTopologyState.infrastructureMachineTemplate != nil {
						if tt.wantInfrastructureMachineTemplateRotation[gotMachineDeployment.Name] {
							g.Expect(currentMachineDeploymentTopologyState.infrastructureMachineTemplate.GetName()).ToNot(Equal(gotInfrastructureMachineTemplate.GetName()))
						} else {
							g.Expect(currentMachineDeploymentTopologyState.infrastructureMachineTemplate.GetName()).To(Equal(gotInfrastructureMachineTemplate.GetName()))
						}
					}
				}
			}
		})
	}
}

func newFakeMachineDeploymentTopologyState(name string, infrastructureMachineTemplate, bootstrapTemplate *unstructured.Unstructured) machineDeploymentTopologyState {
	return machineDeploymentTopologyState{
		object: newFakeMachineDeployment(metav1.NamespaceDefault, name).
			WithInfrastructureTemplate(infrastructureMachineTemplate).
			WithBootstrapTemplate(bootstrapTemplate).
			WithLabels(map[string]string{clusterv1.ClusterTopologyMachineDeploymentLabelName: name + "-topology"}).
			Obj(),
		infrastructureMachineTemplate: infrastructureMachineTemplate,
		bootstrapTemplate:             bootstrapTemplate,
	}
}

func toMachineDeploymentTopologyStateMap(states []machineDeploymentTopologyState) map[string]machineDeploymentTopologyState {
	ret := map[string]machineDeploymentTopologyState{}
	for _, state := range states {
		ret[state.object.Labels[clusterv1.ClusterTopologyMachineDeploymentLabelName]] = state
	}
	return ret
}

type referencedObjectsCompatibilityTestCase struct {
	name    string
	current *unstructured.Unstructured
	desired *unstructured.Unstructured
	wantErr bool
}

var referencedObjectsCompatibilityTestCases = []referencedObjectsCompatibilityTestCase{
	{
		name: "Fails if group changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "foo/v1alpha4",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "bar/v1alpha4",
			},
		},
		wantErr: true,
	},
	{
		name: "Fails if kind changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"kind": "bar",
			},
		},
		wantErr: true,
	},
	{
		name: "Pass if gvk remains the same",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Pass if version changes but group and kind remains the same",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/foo",
				"kind":       "foo",
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/bar",
				"kind":       "foo",
			},
		},
		wantErr: false,
	},
	{
		name: "Fails if namespace changes",
		current: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"namespace": "foo",
				},
			},
		},
		desired: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"namespace": "bar",
				},
			},
		},
		wantErr: true,
	},
}

func TestCheckReferencedObjectsAreCompatible(t *testing.T) {
	for _, tt := range referencedObjectsCompatibilityTestCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := checkReferencedObjectsAreCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestCheckReferencedObjectsAreStrictlyCompatible(t *testing.T) {
	referencedObjectsStrictCompatibilityTestCases := append(referencedObjectsCompatibilityTestCases, []referencedObjectsCompatibilityTestCase{
		{
			name: "Fails if name changes",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "bar",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Pass if name remains the same",
			current: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			desired: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "foo",
					},
				},
			},
			wantErr: false,
		},
	}...)

	for _, tt := range referencedObjectsStrictCompatibilityTestCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := checkReferencedObjectsAreStrictlyCompatible(tt.current, tt.desired)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}