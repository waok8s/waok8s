package controller_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	waoclient "github.com/waok8s/wao-core/pkg/client"
	waocontroller "github.com/waok8s/wao-core/pkg/controller"
	waometrics "github.com/waok8s/wao-core/pkg/metrics"
	waopredictor "github.com/waok8s/wao-core/pkg/predictor"

	"github.com/waok8s/wao-core/api/wao/v1beta1"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

var secretClient *kubernetes.Clientset
var cachedPredictorClient *waoclient.CachedPredictorClient

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
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

	// init clients
	secretClient = kubernetes.NewForConfigOrDie(cfg)
	cachedPredictorClient = waoclient.NewCachedPredictorClient(secretClient, 10*time.Second)
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
	testSecretName = "test-secret"
	testSecret     = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testSecretName,
			Namespace: testNS,
		},
		Type: corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			// invalid base64 values, but it's ok for this test
			"username": []byte("test-user"),
			"password": []byte("test-password"),
		},
	}
	testNC0EP         = "http://10.0.0.100/10.0.0.100-node-0"
	testNC1EP         = "http://10.0.0.101/10.0.0.101-node-1"
	testFetchInterval = metav1.Duration{Duration: 1 * time.Second}
	testNC0           = v1beta1.NodeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNode0Name,
			Namespace: testNS,
		},
		Spec: v1beta1.NodeConfigSpec{
			NodeName: testNode0Name,
			MetricsCollector: v1beta1.MetricsCollector{
				InletTemp: v1beta1.EndpointTerm{
					Type:            v1beta1.TypeFake,
					Endpoint:        testNC0EP,
					BasicAuthSecret: &corev1.LocalObjectReference{Name: testSecretName},
					FetchInterval:   &testFetchInterval,
				},
				DeltaP: v1beta1.EndpointTerm{
					Type:          v1beta1.TypeFake,
					Endpoint:      testNC0EP,
					FetchInterval: &testFetchInterval,
				},
			},
			Predictor: v1beta1.Predictor{
				PowerConsumption: &v1beta1.EndpointTerm{
					Type:     v1beta1.TypeFake,
					Endpoint: testNC0EP,
					BasicAuthSecret: &corev1.LocalObjectReference{
						Name: testSecretName,
					},
				},
			},
		},
	}
	testNC1 = v1beta1.NodeConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNode1Name,
			Namespace: testNS,
		},
		Spec: v1beta1.NodeConfigSpec{
			NodeName: testNode1Name,
			MetricsCollector: v1beta1.MetricsCollector{
				InletTemp: v1beta1.EndpointTerm{
					Type:          v1beta1.TypeFake,
					Endpoint:      testNC1EP,
					FetchInterval: &testFetchInterval,
				},
				DeltaP: v1beta1.EndpointTerm{
					Type:            v1beta1.TypeFake,
					Endpoint:        testNC1EP,
					BasicAuthSecret: &corev1.LocalObjectReference{Name: testSecretName},
					FetchInterval:   &testFetchInterval,
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

var _ = Describe("NodeConfig Controller", func() {
	var cncl context.CancelFunc
	var reconciler waocontroller.NodeConfigReconciler

	BeforeEach(func() {
		ctx, cancel := context.WithCancel(context.Background())
		cncl = cancel

		var err error

		// Setup Namespace
		k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNS}})

		// Reset resources
		err = k8sClient.DeleteAllOf(ctx, &v1beta1.NodeConfig{}, client.InNamespace(testNS))
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.DeleteAllOf(ctx, &corev1.Secret{}, client.InNamespace(testNS))
		Expect(err).NotTo(HaveOccurred())
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

		// Setup manager
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme.Scheme,
			Metrics: metricsserver.Options{
				BindAddress: "0", // disable metrics server to avoid port conflict
			},
		})
		Expect(err).NotTo(HaveOccurred())

		reconciler = waocontroller.NodeConfigReconciler{
			Client:           k8sClient,
			Scheme:           scheme.Scheme,
			SecretClient:     kubernetes.NewForConfigOrDie(cfg),
			MetricsCollector: &waometrics.Collector{},
			MetricsStore:     &waometrics.Store{},
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

	It("should start agentRunners", func() {
		ctx := context.Background()

		var err error

		// Create Secret
		err = k8sClient.Create(ctx, testSecret.DeepCopy())
		Expect(err).NotTo(HaveOccurred())

		// Create NodeConfig
		err = k8sClient.Create(ctx, testNC0.DeepCopy())
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.Create(ctx, testNC1.DeepCopy())
		Expect(err).NotTo(HaveOccurred())

		// Wait for agentRunners to start
		<-time.After(testFetchInterval.Duration * 3)

		// Check stored metrics
		for _, nodeName := range []string{testNode0Name, testNode1Name} {
			md, ok := reconciler.MetricsStore.Get(waometrics.StoreKeyForNode(nodeName))
			Expect(ok).To(BeTrue())
			// no check timestamp because it's not deterministic
			Expect(md.InletTemp).To(Equal(15.5))
			Expect(md.DeltaPressure).To(Equal(7.5))
		}
	})

	It("should parse predictors", func() {
		ctx := context.Background()

		var err error

		// Create Secret
		err = k8sClient.Create(ctx, testSecret.DeepCopy())
		Expect(err).NotTo(HaveOccurred())

		// Create NodeConfig
		err = k8sClient.Create(ctx, testNC0.DeepCopy())
		Expect(err).NotTo(HaveOccurred())
		err = k8sClient.Create(ctx, testNC1.DeepCopy())
		Expect(err).NotTo(HaveOccurred())

		// node-0: direct endpoint
		v, err := cachedPredictorClient.PredictPowerConsumption(ctx, testNS, testNC0.Spec.Predictor.PowerConsumption, 20.0, 15.5, 7.5)
		Expect(err).NotTo(HaveOccurred())
		Expect(v).To(Equal(3.14)) // fake value

		// node-1: endpoint provider
		ep, err := cachedPredictorClient.GetPredictorEndpoint(ctx, testNS, testNC1.Spec.Predictor.PowerConsumptionEndpointProvider, waopredictor.TypePowerConsumption)
		Expect(err).NotTo(HaveOccurred())
		Expect(ep).To(Equal(&v1beta1.EndpointTerm{
			// fake endpoint provider returns this value
			Type:     v1beta1.TypeFake,
			Endpoint: "https://fake-endpoint",
		}))
	})

})
