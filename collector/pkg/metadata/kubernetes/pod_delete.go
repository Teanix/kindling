// Copyright 2020 OpenTelemetry Authors
// Source: https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/main/processor/k8sattributesprocessor/internal/kube/client.go
// Modification: Use deletedPodInfo as deleted elements and delete our cache map as needed.

package kubernetes

import (
	"sync"
	"time"
)

var (
	podDeleteQueueMut sync.Mutex
	podDeleteQueue    []deleteRequest
)

type deleteRequest struct {
	podInfo *deletedPodInfo
	ts      time.Time
}

type deletedPodInfo struct {
	name         string
	namespace    string
	containerIds []string
	ip           string
	ports        []int32
	hostIp       string
	hostPorts    []int32
}

// deleteLoop deletes pods from cache periodically.
func podDeleteLoop(interval time.Duration, gracePeriod time.Duration, stopCh chan struct{}) {
	// This loop runs after N seconds and deletes pods from cache.
	// It iterates over the delete queue and deletes all that aren't
	// in the grace period anymore.
	for {
		select {
		case <-time.After(interval):
			var cutoff int
			now := time.Now()
			podDeleteQueueMut.Lock()
			for i, d := range podDeleteQueue {
				if d.ts.Add(gracePeriod).After(now) {
					break
				}
				cutoff = i + 1
			}
			toDelete := podDeleteQueue[:cutoff]
			podDeleteQueue = podDeleteQueue[cutoff:]
			podDeleteQueueMut.Unlock()
			for _, d := range toDelete {
				deletePodInfo(d.podInfo)
			}

		case <-stopCh:
			return
		}
	}
}

func deletePodInfo(podInfo *deletedPodInfo) {
	if podInfo.name != "" {
		globalPodInfo.delete(podInfo.namespace, podInfo.name)
	}
	if len(podInfo.containerIds) != 0 {
		for i := 0; i < len(podInfo.containerIds); i++ {
			MetaDataCache.DeleteByContainerId(podInfo.containerIds[i])
		}
	}
	if podInfo.ip != "" && len(podInfo.ports) != 0 {
		for _, port := range podInfo.ports {
			// Assume that PodIP:Port can't be reused in a few seconds
			MetaDataCache.DeleteContainerByIpPort(podInfo.ip, uint32(port))
		}
	}
	if podInfo.hostIp != "" && len(podInfo.hostPorts) != 0 {
		for _, port := range podInfo.hostPorts {
			MetaDataCache.DeleteContainerByHostIpPort(podInfo.hostIp, uint32(port))
		}
	}
}
