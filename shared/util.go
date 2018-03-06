package shared

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	lxd "github.com/lxc/lxd/shared"
)

// Copy copies a file.
func Copy(src, dest string) error {
	var err error

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

// RunCommand runs a command hereby setting the SHELL and PATH env variables,
// and redirecting the process's stdout and stderr to the real stdout and stderr
// respectively.
func RunCommand(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)

	cmd.Env = []string{
		"PATH=/sbin:/bin:/usr/sbin:/usr/bin:/usr/local/sbin:/usr/local/bin",
		"SHELL=/bin/sh",
		"TERM=xterm"}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// VerifyFile verifies a file using gpg.
func VerifyFile(signedFile, signatureFile string, keys []string, keyserver string) (bool, error) {
	keyring, err := CreateGPGKeyring(keyserver, keys)
	if err != nil {
		return false, err
	}
	gpgDir := path.Dir(keyring)
	defer os.RemoveAll(gpgDir)

	if signatureFile != "" {
		out, err := lxd.RunCommand("gpg", "--homedir", gpgDir, "--keyring", keyring,
			"--verify", signatureFile, signedFile)
		if err != nil {
			return false, fmt.Errorf("Failed to verify: %s", out)
		}
	} else {
		out, err := lxd.RunCommand("gpg", "--homedir", gpgDir, "--keyring", keyring,
			"--verify", signedFile)
		if err != nil {
			return false, fmt.Errorf("Failed to verify: %s", out)
		}
	}

	return true, nil
}

// CreateGPGKeyring creates a new GPG keyring.
func CreateGPGKeyring(keyserver string, keys []string) (string, error) {
	gpgDir, err := ioutil.TempDir(os.TempDir(), "distrobuilder.")
	if err != nil {
		return "", fmt.Errorf("Failed to create gpg directory: %s", err)
	}

	err = os.MkdirAll(gpgDir, 0700)
	if err != nil {
		return "", err
	}

	args := []string{"--homedir", gpgDir}

	if keyserver != "" {
		args = append(args, "--keyserver", keyserver)
	}

	args = append(args, append([]string{"--recv-keys"}, keys...)...)

	out, err := lxd.RunCommand("gpg", args...)
	if err != nil {
		os.RemoveAll(gpgDir)
		return "", fmt.Errorf("Failed to create keyring: %s", out)
	}

	// Export keys to support gpg1 and gpg2
	out, err = lxd.RunCommand("gpg", "--homedir", gpgDir, "--export", "--output",
		filepath.Join(gpgDir, "distrobuilder.gpg"))
	if err != nil {
		os.RemoveAll(gpgDir)
		return "", fmt.Errorf("Failed to export keyring: %s", out)
	}

	return filepath.Join(gpgDir, "distrobuilder.gpg"), nil
}

// Pack creates an xz-compressed tarball.
func Pack(filename, path string, args ...string) error {
	return RunCommand("tar", append([]string{"-cJf", filename, "-C", path}, args...)...)
}

//GetExpiryDate returns an expiry date based on the creationDate and format.
func GetExpiryDate(creationDate time.Time, format string) time.Time {
	regex := regexp.MustCompile(`(?:(\d+)(s|m|h|d|w))*`)
	expiryDate := creationDate

	for _, match := range regex.FindAllStringSubmatch(format, -1) {
		// Ignore empty matches
		if match[0] == "" {
			continue
		}

		var duration time.Duration

		switch match[2] {
		case "s":
			duration = time.Second
		case "m":
			duration = time.Minute
		case "h":
			duration = time.Hour
		case "d":
			duration = 24 * time.Hour
		case "w":
			duration = 7 * 24 * time.Hour
		}

		// Ignore any error since it will be an integer.
		value, _ := strconv.Atoi(match[1])
		expiryDate = expiryDate.Add(time.Duration(value) * duration)
	}

	return expiryDate
}
