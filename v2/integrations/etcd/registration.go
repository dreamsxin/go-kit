package etcd

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DefaultNamespace prefixes every service key. Keeping registrations under one
// namespace keeps a shared etcd cluster legible and makes a prefix watch cheap.
const DefaultNamespace = "/services"

// registration is the value stored under an instance key.
//
// etcd stores opaque bytes, so this package has to choose a format. JSON is the
// choice because it is what an operator sees in etcdctl output and because
// labels are open-ended. Metadata is map[string]string rather than any, so the
// wire format matches what every registry ends up delivering.
type registration struct {
	Address  string            `json:"address"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func encodeRegistration(address string, metadata map[string]string) (string, error) {
	encoded, err := json.Marshal(registration{Address: address, Metadata: metadata})
	if err != nil {
		return "", fmt.Errorf("etcd: encode registration: %w", err)
	}
	return string(encoded), nil
}

// decodeRegistration accepts either the JSON this package writes or a bare
// address. The bare form exists so that `etcdctl put /services/users/1
// 10.0.0.1:8080` works: a registry a human cannot populate by hand is hard to
// operate.
func decodeRegistration(value string) (Instance, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return Instance{}, fmt.Errorf("etcd: empty registration")
	}

	if !strings.HasPrefix(trimmed, "{") {
		return Instance{Address: trimmed}, nil
	}

	var decoded registration
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return Instance{}, fmt.Errorf("etcd: decode registration: %w", err)
	}
	if decoded.Address == "" {
		return Instance{}, fmt.Errorf("etcd: registration has no address")
	}

	instance := Instance{Address: decoded.Address}
	if len(decoded.Metadata) > 0 {
		instance.Metadata = make(map[string]any, len(decoded.Metadata))
		for key, label := range decoded.Metadata {
			instance.Metadata[key] = label
		}
	}
	return instance, nil
}

// servicePrefix is the key prefix holding one service's instances. The trailing
// separator matters: without it a watch on "users" would also match
// "users-admin".
func servicePrefix(namespace, service string) string {
	namespace = strings.TrimSuffix(strings.TrimSpace(namespace), "/")
	if namespace == "" {
		namespace = DefaultNamespace
	}
	return namespace + "/" + strings.Trim(service, "/") + "/"
}
