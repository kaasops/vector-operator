package config

import "encoding/json"

func decodeRaw(raw []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

func addPrefix(Namespace, Name, componentName string) string {
	return generateName(Namespace, Name) + "-" + componentName
}

func generateName(Namespace, Name string) string {
	if Namespace != "" {
		return Namespace + "-" + Name
	}
	return Name
}
