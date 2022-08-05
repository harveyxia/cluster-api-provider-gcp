package controllers

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	expinfrav1 "sigs.k8s.io/cluster-api-provider-gcp/exp/api/v1beta1"
)

var _ = Describe("GCPMachinePoolReconciler", func() {
	It("should succesfully reconcile a GCPMachinePool", func() {
		When("there are no capi resources defined", func() {
			r := &GCPMachinePoolReconciler{
				Client: kube,
			}

			ctx := context.Background()
			gcpMachinePool := &expinfrav1.GCPMachinePool{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-machinepool",
					Namespace: "test-ns",
				},
			}

			result, err := r.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKeyFromObject(gcpMachinePool),
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(BeZero())
		})
	})
})
