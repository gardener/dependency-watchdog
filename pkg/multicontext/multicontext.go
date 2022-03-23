// SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package multicontext

import (
	"context"

	"k8s.io/klog"
)

// ContextMessage describes a context message.
type ContextMessage struct {
	Key      string
	CancelFn context.CancelFunc
}

// Multicontext provides a way to keep track of multiple contexts.
type Multicontext struct {
	CancelFns map[string]context.CancelFunc
	ContextCh chan *ContextMessage
}

// New returns a new instance of Multicontext.
func New() *Multicontext {
	return &Multicontext{
		CancelFns: make(map[string]context.CancelFunc),
		ContextCh: make(chan *ContextMessage),
	}
}

// Start will handle context messages and
// close any old context associated with the key from the message
// and register the new cancel function for the future.
// This is to ensure that there is only one active context for any given key.
func (m *Multicontext) Start(stopCh <-chan struct{}) {
	// m.ContextCh is never closed to avoid the possibility of senders sending to a closed channel
	// m.ContextCh is anyway not the channel to co-ordinate closing of goroutines.

	klog.Info("Starting to listen for context start messages")
	for {
		select {
		case <-stopCh:
			klog.Info("Received stop signal. Stopping the handling of context messages.")
			m.cancelAll()
			return
		case cmsg := <-m.ContextCh:
			oldCancelFn, ok := m.CancelFns[cmsg.Key]
			klog.V(4).Infof("Checking the oldCancelFn for key %v in the multicontext map and received ok code %v", cmsg.Key, ok)
			if cmsg.CancelFn != nil {
				klog.Infof("Registering the context for the key: %s", cmsg.Key)
				m.CancelFns[cmsg.Key] = cmsg.CancelFn
			} else {
				klog.Infof("Unregistering the context for the key: %s", cmsg.Key)
				delete(m.CancelFns, cmsg.Key)
			}

			if ok && oldCancelFn != nil {
				klog.Infof("Cancelling older context for the key: %s", cmsg.Key)
				oldCancelFn()
			}
		}
	}
}

func (m *Multicontext) cancelAll() {
	for key, cancelFn := range m.CancelFns {
		klog.V(4).Infof("Deleting cancelFn for key %s \n", key)
		delete(m.CancelFns, key)
		cancelFn()
	}
}
