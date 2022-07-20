package redirect

import (
	"bytes"
	_ "embed"

	appsv1 "k8s.io/api/apps/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

//go:embed etcd.yaml
var etcdYaml []byte

func GetEtcdSts(name string, namespace string, labels map[string]string) (*appsv1.StatefulSet, error) {

	etcdSts := &appsv1.StatefulSet{}
	dec := k8syaml.NewYAMLOrJSONDecoder(bytes.NewReader(etcdYaml), 1000)

	err := dec.Decode(etcdSts)
	if err != nil {
		return etcdSts, err
	}

	etcdSts.ObjectMeta.Name = name
	etcdSts.ObjectMeta.Namespace = namespace
	etcdSts.ObjectMeta.Labels = labels
	etcdSts.ObjectMeta.Labels["app"] = "etcd"
	etcdSts.Spec.Template.ObjectMeta.Labels = etcdSts.ObjectMeta.Labels
	etcdSts.Spec.Selector.MatchLabels = etcdSts.ObjectMeta.Labels

	return etcdSts, nil
}
