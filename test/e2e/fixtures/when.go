package fixtures

import (
	"testing"
	"time"

	"github.com/argoproj/pkg/humanize"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	wfv1 "github.com/argoproj/argo/pkg/apis/workflow/v1alpha1"
	"github.com/argoproj/argo/pkg/client/clientset/versioned/typed/workflow/v1alpha1"
	"github.com/argoproj/argo/test/util"
	"github.com/argoproj/argo/workflow/hydrator"
)

type When struct {
	t                 *testing.T
	wf                *wfv1.Workflow
	wfeb              *wfv1.WorkflowEventBinding
	wfTemplates       []*wfv1.WorkflowTemplate
	cwfTemplates      []*wfv1.ClusterWorkflowTemplate
	cronWf            *wfv1.CronWorkflow
	client            v1alpha1.WorkflowInterface
	wfebClient        v1alpha1.WorkflowEventBindingInterface
	wfTemplateClient  v1alpha1.WorkflowTemplateInterface
	cwfTemplateClient v1alpha1.ClusterWorkflowTemplateInterface
	cronClient        v1alpha1.CronWorkflowInterface
	hydrator          hydrator.Interface
	workflowName      string
	wfTemplateNames   []string
	cronWorkflowName  string
	kubeClient        kubernetes.Interface
	resourceQuota     *corev1.ResourceQuota
	storageQuota      *corev1.ResourceQuota
	configMap         *corev1.ConfigMap
}

func (w *When) SubmitWorkflow() *When {
	w.t.Helper()
	if w.wf == nil {
		w.t.Fatal("No workflow to submit")
	}
	log.WithFields(log.Fields{"workflow": w.wf.Name}).Info("Submitting workflow")
	wf, err := w.client.Create(w.wf)
	if err != nil {
		w.t.Fatal(err)
	} else {
		w.workflowName = wf.Name
	}
	log.WithFields(log.Fields{"workflow": wf.Name, "uid": wf.UID}).Info("Workflow submitted")
	return w
}

func (w *When) CreateWorkflowEventBinding() *When {
	w.t.Helper()
	if w.wfeb == nil {
		w.t.Fatal("No workflow event to create")
	}
	log.WithField("event", w.wfeb.Name).Info("Creating workflow event")
	_, err := w.wfebClient.Create(w.wfeb)
	if err != nil {
		w.t.Fatal(err)
	}
	return w
}

func (w *When) CreateWorkflowTemplates() *When {
	w.t.Helper()
	if len(w.wfTemplates) == 0 {
		w.t.Fatal("No workflow templates to create")
	}
	for _, wfTmpl := range w.wfTemplates {
		log.WithField("template", wfTmpl.Name).Info("Creating workflow template")
		wfTmpl, err := w.wfTemplateClient.Create(wfTmpl)
		if err != nil {
			w.t.Fatal(err)
		} else {
			w.wfTemplateNames = append(w.wfTemplateNames, wfTmpl.Name)
		}
		log.WithField("template", wfTmpl.Name).Info("Workflow template created")
	}
	return w
}

func (w *When) CreateClusterWorkflowTemplates() *When {
	w.t.Helper()
	if len(w.cwfTemplates) == 0 {
		w.t.Fatal("No cluster workflow templates to create")
	}
	for _, cwfTmpl := range w.cwfTemplates {
		log.WithField("template", cwfTmpl.Name).Info("Creating cluster workflow template")
		wfTmpl, err := w.cwfTemplateClient.Create(cwfTmpl)
		if err != nil {
			w.t.Fatal(err)
		} else {
			w.wfTemplateNames = append(w.wfTemplateNames, wfTmpl.Name)
		}
		log.WithField("template", wfTmpl.Name).Info("Cluster Workflow template created")
	}
	return w
}

func (w *When) CreateCronWorkflow() *When {
	w.t.Helper()
	if w.cronWf == nil {
		w.t.Fatal("No cron workflow to create")
	}
	log.WithField("cronWorkflow", w.cronWf.Name).Info("Creating cron workflow")
	cronWf, err := w.cronClient.Create(w.cronWf)
	if err != nil {
		w.t.Fatal(err)
	} else {
		w.cronWorkflowName = cronWf.Name
	}
	log.WithField("uid", cronWf.UID).Info("Cron workflow created")
	return w
}

func (w *When) WaitForWorkflowCondition(test func(wf *wfv1.Workflow) bool, condition string, duration time.Duration) *When {
	w.t.Helper()
	return w.waitForWorkflow(w.workflowName, test, condition, duration)
}

func (w *When) waitForWorkflow(workflowName string, test func(wf *wfv1.Workflow) bool, condition string, timeout time.Duration) *When {
	w.t.Helper()
	start := time.Now()

	fieldSelector := ""
	if workflowName != "" {
		fieldSelector = "metadata.name=" + workflowName
	}

	logCtx := log.WithFields(log.Fields{"fieldSelector": fieldSelector, "condition": condition, "timeout": timeout})
	logCtx.Info("Waiting for condition")
	opts := metav1.ListOptions{LabelSelector: Label, FieldSelector: fieldSelector}
	watch, err := w.client.Watch(opts)
	if err != nil {
		w.t.Fatal(err)
	}
	defer watch.Stop()
	timeoutCh := make(chan bool, 1)
	go func() {
		time.Sleep(timeout)
		timeoutCh <- true
	}()
	for {
		select {
		case event := <-watch.ResultChan():
			wf, ok := event.Object.(*wfv1.Workflow)
			if ok {
				logCtx.WithFields(log.Fields{"workflow": wf.Name, "type": event.Type, "phase": wf.Status.Phase, "message": wf.Status.Message}).Info("...")
				w.hydrateWorkflow(wf)
				if test(wf) {
					logCtx.Infof("Condition met after %v", time.Since(start).Truncate(time.Second))
					w.workflowName = wf.Name
					return w
				}
			} else {
				w.t.Fatal("not ok")
			}
		case <-timeoutCh:
			w.t.Fatalf("timeout after %v waiting for condition %s", timeout, condition)
		}
	}
}

