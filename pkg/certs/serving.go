/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package certs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ServingCertificateFiles are the file names of a serving certificate and of
// its private key inside a directory
type ServingCertificateFiles struct {
	// CertName is the file holding the certificate
	CertName string

	// KeyName is the file holding the private key
	KeyName string
}

// servingCertificateConventions are the file name conventions accepted for an
// externally provided webhook certificate, in order of preference: the names
// OLM writes first, then the names of a Kubernetes TLS Secret
var servingCertificateConventions = []ServingCertificateFiles{
	{CertName: "apiserver.crt", KeyName: "apiserver.key"},
	{CertName: TLSCertKey, KeyName: TLSPrivateKeyKey},
}

// FindServingCertificate looks in dir for a complete and readable certificate
// pair, trying each known convention in order, and returns the first one found.
// A pair with one file missing or unreadable is an error, as is a directory
// holding no pair at all, so that a misconfiguration stops the start right away
// instead of failing later when the webhook server opens the files.
func FindServingCertificate(dir string) (ServingCertificateFiles, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return ServingCertificateFiles{}, fmt.Errorf("webhook certificate directory: %w", err)
	}
	if !info.IsDir() {
		return ServingCertificateFiles{}, fmt.Errorf("webhook certificate directory %q is not a directory", dir)
	}

	for _, files := range servingCertificateConventions {
		certErr := checkReadableFile(filepath.Join(dir, files.CertName))
		keyErr := checkReadableFile(filepath.Join(dir, files.KeyName))
		if errors.Is(certErr, os.ErrNotExist) && errors.Is(keyErr, os.ErrNotExist) {
			continue
		}
		if certErr != nil {
			return ServingCertificateFiles{}, fmt.Errorf("webhook certificate %q: %w", files.CertName, certErr)
		}
		if keyErr != nil {
			return ServingCertificateFiles{}, fmt.Errorf("webhook private key %q: %w", files.KeyName, keyErr)
		}

		return files, nil
	}

	conventions := make([]string, 0, len(servingCertificateConventions))
	for _, files := range servingCertificateConventions {
		conventions = append(conventions, files.CertName+"/"+files.KeyName)
	}

	return ServingCertificateFiles{}, fmt.Errorf(
		"no webhook certificate pair in %q: expected %s", dir, strings.Join(conventions, " or "))
}

// checkReadableFile returns nil when path is a regular file, or a symbolic
// link to one, that this process can open for reading
func checkReadableFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", path)
	}

	file, err := os.Open(path) //nolint:gosec
	if err != nil {
		return err
	}

	return file.Close()
}
