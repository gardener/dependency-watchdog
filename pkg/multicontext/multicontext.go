// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	defer close(m.ContextCh)

	klog.Info("Starting to listen for context start messages")
	for {
		select {
		case <-stopCh:
			klog.Info("Received stop signal. Stopping the handling of context messages.")
			m.cancelAll()
			return
		case cmsg := <-m.ContextCh:
			oldCancelFn, ok := m.CancelFns[cmsg.Key]
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
		delete(m.CancelFns, key)
		cancelFn()
	}
}
