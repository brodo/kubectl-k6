package internal

import (
	"context"
	errors2 "errors"
	"fmt"
	"golang.org/x/sync/errgroup"
	"io"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"strings"
	"sync"
	"time"
)

type K8sClient struct {
	clientSet     *kubernetes.Clientset
	dynamicClient *dynamic.DynamicClient
	namespace     string
	k6GVR         schema.GroupVersionResource
}

type LogsWithNames struct {
	PodName string
	Logs    string
}

type Stage int

const (
	InitializationStage Stage = iota
	InitializedStage
	CreatedStage
	StartedStage
	StoppedStage
	FinishedStage
	ErrorStage
)

var stages = map[string]Stage{
	"initialization": InitializationStage,
	"initialized":    InitializedStage,
	"created":        CreatedStage,
	"started":        StartedStage,
	"stopped":        StoppedStage,
	"finished":       FinishedStage,
	"error":          ErrorStage,
}

func NewK8sClient(k8sConfig *rest.Config, namespace string) (error, K8sClient) {
	clientSet, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return err, K8sClient{}
	}
	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	if err != nil {
		return err, K8sClient{}
	}
	return nil, K8sClient{clientSet: clientSet, dynamicClient: dynamicClient, namespace: namespace, k6GVR: schema.GroupVersionResource{
		Group:    "k6.io",
		Version:  "v1alpha1",
		Resource: "testruns",
	}}
}

func (kc *K8sClient) DeleteResources(ctx context.Context, sps *ScriptProperties) error {
	setupCtx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()
	eg, _ := errgroup.WithContext(setupCtx)
	eg.Go(func() error {
		return kc.DeleteConfigMap(setupCtx, sps.ConfigMapName())
	})
	eg.Go(func() error {
		return kc.DeleteCustomResource(setupCtx, sps.ResourceName())
	})
	return eg.Wait()
}

func (kc *K8sClient) CreateConfigMap(ctx context.Context, sps *ScriptProperties, scriptContent string) error {
	configMap := &v1.ConfigMap{
		ObjectMeta: meta.ObjectMeta{
			Name:      sps.ConfigMapName(),
			Namespace: kc.namespace,
		},
		Data: map[string]string{
			"out.js": scriptContent,
		},
	}
	_, err := kc.clientSet.CoreV1().ConfigMaps(kc.namespace).Create(ctx, configMap, meta.CreateOptions{})
	return err
}

func (kc *K8sClient) GetConfigMap(ctx context.Context, name string) (*v1.ConfigMap, error) {
	m, err := kc.clientSet.CoreV1().ConfigMaps(kc.namespace).Get(ctx, name, meta.GetOptions{})
	return m, err
}

func (kc *K8sClient) DeleteConfigMap(ctx context.Context, configMapName string) error {
	deletePolicy := meta.DeletePropagationForeground
	if err := kc.clientSet.CoreV1().ConfigMaps(kc.namespace).Delete(ctx, configMapName, meta.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		if !errors.IsNotFound(err) { // Ignore error if ConfigMap not found
			return err
		}
	} else {
		// Wait for the deletion to complete
		err = wait.PollUntilContextTimeout(ctx, time.Second, 5*time.Second, true, func(ctx context.Context) (done bool, err error) {
			_, getErr := kc.clientSet.CoreV1().ConfigMaps(kc.namespace).Get(ctx, configMapName, meta.GetOptions{})
			if errors.IsNotFound(getErr) {
				return true, nil // CR is deleted
			}
			return false, getErr // Continue polling
		})
		if err != nil {
			return fmt.Errorf("error waiting for confir map '%s' deletion: %v", configMapName, err)
		}
	}
	return nil
}

func (kc *K8sClient) CreateCustomResource(ctx context.Context, k6Conf *K6Config, tVars *TemplateVars) error {
	var script map[string]interface{}

	if k6Conf.Folder == "" {
		script = map[string]interface{}{
			"configMap": map[string]interface{}{
				"name": tVars.ConfigMapName(),
				"file": "out.js",
			},
		}
	} else {
		script = map[string]interface{}{
			"volumeClaim": map[string]interface{}{
				"name": tVars.ConfigMapName(),
				"file": k6Conf.FilePath,
			},
		}
	}

	k6CR := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "k6.io/v1alpha1",
			"kind":       "TestRun",
			"metadata": map[string]interface{}{
				"name": tVars.ResourceName(),
			},
			"spec": map[string]interface{}{
				"parallelism": k6Conf.Parallelism,
				"arguments":   k6Conf.Args,
				"script":      script,
				"runner":      map[string]interface{}{},
			},
		},
	}
	if len(k6Conf.Env) > 0 {
		k6CR.Object["spec"].(map[string]interface{})["runner"].(map[string]interface{})["env"] = k6Conf.Env.ToMapSlice()
	}
	if k6Conf.Image != "" {
		k6CR.Object["spec"].(map[string]interface{})["runner"].(map[string]interface{})["image"] = k6Conf.Image
		k6CR.Object["spec"].(map[string]interface{})["runner"].(map[string]interface{})["imagePullSecrets"] = []map[string]string{
			{"name": k6Conf.ImagePullSecret},
		}
	}

	_, err := kc.dynamicClient.Resource(kc.k6GVR).Namespace(kc.namespace).Create(ctx, k6CR, meta.CreateOptions{})
	return err
}

