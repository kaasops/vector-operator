package config

func addPrefix(Namespace, Name, componentName string) string {
	return generateName(Namespace, Name) + "-" + componentName
}

func generateName(Namespace, Name string) string {
	if Namespace != "" {
		return Namespace + "-" + Name
	}
	return Name
}
