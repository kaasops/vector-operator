package config

func addPrefix(Namespace, Name, componentName string, skipPrefix bool) string {
	if skipPrefix {
		return componentName
	}
	return generateName(Namespace, Name) + "-" + componentName
}

func generateName(Namespace, Name string) string {
	if Namespace != "" {
		return Namespace + "-" + Name
	}
	return Name
}