func (kc *K8sClient) GetCustomResource(ctx context.Context, resName string) (*unstructured.Unstructured, error) {
	return kc.dynamicClient.Resource(kc.k6GVR).Namespace(kc.namespace).Get(ctx, resName, meta.GetOptions{})
}

func (kc *K8sClient) GetCurrK6Stage(ctx context.Context, resName string) (string, error) {
	k6CR, err := kc.GetCustomResource(ctx, resName)
	if err != nil {
		return "", err
	}
	return k6CR.Object["status"].(map[string]interface{})["stage"].(string), nil
}

func (kc *K8sClient) DeleteCustomResource(ctx context.Context, resName string) error {
	deletePolicy := meta.DeletePropagationForeground
	if err := kc.dynamicClient.Resource(kc.k6GVR).Namespace(kc.namespace).Delete(ctx, resName, meta.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	}); err != nil {
		if !errors.IsNotFound(err) { // Ignore if CR not found
			return err
		}
	} else {
		// Wait for the deletion to complete
		err = wait.PollUntilContextTimeout(ctx, time.Second, 5*time.Second, true, func(ctx context.Context) (done bool, err error) {
			_, getErr := kc.dynamicClient.Resource(kc.k6GVR).Namespace(kc.namespace).Get(context.TODO(), resName, meta.GetOptions{})
			if errors.IsNotFound(getErr) {
				return true, nil // CR is deleted
			}
			return false, getErr // Continue polling

		})
		if err != nil {
			return fmt.Errorf("error waiting for custom resource '%s' deletion: %v", resName, err)
		}
	}
	return nil
}

func (kc *K8sClient) WaitForStage(cxt context.Context, resName string, expectedStage Stage) error {
	return wait.PollUntilContextTimeout(cxt, 2*time.Second, time.Minute*3, false, func(ctx context.Context) (done bool, err error) {
		stage, err := kc.GetCurrK6Stage(ctx, resName)
		if err != nil {
			return false, err
		}
		if stage == "error" {
			return true, fmt.Errorf("k6 run failed")
		}
		if stages[stage] >= expectedStage {
			return true, nil
		}
		return false, nil
	})

}

func (kc *K8sClient) WaitForInitJobCompletion(cxt context.Context, sps *ScriptProperties, startTime time.Time) error {
	return wait.PollUntilContextTimeout(cxt, 2*time.Second, time.Minute*3, true, func(ctx context.Context) (done bool, err error) {
		job, getErr := kc.clientSet.BatchV1().Jobs(kc.namespace).Get(ctx, sps.InitJobName(), meta.GetOptions{})
		if errors.IsNotFound(getErr) {
			return false, nil
		}

		if job.Status.Failed > 0 {
			// Get logs for the failed job
			err = fmt.Errorf("job '%s' failed", sps.InitJobName())
			logs, glErr := kc.GetPodLogs(ctx, sps.InitJobName(), startTime)
			if glErr != nil {
				err = fmt.Errorf("error getting logs for job '%s': %w", sps.InitJobName(), glErr)
			} else {
				err = fmt.Errorf("previous error: %w, logs: %s", err, logs)
			}
			return true, err
		}
		if job.Status.Failed+job.Status.Succeeded == 1 {
			return true, nil
		}
		return false, getErr // Continue polling
	})
}

