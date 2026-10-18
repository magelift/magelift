//go:build windows

package containerrunner

func containerUser() string {
	return "10001:10001"
}
