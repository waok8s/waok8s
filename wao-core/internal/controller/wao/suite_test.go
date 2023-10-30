package wao

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/waok8s/wao-core/api/wao/v1beta1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = v1beta1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

var (
	wait   = func() { time.Sleep(100 * time.Millisecond) }
	testNS = "wao-system"

	labelHostname  = "kubernetes.io/hostname"
	testNode0Name  = "node-0"
	testNode1Name  = "node-1"
	testNode0Addr  = "10.0.0.100"
	testNode1Addr  = "10.0.0.101"
	testLabel      = "test-label"
	testLabelValue = "test-label-value"
	testNode0      = corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testNode0Name,
			Labels: map[string]string{labelHostname: testNode0Name, testLabel: testLabelValue}},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: testNode0Addr}},
		},
	}
	testNode1 = corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   testNode1Name,
			Labels: map[string]string{labelHostname: testNode1Name, testLabel: testLabelValue}},
		Status: corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: testNode1Addr}},
		},
	}
	testNCT0Name = "nct0"
	testNCT0EP   = "http://{{ .IPv4.Octet1 }}.{{ .IPv4.Octet2 }}.{{ .IPv4.Octet3 }}.{{ .IPv4.Octet4 }}/{{ .IPv4.Address }}-{{ .Hostname }}"
	testNCT0     = v1beta1.NodeConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNCT0Name,
			Namespace: testNS,
		},
		Spec: v1beta1.NodeConfigTemplateSpec{
			NodeSelector: metav1.LabelSelector{MatchLabels: map[string]string{testLabel: testLabelValue}},
			Template: v1beta1.NodeConfigSpec{
				NodeName: "",
				MetricsCollector: v1beta1.MetricsCollector{
					InletTemp: v1beta1.EndpointTerm{
						Type:     v1beta1.TypeFake,
						Endpoint: testNCT0EP,
					},
					DeltaP: v1beta1.EndpointTerm{
						Type:     v1beta1.TypeFake,
						Endpoint: testNCT0EP,
					},
				},
				Predictor: v1beta1.Predictor{
					PowerConsumptionEndpointProvider: &v1beta1.EndpointTerm{
						Type:     v1beta1.TypeFake,
						Endpoint: testNCT0EP,
					},
				},
			},
		},
	}
	testNC0EP = "http://10.0.0.100/10.0.0.100-node-0"
	testNC1EP = "http://10.0.0.101/10.0.0.101-node-1"
	testNC0   = v1beta1.NodeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNCT0Name + "-" + testNode0Name,
			Namespace: testNS,
		},
		Spec: v1beta1.NodeConfigSpec{
			NodeName: testNode0Name,
			MetricsCollector: v1beta1.MetricsCollector{
				InletTemp: v1beta1.EndpointTerm{
					Type:     v1beta1.TypeFake,
					Endpoint: testNC0EP,
				},
				DeltaP: v1beta1.EndpointTerm{
					Type:     v1beta1.TypeFake,
					Endpoint: testNC0EP,
				},
			},
			Predictor: v1beta1.Predictor{
				PowerConsumptionEndpointProvider: &v1beta1.EndpointTerm{
					Type:     v1beta1.TypeFake,
					Endpoint: testNC0EP,
				},
			},
		},
	}
	testNC1 = v1beta1.NodeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNCT0Name + "-" + testNode1Name,
			Namespace: testNS,
		},
		Spec: v1beta1.NodeConfigSpec{
			NodeName: testNode1Name,
			MetricsCollector: v1beta1.MetricsCollector{
				InletTemp: v1beta1.EndpointTerm{
					Type:     v1beta1.TypeFake,
					Endpoint: testNC1EP,
				},
				DeltaP: v1beta1.EndpointTerm{
					Type:     v1beta1.TypeFake,
					Endpoint: testNC1EP,
				},
			},
			Predictor: v1beta1.Predictor{
				PowerConsumptionEndpointProvider: &v1beta1.EndpointTerm{
					Type:     v1beta1.TypeFake,
					Endpoint: testNC1EP,
				},
			},
		},
	}
)