func (kc *K8sClient) getJobPods(ctx context.Context, jobName string) ([]v1.Pod, error) {
	podList, err := kc.clientSet.CoreV1().Pods(kc.namespace).List(ctx, meta.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func (kc *K8sClient) GetJobPodLogs(ctx context.Context, jobName string) ([]LogsWithNames, error) {
	podList, err := kc.getJobPods(ctx, jobName)
	if err != nil {
		return nil, err
	}
	logs := make([]LogsWithNames, len(podList))
	for i, pod := range podList {
		podLogs, err := kc.clientSet.CoreV1().Pods(kc.namespace).GetLogs(pod.Name, &v1.PodLogOptions{}).Do(ctx).Raw()
		if err != nil {
			return nil, err
		}
		logs[i] = LogsWithNames{PodName: pod.Name, Logs: string(podLogs)}
	}
	return logs, nil
}

type errSync struct {
	errs []error
	mu   sync.Mutex
}

func (kc *K8sClient) WaitForRunJobCompletion(ctx context.Context, sps *ScriptProperties, k6Conf *K6Config, startTime time.Time) error {
	var wg sync.WaitGroup
	es := &errSync{}
	for i := 0; i < k6Conf.Parallelism; i++ {
		wg.Add(1)
		runnerJobName := sps.RunnerJobName(i)
		go func() {
			defer wg.Done()
			err := func() error {
				return wait.PollUntilContextTimeout(ctx, 10*time.Second, time.Hour, false, func(ctx context.Context) (done bool, err error) {
					job, getErr := kc.clientSet.BatchV1().Jobs(kc.namespace).Get(ctx, runnerJobName, meta.GetOptions{})
					if errors.IsNotFound(getErr) {
						return false, nil
					}

					var failErr error = nil
					if job.Status.Failed > 0 {
						// Get logs for the failed job
						logs, err := kc.GetPodLogs(ctx, runnerJobName, startTime)
						if err != nil {
							failErr = fmt.Errorf("job '%s' failed, could not retreive logs:\n %w", runnerJobName, err)
						} else {
							failErr = fmt.Errorf("job '%s' failed, logs:\n%s", runnerJobName, logs)
						}

					}

					if job.Status.Failed+job.Status.Succeeded == 1 {
						return true, failErr
					}
					return false, getErr // Continue polling
				})
			}()
			es.mu.Lock()
			es.errs = append(es.errs, err)
			es.mu.Unlock()
		}()
	}
	wg.Wait()
	return errors2.Join(es.errs...)
}

// GetPodLogs retrieves the logs of a specific job in the given namespace.
// It takes a context, a Kubernetes clientset, and the name of the job.
// It returns the logs as a string and an error if any occurred.
func (kc *K8sClient) GetPodLogs(ctx context.Context, jobName string, since time.Time) (string, error) {
	podList, err := kc.clientSet.CoreV1().Pods(kc.namespace).List(ctx, meta.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil {
		return "", err
	}
	for _, pod := range podList.Items {
		podLogs, err := kc.clientSet.CoreV1().Pods(kc.namespace).GetLogs(pod.Name, &v1.PodLogOptions{SinceTime: &meta.Time{Time: since}}).Do(ctx).Raw()
		if err != nil {
			return "", err
		}
		return string(podLogs), nil
	}
	return "", fmt.Errorf("job '%s' does not exist", jobName)
}

func (kc *K8sClient) GetPodLogStream(ctx context.Context, jobName string, since time.Time) (io.ReadCloser, error) {
	podList, err := kc.clientSet.CoreV1().Pods(kc.namespace).List(ctx, meta.ListOptions{
		LabelSelector: fmt.Sprintf("job-name=%s", jobName),
	})
	if err != nil {
		return nil, err
	}
	count := int64(100)
	podLogOptions := v1.PodLogOptions{
		SinceTime: &meta.Time{Time: since},
		Follow:    true,
		TailLines: &count,
	}
	for _, pod := range podList.Items {
		return kc.clientSet.CoreV1().Pods(kc.namespace).GetLogs(pod.Name, &podLogOptions).Stream(ctx)
	}
	return nil, fmt.Errorf("job '%s' does not exist", jobName)
}

func (kc *K8sClient) GetOperatorLogsSince(ctx context.Context, since time.Time) (string, error) {
	podList, err := kc.clientSet.CoreV1().Pods(kc.namespace).List(ctx, meta.ListOptions{
		LabelSelector: "app.kubernetes.io/name=k6-operator",
	})
	if err != nil {
		return "", err
	}
	var logs strings.Builder
	for _, pod := range podList.Items {
		podLogs, err := kc.clientSet.CoreV1().Pods(kc.namespace).GetLogs(pod.Name, &v1.PodLogOptions{
			SinceTime: &meta.Time{Time: since},
			Container: "manager", // only get logs from the "manager" container
		}).Do(ctx).Raw()
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(string(podLogs), "\n") {
			if strings.Contains(line, "ERROR") || strings.Contains(line, "WARN") {
				logs.WriteString(line + "\n")
			}
		}
	}
	return logs.String(), nil
}

func (kc *K8sClient) UploadFolderToPV(ctx context.Context, folder, pvName, namespace string) error {
	pv := &v1.PersistentVolume{
		ObjectMeta: meta.ObjectMeta{
			Name:      pvName,
			Namespace: namespace,
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse("1Gi"),
			},
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteOnce,
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: folder,
				},
			},
		},
	}
	_, err := kc.clientSet.CoreV1().PersistentVolumes().Create(ctx, pv, meta.CreateOptions{})
	return err
}

func (kc *K8sClient) DeletePV(ctx context.Context, pvName string) error {
	deletePolicy := meta.DeletePropagationForeground
	err := kc.clientSet.CoreV1().PersistentVolumes().Delete(ctx, pvName, meta.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})
	return err
}

func (kc *K8sClient) CreatePVC(ctx context.Context, pvName, pvcName, namespace string) error {
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: meta.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName:  pvName,
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources: v1.VolumeResourceRequirements{Requests: v1.ResourceList{
				"storage": resource.MustParse("1Gi"),
			}},
		},
	}
	_, err := kc.clientSet.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, meta.CreateOptions{})
	return err
}
