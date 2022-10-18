// SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors.
//
// SPDX-License-Identifier: Apache-2.0

package scaler

import (
	"crypto/sha256"

	"github.com/gardener/dependency-watchdog/pkg/scaler/api"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

var (
	kubeconfig1 = `apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    insecure-skip-tls-verify: true
    server: https://localhost:433/1
contexts:
- context:
    cluster: local
    user: admin
  name: context
current-context: context
users:
- name: admin
  user:
    password: admin
    username: admin`

	kubeconfig2 = `apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    insecure-skip-tls-verify: true
    server: https://localhost:433/2
contexts:
- context:
    cluster: local
    user: admin
  name: context
current-context: context
users:
- name: admin
  user:
    password: admin
    username: admin`
)

func shaOf(s string) []byte {
	sha := sha256.Sum256([]byte(s))
	return sha[:]
}

var _ = Describe("prober", func() {
	const (
		ns         = "test"
		secretName = "test"
	)
	DescribeTable("getClientFromSecret", func(oldSHA []byte, kubeconfig string, expectedSHA []byte, expectedErr error) {
		var (
			timeout = int32(10)
			indexer = cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{})
		)

		p := &prober{
			namespace:    ns,
			secretLister: listerv1.NewSecretLister(indexer),
			probeDeps: &api.ProbeDependants{
				Probe: &api.ProbeConfig{
					TimeoutSeconds: &timeout,
				},
			},
		}

		if kubeconfig != "" {
			indexingError := indexer.Add(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: ns,
				},
				Data: map[string][]byte{
					"kubeconfig": []byte(kubeconfig),
				},
			})
			if indexingError != nil {
				return
			}

			var snl = p.secretLister.Secrets(ns)
			Expect(snl).ToNot(BeNil())

			var secret, err = snl.Get(secretName)
			Expect(err).ToNot(HaveOccurred())
			Expect(secret).ToNot(BeNil())
		}

		_, actualSHA, actualErr := p.getClientFromSecret(secretName, oldSHA)

		if expectedSHA == nil {
			Expect(actualSHA).To(BeNil())
		} else {
			Expect(actualSHA).To(Equal(expectedSHA))
		}

		if expectedErr == nil {
			Expect(actualErr).To(BeNil())
		} else {
			Expect(actualErr).To(Equal(expectedErr))
		}
	},
		Entry("No kubeconfig secret", nil, "", nil, apierrors.NewNotFound(schema.GroupResource{Resource: "secret"}, secretName)),
		Entry("No cached oldSHA", nil, kubeconfig1, shaOf(kubeconfig1), nil),
		Entry("No change in kubeconfig", shaOf(kubeconfig1), kubeconfig1, nil, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "secret"}, secretName)),
		Entry("Changed kubeconfig", shaOf(kubeconfig1), kubeconfig2, shaOf(kubeconfig2), nil))
})
