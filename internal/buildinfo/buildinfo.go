package buildinfo

const defaultVersion = "dev"

var version = defaultVersion

func Version() string {
	if version == "" {
		return defaultVersion
	}
	return version
}