func (w *When) hydrateWorkflow(wf *wfv1.Workflow) {
	w.t.Helper()
	err := w.hydrator.Hydrate(wf)
	if err != nil {
		w.t.Fatal(err)
	}
}
func (w *When) WaitForWorkflowToStart(timeout time.Duration) *When {
	w.t.Helper()
	return w.waitForWorkflow(w.workflowName, func(wf *wfv1.Workflow) bool {
		return !wf.Status.StartedAt.IsZero()
	}, "to start", timeout)
}

func (w *When) WaitForWorkflow(timeout time.Duration) *When {
	w.t.Helper()
	return w.waitForWorkflow(w.workflowName, func(wf *wfv1.Workflow) bool {
		return !wf.Status.FinishedAt.IsZero()
	}, "to finish", timeout)
}

func (w *When) WaitForWorkflowName(workflowName string, timeout time.Duration) *When {
	w.t.Helper()
	return w.waitForWorkflow(workflowName, func(wf *wfv1.Workflow) bool {
		return !wf.Status.FinishedAt.IsZero()
	}, "to finish", timeout)
}

func (w *When) Wait(timeout time.Duration) *When {
	w.t.Helper()
	log.Infof("Waiting for %s", humanize.Duration(timeout))
	time.Sleep(timeout)
	log.Infof("Done waiting")
	return w
}

func (w *When) DeleteWorkflow() *When {
	w.t.Helper()
	log.WithField("workflow", w.workflowName).Info("Deleting")
	err := w.client.Delete(w.workflowName, nil)
	if err != nil {
		w.t.Fatal(err)
	}
	return w
}

func (w *When) And(block func()) *When {
	w.t.Helper()
	block()
	if w.t.Failed() {
		w.t.FailNow()
	}
	return w
}

func (w *When) RunCli(args []string, block func(t *testing.T, output string, err error)) *When {
	w.t.Helper()
	output, err := runCli("../../dist/argo", append([]string{"-n", Namespace}, args...)...)
	block(w.t, output, err)
	if w.t.Failed() {
		w.t.FailNow()
	}
	return w
}

func (w *When) CreateConfigMap(name string, data map[string]string) *When {
	w.t.Helper()
	//Clean if same map is already exist
	err := w.kubeClient.CoreV1().ConfigMaps("argo").Delete(name, &metav1.DeleteOptions{})
	if err != nil {
		if !apierr.IsNotFound(err) {
			panic(err)
		}
	}
	obj, err := util.CreateConfigMap(w.kubeClient, "argo", name, data)
	if err != nil {
		w.t.Fatal(err)
	}
	w.configMap = obj
	return w
}

func (w *When) DeleteConfigMap() *When {
	w.t.Helper()
	err := util.DeleteConfigMap(w.kubeClient, w.configMap)
	if err != nil {
		if !apierr.IsNotFound(err) {
			w.t.Fatal(err)
		}
	}
	w.configMap = nil
	return w
}

func (w *When) MemoryQuota(quota string) *When {
	w.t.Helper()
	obj, err := util.CreateHardMemoryQuota(w.kubeClient, "argo", "memory-quota", quota)
	if err != nil {
		w.t.Fatal(err)
	}
	w.resourceQuota = obj
	return w
}

func (w *When) StorageQuota(quota string) *When {
	w.t.Helper()
	obj, err := util.CreateHardStorageQuota(w.kubeClient, "argo", "storage-quota", quota)
	if err != nil {
		w.t.Fatal(err)
	}
	w.storageQuota = obj
	return w
}

func (w *When) DeleteStorageQuota() *When {
	w.t.Helper()
	err := util.DeleteQuota(w.kubeClient, w.storageQuota)
	if err != nil {
		w.t.Fatal(err)
	}
	w.storageQuota = nil
	return w
}

func (w *When) DeleteQuota() *When {
	w.t.Helper()
	err := util.DeleteQuota(w.kubeClient, w.resourceQuota)
	if err != nil {
		w.t.Fatal(err)
	}
	w.resourceQuota = nil
	return w
}

func (w *When) Then() *Then {
	return &Then{
		t:                w.t,
		workflowName:     w.workflowName,
		wfTemplateNames:  w.wfTemplateNames,
		cronWorkflowName: w.cronWorkflowName,
		client:           w.client,
		cronClient:       w.cronClient,
		hydrator:         w.hydrator,
		kubeClient:       w.kubeClient,
	}
}

func (w *When) Given() *Given {
	return &Given{
		t:                 w.t,
		client:            w.client,
		wfebClient:        w.wfebClient,
		wfTemplateClient:  w.wfTemplateClient,
		cwfTemplateClient: w.cwfTemplateClient,
		cronClient:        w.cronClient,
		hydrator:          w.hydrator,
		wf:                w.wf,
		wfeb:              w.wfeb,
		wfTemplates:       w.wfTemplates,
		cwfTemplates:      w.cwfTemplates,
		cronWf:            w.cronWf,
		workflowName:      w.workflowName,
		kubeClient:        w.kubeClient,
	}
}