var _ = Describe("NodeConfigTemplate Controller", func() {
	var cncl context.CancelFunc

	BeforeEach(func() {
		ctx, cancel := context.WithCancel(context.Background())
		cncl = cancel

		var err error

		err = k8sClient.DeleteAllOf(ctx, &v1beta1.NodeConfigTemplate{}, client.InNamespace(testNS))
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.DeleteAllOf(ctx, &v1beta1.NodeConfig{}, client.InNamespace(testNS))
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.DeleteAllOf(ctx, &corev1.Secret{}, client.InNamespace(testNS))
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.DeleteAllOf(ctx, &corev1.Node{}, client.InNamespace(testNS))
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() int {
			var objs v1beta1.NodeConfigTemplateList
			err = k8sClient.List(ctx, &objs, client.InNamespace(testNS))
			Expect(err).NotTo(HaveOccurred())
			return len(objs.Items)
		}).Should(Equal(0))
		Eventually(func() int {
			var objs v1beta1.NodeConfigList
			err = k8sClient.List(ctx, &objs, client.InNamespace(testNS))
			Expect(err).NotTo(HaveOccurred())
			return len(objs.Items)
		}).Should(Equal(0))
		Eventually(func() int {
			var objs corev1.SecretList
			err = k8sClient.List(ctx, &objs, client.InNamespace(testNS))
			Expect(err).NotTo(HaveOccurred())
			return len(objs.Items)
		}).Should(Equal(0))
		Eventually(func() int {
			var objs corev1.NodeList
			err = k8sClient.List(ctx, &objs, client.InNamespace(testNS))
			Expect(err).NotTo(HaveOccurred())
			return len(objs.Items)
		}).Should(Equal(0))

		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme.Scheme,
		})
		Expect(err).NotTo(HaveOccurred())

		reconciler := NodeConfigTemplateReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}
		err = reconciler.SetupWithManager(mgr)
		Expect(err).NotTo(HaveOccurred())

		go func() {
			err := mgr.Start(ctx)
			if err != nil {
				panic(err)
			}
		}()
		wait()
	})

	AfterEach(func() {
		cncl() // stop the mgr
		wait()
	})

	It("should create a NodeConfigTemplate and NodeConfigs", func() {
		ctx := context.Background()

		var err error

		// Setup Namespace and Nodes
		k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNS}})
		err = k8sClient.Create(ctx, &testNode0)
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.Create(ctx, &testNode1)
		Expect(err).NotTo(HaveOccurred())

		// Create NodeConfigTemplate
		err = k8sClient.Create(ctx, &testNCT0)
		Expect(err).NotTo(HaveOccurred())

		// Check NodeConfigs are created
		Eventually(func() error {

			var obj v1beta1.NodeConfig
			err = k8sClient.Get(ctx, client.ObjectKey{Name: testNC0.Name, Namespace: testNS}, &obj)
			if err != nil {
				return err
			}
			return compareNodeConfig(obj, testNC0)
		}).ShouldNot(HaveOccurred())

		Eventually(func() error {
			var obj v1beta1.NodeConfig
			err = k8sClient.Get(ctx, client.ObjectKey{Name: testNC1.Name, Namespace: testNS}, &obj)
			if err != nil {
				return err
			}
			return compareNodeConfig(obj, testNC1)
		}).ShouldNot(HaveOccurred())
	})

})

func compareNodeConfig(got, want v1beta1.NodeConfig) error {
	var err error
	if got.Spec.NodeName != want.Spec.NodeName {
		err = errors.Join(err, fmt.Errorf("NodeName: got %s, want %s", got.Spec.NodeName, want.Spec.NodeName))
	}
	if diff := cmp.Diff(got.Spec.MetricsCollector, want.Spec.MetricsCollector); diff != "" {
		err = errors.Join(err, fmt.Errorf("MetricsCollector: %s", diff))
	}
	if diff := cmp.Diff(got.Spec.Predictor, want.Spec.Predictor); diff != "" {
		err = errors.Join(err, fmt.Errorf("Predictor: %s", diff))
	}
	return err
}
